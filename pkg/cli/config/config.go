// Package config implements agent-factory config command behavior.
package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
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

// FlattenFactoryConfig writes the canonical single-file factory config for a
// factory directory or an existing factory.json payload.
func FlattenFactoryConfig(cfg FactoryConfigFlattenConfig) error {
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config flatten request inputPath=%s layoutSource=%s outputMode=stdout", cfg.Path, layoutSourceLabel(cfg.Path))
	formatted, err := factoryconfig.FlattenFactoryConfig(cfg.Path)
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

// ExpandFactoryConfig writes a split factory directory layout from a canonical
// factory.json file. The target directory is the input file's parent directory,
// or the provided directory when cfg.Path points at a directory.
func ExpandFactoryConfig(cfg FactoryConfigExpandConfig) error {
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "config expand request inputPath=%s outputMode=filesystem", cfg.Path)
	targetDir, report, err := factoryconfig.ExpandFactoryConfigLayoutWithExpansionReport(cfg.Path)
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

func layoutSourceLabel(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}
	if info.IsDir() {
		return "directory"
	}
	return "file"
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
