// Package configinitcmd implements the you config init CLI command.
package configinitcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

// InitConfig holds CLI inputs for you config init.
type InitConfig struct {
	Context     context.Context
	HomeDir     string
	JSON        bool
	Output      io.Writer
	Diagnostics io.Writer
	Verbose     bool
}

// InitResult is the JSON payload for a successful you config init run.
type InitResult struct {
	HomeDir             string                      `json:"homeDir"`
	ConfigPath          string                      `json:"configPath"`
	NamedFactoriesRoot  string                      `json:"namedFactoriesRoot"`
	SystemConfigOutcome string                      `json:"systemConfigOutcome"`
	PackagedFactories   []PackagedFactoryInitResult `json:"packagedFactories"`
}

// PackagedFactoryInitResult is the JSON payload for one packaged default factory.
type PackagedFactoryInitResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDirectory"`
	Outcome    string `json:"outcome"`
}

// NewInitializer binds the CLI renderer to the injected system initialization
// service.
func NewInitializer(service systeminitialization.Service) func(InitConfig) error {
	return func(cfg InitConfig) error {
		return initialize(cfg, service)
	}
}

func initialize(cfg InitConfig, service systeminitialization.Service) error {
	if service == nil {
		return fmt.Errorf("system initialization service is required")
	}
	if cfg.Context == nil {
		return fmt.Errorf("config init context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("config init output is required")
	}
	cfg = normalizeInitConfig(cfg)

	homeDir, err := resolveInitHomeDir(cfg)
	if err != nil {
		return err
	}

	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config init request homeDir=%s", homeDir)

	result, err := service.Initialize(
		cfg.Context,
		systeminitialization.Request{HomeDir: homeDir},
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config init failed homeDir=%s", homeDir)
		return err
	}
	if err := validateInitResult(result); err != nil {
		return err
	}

	if cfg.JSON {
		return writeInitJSONResult(cfg, result)
	}
	if err := writeInitTextResult(cfg, result); err != nil {
		return err
	}

	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"config init complete homeDir=%s configPath=%s systemConfigOutcome=%s",
		result.HomeDir,
		result.ConfigPath,
		result.SystemConfigOutcome,
	)
	return nil
}

func normalizeInitConfig(cfg InitConfig) InitConfig {
	if cfg.Diagnostics == nil {
		cfg.Diagnostics = io.Discard
	}
	return cfg
}

func resolveInitHomeDir(cfg InitConfig) (string, error) {
	homeDir := strings.TrimSpace(cfg.HomeDir)
	if homeDir != "" {
		return homeDir, nil
	}
	return "", fmt.Errorf("config init home directory is required")
}

func validateInitResult(result systeminitialization.Result) error {
	switch result.SystemConfigOutcome {
	case systeminitialization.SystemConfigCreated, systeminitialization.SystemConfigSkipped:
	default:
		return fmt.Errorf("unknown system config outcome %q", result.SystemConfigOutcome)
	}

	for _, factory := range result.PackagedFactories {
		switch factory.Outcome {
		case systeminitialization.PackagedFactoryCreated, systeminitialization.PackagedFactorySkipped:
		default:
			return fmt.Errorf("unknown packaged factory outcome %q for %q", factory.Outcome, factory.Name)
		}
	}
	return nil
}

func writeInitJSONResult(cfg InitConfig, result systeminitialization.Result) error {
	return json.NewEncoder(cfg.Output).Encode(InitResult{
		HomeDir:             result.HomeDir,
		ConfigPath:          result.ConfigPath,
		NamedFactoriesRoot:  result.NamedFactoriesRoot,
		SystemConfigOutcome: string(result.SystemConfigOutcome),
		PackagedFactories:   packagedFactoryInitResults(result.PackagedFactories),
	})
}

func writeInitTextResult(cfg InitConfig, result systeminitialization.Result) error {
	if err := writeSystemConfigOutcome(cfg.Output, result); err != nil {
		return err
	}
	return writePackagedFactoryOutcomes(cfg.Output, result.PackagedFactories)
}

func writeSystemConfigOutcome(output io.Writer, result systeminitialization.Result) error {
	var message string
	switch result.SystemConfigOutcome {
	case systeminitialization.SystemConfigCreated:
		message = fmt.Sprintf("Created system config at %s\n", result.ConfigPath)
	case systeminitialization.SystemConfigSkipped:
		message = fmt.Sprintf("System config already present at %s\n", result.ConfigPath)
	default:
		return fmt.Errorf("unknown system config outcome %q", result.SystemConfigOutcome)
	}
	_, err := fmt.Fprint(output, message)
	return err
}

func writePackagedFactoryOutcomes(output io.Writer, factories []systeminitialization.PackagedFactoryResult) error {
	for _, factory := range factories {
		var message string
		switch factory.Outcome {
		case systeminitialization.PackagedFactoryCreated:
			message = fmt.Sprintf("Created packaged factory %s at %s\n", factory.Name, factory.FactoryDir)
		case systeminitialization.PackagedFactorySkipped:
			message = fmt.Sprintf("Packaged factory %s already present at %s\n", factory.Name, factory.FactoryDir)
		default:
			return fmt.Errorf("unknown packaged factory outcome %q for %q", factory.Outcome, factory.Name)
		}
		if _, err := fmt.Fprint(output, message); err != nil {
			return err
		}
	}
	return nil
}

func packagedFactoryInitResults(factories []systeminitialization.PackagedFactoryResult) []PackagedFactoryInitResult {
	if len(factories) == 0 {
		return nil
	}
	results := make([]PackagedFactoryInitResult, 0, len(factories))
	for _, factory := range factories {
		results = append(results, PackagedFactoryInitResult{
			Name:       factory.Name,
			FactoryDir: factory.FactoryDir,
			Outcome:    string(factory.Outcome),
		})
	}
	return results
}
