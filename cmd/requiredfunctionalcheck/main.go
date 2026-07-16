package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/internal/functionalscenarios"
)

const defaultManifestPath = "contracts/required-functional-scenarios.json"

type config struct {
	root         string
	manifestPath string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root containing the required functional tests")
	flag.StringVar(&cfg.manifestPath, "manifest", defaultManifestPath, "reviewed required functional scenario manifest")
	flag.Parse()
	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	manifestPath := cfg.manifestPath
	if !filepath.IsAbs(manifestPath) {
		manifestPath = filepath.Join(cfg.root, filepath.FromSlash(manifestPath))
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read required functional scenario manifest %s: %w", manifestPath, err)
	}
	manifest, err := functionalscenarios.DecodeRequiredManifest(data)
	if err != nil {
		return err
	}
	if err := functionalscenarios.CheckRequiredScenarios(cfg.root, manifest); err != nil {
		return err
	}
	boundaryReport, err := functionalscenarios.CheckFunctionalTestBoundariesReport(cfg.root)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		stdout,
		"[agent-factory:required-functional] %d required short customer-boundary scenario(s) are current; %d reviewed non-required SSE disposition(s) are explicit; the full functional tree is boundary-enforced; %d unchanged legacy file(s) remain quarantined by the reviewed migration baseline\n",
		len(manifest.Scenarios),
		len(manifest.NonRequiredScenarios),
		boundaryReport.BaselinedLegacyFiles,
	)
	return err
}
