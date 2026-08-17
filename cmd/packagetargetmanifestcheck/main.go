// Command packagetargetmanifestcheck validates the Packaged Service Structure
// migration contract: the package-target manifest's schema and closed
// destination vocabulary, plus the consolidated open-move ledger's schema and
// the requirement that every remaining move row still names a package that
// exists.
//
// The ledger records only unfinished migration intent. A package that stays
// where it already lives derives its destination from its own path and carries
// no row, so package churn inside a service requires no registry edit.
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
	movesPath    string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root containing the package-target manifest")
	flag.StringVar(&cfg.manifestPath, "manifest", manifestRelativePath, "repository-relative path to the package-target manifest")
	flag.StringVar(&cfg.movesPath, "moves", unfinishedMovesRelativePath, "repository-relative path to the consolidated unfinished-package-move ledger")
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
	movesFile := resolveManifestPath(repoRoot, cfg.movesPath)
	moves, err := loadUnfinishedMoves(movesFile)
	if err != nil {
		return err
	}
	if err := validateManifestAt(repoRoot, manifest, moves); err != nil {
		return fmt.Errorf("[agent-factory:package-target-manifest] %w", err)
	}
	fmt.Fprintf(
		stdout,
		"[agent-factory:package-target-manifest] destination vocabulary holds (%s) and all %d open migration row(s) name live packages (%s)\n",
		filepath.ToSlash(manifestFile),
		len(moves.Moves),
		filepath.ToSlash(movesFile),
	)
	return nil
}
