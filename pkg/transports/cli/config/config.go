// Package config implements agent-factory config command behavior.
package config

import (
	"fmt"
	"io"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

// FactoryConfigFlattenConfig holds parameters for the config flatten command.
type FactoryConfigFlattenConfig struct {
	Path        string
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// FactoryConfigExpandConfig holds parameters for the config expand command.
type FactoryConfigExpandConfig struct {
	Path        string
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// NewFlattenFactoryConfig binds the Factory Definitions layout capability to
// the CLI representation handler.
func NewFlattenFactoryConfig(
	persistence factorydefinitions.Persistence,
) func(FactoryConfigFlattenConfig) error {
	return func(cfg FactoryConfigFlattenConfig) error {
		return flattenFactoryConfig(persistence, cfg)
	}
}

func flattenFactoryConfig(
	persistence factorydefinitions.Persistence,
	cfg FactoryConfigFlattenConfig,
) error {
	output := cfg.Output
	if output == nil {
		return fmt.Errorf("config flatten output is required")
	}
	if persistence == nil {
		return fmt.Errorf("Factory Definitions persistence service is required")
	}

	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config flatten request inputPath=%s outputMode=stdout", cfg.Path)
	formatted, err := persistence.FlattenFactoryLayout(cfg.Path)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config flatten failed inputPath=%s phase=%s", cfg.Path, configFailurePhase(err))
		return err
	}

	if _, err := output.Write(formatted); err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config flatten failed inputPath=%s phase=write-output", cfg.Path)
		return fmt.Errorf("write canonical factory config: %w", err)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config flatten complete inputPath=%s outputMode=stdout outputBytes=%d", cfg.Path, len(formatted))
	return nil
}

// NewExpandFactoryConfig binds the Factory Definitions layout capability to
// the CLI representation handler.
func NewExpandFactoryConfig(
	persistence factorydefinitions.Persistence,
) func(FactoryConfigExpandConfig) error {
	return func(cfg FactoryConfigExpandConfig) error {
		return expandFactoryConfig(persistence, cfg)
	}
}

func expandFactoryConfig(
	persistence factorydefinitions.Persistence,
	cfg FactoryConfigExpandConfig,
) error {
	output := cfg.Output
	if output == nil {
		return fmt.Errorf("config expand output is required")
	}
	if persistence == nil {
		return fmt.Errorf("Factory Definitions persistence service is required")
	}

	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config expand request inputPath=%s outputMode=filesystem", cfg.Path)
	targetDir, report, err := persistence.ExpandFactoryLayout(cfg.Path)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config expand failed inputPath=%s phase=%s", cfg.Path, configFailurePhase(err))
		return err
	}

	if _, err := fmt.Fprintf(output, "Expanded factory config into %s\n", targetDir); err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config expand failed inputPath=%s outputDir=%s phase=write-output", cfg.Path, targetDir)
		return fmt.Errorf("write expand result: %w", err)
	}
	for _, replacement := range report.BundledReplacements {
		if _, err := fmt.Fprintf(output, "Replaced existing portable bundled file at %s\n", replacement.TargetPath); err != nil {
			clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config expand failed inputPath=%s outputDir=%s phase=write-output", cfg.Path, targetDir)
			return fmt.Errorf("write expand replacement report: %w", err)
		}
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"config expand complete inputPath=%s outputDir=%s writtenFactoryConfigs=%d writtenWorkerAgents=%d writtenWorkstationAgents=%d writtenPromptFiles=%d replacedBundledFiles=%d",
		cfg.Path,
		targetDir,
		report.FactoryConfigPaths,
		report.WorkerAgentPaths,
		report.WorkstationAgentPaths,
		report.PromptPaths,
		len(report.BundledReplacements),
	)
	return nil
}

func configFailurePhase(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "parse factory config"), strings.Contains(message, "decode factory"):
		return "parse"
	case strings.Contains(message, "validate"), strings.Contains(message, "validation"), strings.Contains(message, "not supported"):
		return "validation"
	case strings.Contains(message, "read "), strings.Contains(message, "find factory config"):
		return "read"
	case strings.Contains(message, "write "), strings.Contains(message, "create "):
		return "write"
	default:
		return "process"
	}
}
