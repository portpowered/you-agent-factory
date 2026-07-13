// Package configinitcmd implements the you config init CLI command.
package configinitcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	configinit "github.com/portpowered/infinite-you/pkg/config/configinit"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

// InitConfig holds CLI inputs for you config init.
type InitConfig struct {
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

// Init runs the canonical system initializer for the configured home directory.
func Init(cfg InitConfig) error {
	cfg = normalizeInitConfig(cfg)

	homeDir, err := resolveInitHomeDir(cfg)
	if err != nil {
		return err
	}

	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config init request homeDir=%s", homeDir)

	result, err := configinit.Init(homeDir)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config init failed homeDir=%s", homeDir)
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
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if cfg.Diagnostics == nil {
		cfg.Diagnostics = os.Stderr
	}
	return cfg
}

func resolveInitHomeDir(cfg InitConfig) (string, error) {
	homeDir := strings.TrimSpace(cfg.HomeDir)
	if homeDir != "" {
		return homeDir, nil
	}
	resolved, err := os.UserHomeDir()
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config init failed phase=resolve-home")
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return resolved, nil
}

func writeInitJSONResult(cfg InitConfig, result configinit.Result) error {
	return json.NewEncoder(cfg.Output).Encode(InitResult{
		HomeDir:             result.HomeDir,
		ConfigPath:          result.ConfigPath,
		NamedFactoriesRoot:  result.NamedFactoriesRoot,
		SystemConfigOutcome: string(result.SystemConfigOutcome),
		PackagedFactories:   packagedFactoryInitResults(result.PackagedFactories),
	})
}

func writeInitTextResult(cfg InitConfig, result configinit.Result) error {
	if err := writeSystemConfigOutcome(cfg.Output, result); err != nil {
		return err
	}
	return writePackagedFactoryOutcomes(cfg.Output, result.PackagedFactories)
}

func writeSystemConfigOutcome(output io.Writer, result configinit.Result) error {
	var message string
	switch result.SystemConfigOutcome {
	case configinit.SystemConfigCreated:
		message = fmt.Sprintf("Created system config at %s\n", result.ConfigPath)
	case configinit.SystemConfigSkipped:
		message = fmt.Sprintf("System config already present at %s\n", result.ConfigPath)
	default:
		message = fmt.Sprintf("Config init completed for %s\n", result.ConfigPath)
	}
	_, err := fmt.Fprint(output, message)
	return err
}

func writePackagedFactoryOutcomes(output io.Writer, factories []configinit.PackagedFactoryResult) error {
	for _, factory := range factories {
		var message string
		switch factory.Outcome {
		case configinit.PackagedFactoryCreated:
			message = fmt.Sprintf("Created packaged factory %s at %s\n", factory.Name, factory.FactoryDir)
		case configinit.PackagedFactorySkipped:
			message = fmt.Sprintf("Packaged factory %s already present at %s\n", factory.Name, factory.FactoryDir)
		default:
			continue
		}
		if _, err := fmt.Fprint(output, message); err != nil {
			return err
		}
	}
	return nil
}

func packagedFactoryInitResults(factories []configinit.PackagedFactoryResult) []PackagedFactoryInitResult {
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
