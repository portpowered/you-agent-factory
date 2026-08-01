package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// ContextLineReader is the exact cancellation-aware input role used by setup.
type ContextLineReader interface {
	ReadLine(context.Context) (string, error)
}

// ContextLineReaderFactory binds an invocation-local input stream to the
// policy-free line reader selected at composition.
type ContextLineReaderFactory func(io.Reader, int) (ContextLineReader, error)

// ConfigureConfig carries supplied provider/model setup inputs from the CLI boundary.
type ConfigureConfig struct {
	Context  context.Context
	HomeDir  string
	Provider string
	Model    *string
	Input    io.Reader
	Output   io.Writer
	// Interactive is true only when the process classified both stdin and
	// stdout as terminals. Supplied provider input never requires it.
	Interactive bool
	// NewLineReader is required for interactive prompting.
	NewLineReader ContextLineReaderFactory
}

// Configure delegates configure intent to the Settings-owned CLI adapter
// Service and surfaces typed results, validation failures, and cancel outcomes
// for CLI consumption.
func Configure(cfg ConfigureConfig, root operatorsettings.Service) error {
	if err := ValidateConfigureBoundary(cfg); err != nil {
		return err
	}
	adapter := New(root)
	if adapter == nil {
		return fmt.Errorf("operator settings service is required")
	}
	return adapter.Configure(cfg)
}

// ValidateConfigureBoundary rejects configure inputs before Settings root access.
func ValidateConfigureBoundary(cfg ConfigureConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("init context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("init output is required")
	}
	homeDir := strings.TrimSpace(cfg.HomeDir)
	if homeDir == "" {
		return fmt.Errorf("init home directory is required")
	}
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		if !cfg.Interactive {
			return fmt.Errorf("provider is required when interactive prompting is unavailable; use --provider")
		}
		if cfg.Input == nil {
			return fmt.Errorf("interactive init input is required")
		}
		return nil
	}
	if cfg.Model != nil && strings.TrimSpace(*cfg.Model) == "" {
		return fmt.Errorf("model must be non-empty when supplied")
	}
	return nil
}

func (service *service) Configure(cfg ConfigureConfig) error {
	if err := ValidateConfigureBoundary(cfg); err != nil {
		return err
	}
	homeDir := strings.TrimSpace(cfg.HomeDir)
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		return service.configurePrompted(cfg, homeDir)
	}
	var model *string
	if cfg.Model != nil {
		value := strings.TrimSpace(*cfg.Model)
		model = &value
	}

	path := service.root.DefaultConfigPath(homeDir)
	document, err := service.applyProviderModelUpdate(cfg.Context, path, operatorsettings.DocumentProviderModelUpdate{
		Provider: &provider,
		Model:    model,
	})
	if err != nil {
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	return reportConfiguredDefaults(cfg.Output, path, document)
}

func (service *service) configurePrompted(cfg ConfigureConfig, homeDir string) error {
	if cfg.NewLineReader == nil {
		return fmt.Errorf("interactive init line reader is required")
	}
	path := service.root.DefaultConfigPath(homeDir)
	if err := operationContextError(cfg.Context); err != nil {
		return err
	}
	loaded, err := service.root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	if err := operationContextError(cfg.Context); err != nil {
		return err
	}
	update, err := service.acquireProviderModelPrompt(cfg, loaded.Document.Defaults)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("configure provider/model defaults: setup canceled: %w", err)
		}
		if errors.Is(err, io.EOF) || errors.Is(err, operatorsettings.ErrProviderModelInputCanceled) {
			return fmt.Errorf("configure provider/model defaults: setup canceled: %w", operatorsettings.ErrProviderModelInputCanceled)
		}
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	if err := operationContextError(cfg.Context); err != nil {
		return err
	}
	document, err := service.applyProviderModelUpdate(cfg.Context, path, documentProviderModelUpdateFromPrompt(update))
	if err != nil {
		if errors.Is(err, operatorsettings.ErrProviderModelInputCanceled) ||
			errors.Is(err, context.Canceled) {
			return fmt.Errorf("configure provider/model defaults: setup canceled: %w", err)
		}
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	return reportConfiguredDefaults(cfg.Output, path, document)
}

func (service *service) applyProviderModelUpdate(
	ctx context.Context,
	path string,
	update operatorsettings.DocumentProviderModelUpdate,
) (operatorsettings.Document, error) {
	if err := operationContextError(ctx); err != nil {
		return operatorsettings.Document{}, err
	}
	loaded, err := service.root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return operatorsettings.Document{}, err
	}
	if err := operationContextError(ctx); err != nil {
		return operatorsettings.Document{}, err
	}
	request := operatorsettings.ApplyDocumentUpdateRequest{
		Path:          path,
		ProviderModel: update,
	}
	if scopeID := strings.TrimSpace(loaded.Document.BackendScopeID); scopeID != "" {
		request.ExpectedBackendScope = scopeID
	}
	result, err := service.root.ApplyDocumentUpdate(request)
	if err != nil {
		return operatorsettings.Document{}, err
	}
	if err := operationContextError(ctx); err != nil {
		return operatorsettings.Document{}, err
	}
	return result.Document, nil
}

func (service *service) acquireProviderModelPrompt(
	cfg ConfigureConfig,
	defaults operatorsettings.DocumentDefaults,
) (operatorsettings.ProviderModelUpdate, error) {
	lines, err := cfg.NewLineReader(cfg.Input, 2)
	if err != nil {
		return operatorsettings.ProviderModelUpdate{}, err
	}
	provider, err := promptValue(
		cfg.Context,
		cfg.Output,
		lines,
		"Provider",
		defaults.WorkerModelProvider,
	)
	if err != nil {
		return operatorsettings.ProviderModelUpdate{}, err
	}
	model, err := promptValue(cfg.Context, cfg.Output, lines, "Model", defaults.WorkerModel)
	if err != nil {
		return operatorsettings.ProviderModelUpdate{}, err
	}
	if strings.TrimSpace(provider) == "" {
		return operatorsettings.ProviderModelUpdate{}, fmt.Errorf("provider is required")
	}
	if strings.TrimSpace(model) == "" {
		return operatorsettings.ProviderModelUpdate{}, fmt.Errorf("model is required")
	}
	return operatorsettings.ProviderModelUpdate{
		Provider: &provider,
		Model:    &model,
	}, nil
}

func promptValue(
	ctx context.Context,
	output io.Writer,
	lines ContextLineReader,
	label string,
	defaultValue string,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(output, "%s%s: ", label, promptDefault(defaultValue)); err != nil {
		return "", fmt.Errorf("write %s prompt: %w", strings.ToLower(label), err)
	}
	line, err := lines.ReadLine(ctx)
	if err != nil {
		return "", fmt.Errorf("read %s prompt: %w", strings.ToLower(label), err)
	}
	value := strings.TrimSpace(line)
	if value == "/cancel" || value == "\x03" {
		return "", operatorsettings.ErrProviderModelInputCanceled
	}
	if value == "" {
		value = strings.TrimSpace(defaultValue)
	}
	return value, nil
}

func promptDefault(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return " [" + value + "]"
}

func reportConfiguredDefaults(
	output io.Writer,
	path string,
	document operatorsettings.Document,
) error {
	defaults := document.Defaults
	if defaults.WorkerModel == "" {
		_, err := fmt.Fprintf(output, "Configured default provider %s in %s\n", defaults.WorkerModelProvider, path)
		return err
	}
	_, err := fmt.Fprintf(
		output,
		"Configured default provider %s and model %s in %s\n",
		defaults.WorkerModelProvider,
		defaults.WorkerModel,
		path,
	)
	return err
}

func documentProviderModelUpdateFromPrompt(
	update operatorsettings.ProviderModelUpdate,
) operatorsettings.DocumentProviderModelUpdate {
	return operatorsettings.DocumentProviderModelUpdate{
		Provider: update.Provider,
		Model:    update.Model,
	}
}

func operationContextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("operator config context is required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	return nil
}
