package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

type config struct {
	repositoryRoot      string
	functionalRoot      string
	coverageSummaryPath string
	outputPath          string
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.repositoryRoot, "root", ".", "repository root used to resolve golden manifests and default paths")
	flag.StringVar(&cfg.functionalRoot, "functional-root", "", "functional test tree to inventory (default: <root>/tests/functional)")
	flag.StringVar(&cfg.coverageSummaryPath, "coverage-summary", "", "path to gocoveragecheck coverage-summary JSON (required)")
	flag.StringVar(&cfg.outputPath, "output", "", "Markdown output path (default: <root>/.artifacts/functional-test-viz/functional-tests.md)")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout, stderr io.Writer) error {
	generateCfg := functionaltestviz.GenerateConfig{
		RepositoryRoot:      cfg.repositoryRoot,
		FunctionalRoot:      cfg.functionalRoot,
		CoverageSummaryPath: cfg.coverageSummaryPath,
		OutputPath:          cfg.outputPath,
	}
	if err := functionaltestviz.Generate(generateCfg); err != nil {
		return err
	}
	outputPath := cfg.outputPath
	if outputPath == "" {
		root := cfg.repositoryRoot
		if root == "" {
			root = "."
		}
		outputPath = filepath.Join(root, filepath.FromSlash(functionaltestviz.DefaultOutputPath))
	}
	_, _ = fmt.Fprintf(stderr, "[agent-factory:functional-test-viz] wrote catalog to %s\n", outputPath)
	_, _ = fmt.Fprintln(stdout, "ok")
	return nil
}
