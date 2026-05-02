// Package config implements agent-factory config command behavior.
package config

import (
	"fmt"
	"io"
	"os"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// FactoryConfigFlattenConfig holds parameters for the config flatten command.
type FactoryConfigFlattenConfig struct {
	Path   string
	Output io.Writer
}

// FactoryConfigExpandConfig holds parameters for the config expand command.
type FactoryConfigExpandConfig struct {
	Path   string
	Output io.Writer
}

// FlattenFactoryConfig writes the canonical single-file factory config for a
// factory directory or an existing factory.json payload.
func FlattenFactoryConfig(cfg FactoryConfigFlattenConfig) error {
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	formatted, err := factoryconfig.FlattenFactoryConfig(cfg.Path)
	if err != nil {
		return err
	}

	if _, err := output.Write(formatted); err != nil {
		return fmt.Errorf("write canonical factory config: %w", err)
	}
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

	targetDir, err := factoryconfig.ExpandFactoryConfigLayout(cfg.Path)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(output, "Expanded factory config into %s\n", targetDir); err != nil {
		return fmt.Errorf("write expand result: %w", err)
	}
	return nil
}
