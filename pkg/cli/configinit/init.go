// Package configinitcmd implements the you config init CLI command.
package configinitcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	configinit "github.com/portpowered/infinite-you/pkg/config/configinit"
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
	HomeDir             string `json:"homeDir"`
	ConfigPath          string `json:"configPath"`
	SystemConfigOutcome string `json:"systemConfigOutcome"`
}

// Init runs the canonical system initializer for the configured home directory.
func Init(cfg InitConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if cfg.Diagnostics == nil {
		cfg.Diagnostics = os.Stderr
	}

	homeDir := strings.TrimSpace(cfg.HomeDir)
	if homeDir == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config init failed phase=resolve-home")
			return fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolved
	}

	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config init request homeDir=%s", homeDir)

	result, err := configinit.Init(homeDir)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config init failed homeDir=%s", homeDir)
		return err
	}

	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(InitResult{
			HomeDir:             result.HomeDir,
			ConfigPath:          result.ConfigPath,
			SystemConfigOutcome: string(result.SystemConfigOutcome),
		})
	}

	switch result.SystemConfigOutcome {
	case configinit.SystemConfigCreated:
		if _, err := fmt.Fprintf(cfg.Output, "Created system config at %s\n", result.ConfigPath); err != nil {
			return err
		}
	case configinit.SystemConfigSkipped:
		if _, err := fmt.Fprintf(cfg.Output, "System config already present at %s\n", result.ConfigPath); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(cfg.Output, "Config init completed for %s\n", result.ConfigPath); err != nil {
			return err
		}
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
