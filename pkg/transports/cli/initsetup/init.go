// Package initsetup adapts the public init command to atomic operator settings.
package initsetup

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

// Config carries supplied provider/model setup inputs from the CLI boundary.
type Config struct {
	Context  context.Context
	HomeDir  string
	Provider string
	Model    *string
	Input    io.Reader
	Output   io.Writer
	// Interactive is true only when the process classified both stdin and
	// stdout as terminals. Supplied provider input never requires it.
	Interactive bool
}

// NewConfigurer binds CLI setup rendering to the atomic operator-config service.
func NewConfigurer(
	service operatorsettings.ConfigDocumentService,
	newLineReader ContextLineReaderFactory,
) func(Config) error {
	return func(cfg Config) error {
		return configure(cfg, service, newLineReader)
	}
}

func configure(
	cfg Config,
	service operatorsettings.ConfigDocumentService,
	newLineReader ContextLineReaderFactory,
) error {
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
		return configurePrompted(cfg, service, newLineReader, homeDir)
	}
	var model *string
	if cfg.Model != nil {
		value := strings.TrimSpace(*cfg.Model)
		if value == "" {
			return fmt.Errorf("model must be non-empty when supplied")
		}
		model = &value
	}

	path := operatorsettings.DefaultConfigPath(homeDir)
	document, err := service.ConfigureProviderModel(
		cfg.Context,
		path,
		operatorsettings.ProviderModelUpdate{Provider: &provider, Model: model},
	)
	if err != nil {
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	return reportConfiguredDefaults(cfg.Output, path, document)
}

func configurePrompted(
	cfg Config,
	service operatorsettings.ConfigDocumentService,
	newLineReader ContextLineReaderFactory,
	homeDir string,
) error {
	if newLineReader == nil {
		return fmt.Errorf("interactive init line reader is required")
	}
	path := operatorsettings.DefaultConfigPath(homeDir)
	document, err := service.ConfigureProviderModelPrompted(
		cfg.Context,
		path,
		newProviderModelPrompt(cfg.Input, cfg.Output, newLineReader),
	)
	if err != nil {
		if errors.Is(err, operatorsettings.ErrProviderModelInputCanceled) ||
			errors.Is(err, context.Canceled) {
			return fmt.Errorf("configure provider/model defaults: setup canceled: %w", err)
		}
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	return reportConfiguredDefaults(cfg.Output, path, document)
}

func newProviderModelPrompt(
	input io.Reader,
	output io.Writer,
	newLineReader ContextLineReaderFactory,
) operatorsettings.ProviderModelPrompt {
	return func(
		ctx context.Context,
		defaults operatorsettings.Defaults,
	) (operatorsettings.ProviderModelUpdate, error) {
		lines, err := newLineReader(input, 2)
		if err != nil {
			return operatorsettings.ProviderModelUpdate{}, err
		}
		provider, err := promptValue(
			ctx,
			output,
			lines,
			"Provider",
			defaults.WorkerModelProvider,
		)
		if err != nil {
			return operatorsettings.ProviderModelUpdate{}, err
		}
		model, err := promptValue(ctx, output, lines, "Model", defaults.WorkerModel)
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
	document operatorsettings.ConfigDocument,
) error {
	defaults := document.FileConfig().Defaults
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
