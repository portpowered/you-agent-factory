// Command packagetargetmanifestcheck validates the Packaged Service Structure
// package-to-target and deletion manifest schema and closed destination vocabulary.
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
	if err := validateManifest(manifest); err != nil {
		return fmt.Errorf("[agent-factory:package-target-manifest] %w", err)
	}
	fmt.Fprintf(
		stdout,
		"[agent-factory:package-target-manifest] destination vocabulary and row schema hold (%d package rows)\n",
		len(manifest.Packages),
	)
	return nil
}
