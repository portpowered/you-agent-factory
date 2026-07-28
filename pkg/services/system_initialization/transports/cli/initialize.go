package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// InitializeConfig carries resolved CLI inputs for one Bootstrap initialize
// invocation.
type InitializeConfig struct {
	Context     context.Context
	HomeDir     string
	JSON        bool
	Verbose     bool
	Output      io.Writer
	Diagnostics io.Writer
}

// InitializeResult is the structured success payload for Bootstrap initialize
// presentation.
type InitializeResult struct {
	HomeDir             string                         `json:"homeDir"`
	ConfigPath          string                         `json:"configPath"`
	NamedFactoriesRoot  string                         `json:"namedFactoriesRoot"`
	SystemConfigOutcome string                         `json:"systemConfigOutcome"`
	PackagedFactories   []InitializePackagedFactory    `json:"packagedFactories"`
}

// InitializePackagedFactory summarizes one packaged Factory outcome for CLI
// presentation.
type InitializePackagedFactory struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDir"`
	Outcome    string `json:"outcome"`
}

// Initialize delegates home-directory intent to the Bootstrap root and surfaces
// typed results, validation/cancellation failures, and partial-failure rollback
// facts for CLI consumption.
func Initialize(
	cfg InitializeConfig,
	root systeminitialization.Service,
) (systeminitialization.Result, error) {
	adapter := New(root)
	if adapter == nil {
		return systeminitialization.Result{}, fmt.Errorf("system bootstrap service is required")
	}
	return adapter.Initialize(cfg)
}

// InitializeSystem is a composition-stable facade over the owned Service for
// callers that only need exit-relevant success or failure.
func InitializeSystem(
	ctx context.Context,
	homeDir string,
	root systeminitialization.Service,
) error {
	_, err := Initialize(InitializeConfig{
		Context: ctx,
		HomeDir: homeDir,
	}, root)
	return err
}

func (service *service) Initialize(
	cfg InitializeConfig,
) (systeminitialization.Result, error) {
	if cfg.Context == nil {
		return systeminitialization.Result{}, fmt.Errorf("context is required")
	}
	diagnosticPrintf(
		cfg.Diagnostics,
		cfg.Verbose,
		"initialize system request homeDir=%q",
		strings.TrimSpace(cfg.HomeDir),
	)
	result, err := service.root.Initialize(
		cfg.Context,
		systeminitialization.Request{HomeDir: cfg.HomeDir},
	)
	if err != nil {
		diagnosticPrintf(
			cfg.Diagnostics,
			cfg.Verbose,
			"initialize system failed homeDir=%q err=%v",
			strings.TrimSpace(cfg.HomeDir),
			err,
		)
		return systeminitialization.Result{}, renderInitializeError(err)
	}
	diagnosticPrintf(
		cfg.Diagnostics,
		cfg.Verbose,
		"initialize system complete homeDir=%q systemConfigOutcome=%s packagedFactories=%d",
		result.HomeDir,
		result.SystemConfigOutcome,
		len(result.PackagedFactories),
	)
	if cfg.Output != nil {
		if renderErr := renderInitializeSuccess(cfg, result); renderErr != nil {
			return result, renderErr
		}
	}
	return result, nil
}

func renderInitializeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, systeminitialization.ErrMissingHomeDir) ||
		errors.Is(err, systeminitialization.ErrInitializeCancelled) ||
		errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
		return err
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if errors.As(err, &partialFailure) {
		return err
	}
	return fmt.Errorf("initialize system: %w", err)
}

func renderInitializeSuccess(
	cfg InitializeConfig,
	result systeminitialization.Result,
) error {
	payload := initializeResultPayload(result)
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(payload)
	}
	for _, factory := range payload.PackagedFactories {
		switch factory.Outcome {
		case string(systeminitialization.PackagedFactorySkipped):
			if _, err := fmt.Fprintf(
				cfg.Output,
				"Packaged factory %s is already installed at %s\n",
				factory.Name,
				factory.FactoryDir,
			); err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintf(
				cfg.Output,
				"Installed packaged factory %s\nDirectory: %s\n",
				factory.Name,
				factory.FactoryDir,
			); err != nil {
				return err
			}
		}
	}
	switch result.SystemConfigOutcome {
	case systeminitialization.SystemConfigSkipped:
		_, err := fmt.Fprintf(
			cfg.Output,
			"Operator config already present at %s\n",
			result.ConfigPath,
		)
		return err
	default:
		_, err := fmt.Fprintf(
			cfg.Output,
			"Initialized operator config at %s\nNamed factories root: %s\n",
			result.ConfigPath,
			result.NamedFactoriesRoot,
		)
		return err
	}
}

func initializeResultPayload(result systeminitialization.Result) InitializeResult {
	factories := make([]InitializePackagedFactory, 0, len(result.PackagedFactories))
	for _, factory := range result.PackagedFactories {
		factories = append(factories, InitializePackagedFactory{
			Name:       factory.Name,
			FactoryDir: factory.FactoryDir,
			Outcome:    string(factory.Outcome),
		})
	}
	return InitializeResult{
		HomeDir:             result.HomeDir,
		ConfigPath:          result.ConfigPath,
		NamedFactoriesRoot:  result.NamedFactoriesRoot,
		SystemConfigOutcome: string(result.SystemConfigOutcome),
		PackagedFactories:   factories,
	}
}

func diagnosticPrintf(output io.Writer, enabled bool, format string, args ...any) {
	if !enabled || output == nil {
		return
	}
	_, _ = fmt.Fprintf(output, format+"\n", args...)
}
