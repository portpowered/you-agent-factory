package splitreplacetests

import (
	"fmt"
	"io"
)

type FactoryConfigFlattenConfig struct {
	Path   string
	Output io.Writer
}

type FactoryConfigExpandConfig struct {
	Path   string
	Output io.Writer
}

func FlattenFactoryConfig(cfg FactoryConfigFlattenConfig) error {
	formatted, err := factorydefinitioncomposition.FlattenFactoryConfig(cfg.Path)
	if err != nil {
		return err
	}
	if cfg.Output == nil {
		return nil
	}
	_, err = cfg.Output.Write(formatted)
	return err
}

func ExpandFactoryConfig(cfg FactoryConfigExpandConfig) error {
	targetDir, report, err := factorydefinitioncomposition.ExpandLayout(cfg.Path)
	if err != nil {
		return err
	}
	if cfg.Output == nil {
		return nil
	}
	if _, err := fmt.Fprintf(
		cfg.Output,
		"Expanded factory config into %s\n",
		targetDir,
	); err != nil {
		return err
	}
	for _, replacement := range report.BundledReplacements {
		if _, err := fmt.Fprintf(
			cfg.Output,
			"Replaced existing portable bundled file at %s\n",
			replacement.TargetPath,
		); err != nil {
			return err
		}
	}
	return nil
}
