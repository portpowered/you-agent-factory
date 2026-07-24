// Command packagetargetmanifestcheck validates the Packaged Service Structure
// package-to-target and deletion manifest schema, closed destination vocabulary,
// and stable-sorted production pkg inventory ledger seed.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type config struct {
	root           string
	manifestPath   string
	writeInventory bool
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root containing the package-target manifest")
	flag.StringVar(&cfg.manifestPath, "manifest", manifestRelativePath, "repository-relative path to the package-target manifest")
	flag.BoolVar(&cfg.writeInventory, "write-inventory", false, "rewrite the committed inventory from the live production pkg tree")
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
	if cfg.writeInventory {
		inventory, listErr := listProductionPkgPackages(repoRoot)
		if listErr != nil {
			return listErr
		}
		manifest.Inventory = inventory
		if writeErr := writeManifest(manifestFile, manifest); writeErr != nil {
			return writeErr
		}
		fmt.Fprintf(stdout, "[agent-factory:package-target-manifest] wrote %d inventory rows to %s\n", len(inventory), filepath.ToSlash(manifestFile))
	}
	if err := validateManifestAt(repoRoot, manifest); err != nil {
		return fmt.Errorf("[agent-factory:package-target-manifest] %w", err)
	}
	fmt.Fprintf(
		stdout,
		"[agent-factory:package-target-manifest] inventory and destination vocabulary hold (%d inventory rows, %d package rows)\n",
		len(manifest.Inventory),
		len(manifest.Packages),
	)
	return nil
}

func writeManifest(path string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	return nil
}
