// Command packagetargetmanifestcheck validates the Packaged Service Structure
// migration manifest: its schema, its closed destination vocabulary, and that
// every committed row still names a package that exists.
//
// The manifest records only unfinished migration intent. A package that stays
// where it already lives derives its destination from its own path and carries
// no row, so package churn inside a service requires no manifest edit.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type config struct {
	root         string
	manifestPath string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root containing the package-target manifest")
	flag.StringVar(&cfg.manifestPath, "manifest", manifestRelativePath, "repository-relative path to the package-target manifest")
	flag.Parse()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout, _ io.Writer) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	manifestFile := resolveManifestPath(repoRoot, cfg.manifestPath)
	manifest, err := loadManifest(manifestFile)
	if err != nil {
		return err
	}
	if err := validateManifestAt(repoRoot, manifest); err != nil {
		return fmt.Errorf("[agent-factory:package-target-manifest] %w", err)
	}
	fmt.Fprintf(
		stdout,
		"[agent-factory:package-target-manifest] destination vocabulary holds and all %d open migration row(s) name live packages (%s)\n",
		len(manifest.Packages),
		filepath.ToSlash(manifestFile),
	)
	return nil
}
