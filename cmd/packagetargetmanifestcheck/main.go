// Command packagetargetmanifestcheck validates the Packaged Service Structure
// migration contract: the consolidated open-move ledger's schema, its closed
// destination vocabulary, and the requirement that every remaining move row
// still names a package that exists.
//
// The ledger records only unfinished migration intent. A package that stays
// where it already lives derives its destination from its own path and carries
// no row, so package churn inside a service requires no registry edit. The
// package-target manifest document this command was named for held no
// ratcheting content once its package rows moved into that ledger; the
// destination vocabulary and architecture-exception rationale it also carried
// are published at docs/architecture/service-ownership-rationale.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	root      string
	movesPath string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root containing the open-move ledger")
	flag.StringVar(&cfg.movesPath, "moves", unfinishedMovesRelativePath, "repository-relative path to the consolidated unfinished-package-move ledger")
	flag.Parse()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout, stderr io.Writer) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	movesPath := cfg.movesPath
	if movesPath == "" {
		movesPath = unfinishedMovesRelativePath
	}
	movesFile := resolveRepoPath(repoRoot, movesPath)
	moves, err := loadUnfinishedMoves(movesFile)
	if err != nil {
		return err
	}
	if err := validateOpenMoveLedgerSchema(repoRoot, moves); err != nil {
		return fmt.Errorf("[agent-factory:package-target-manifest] %w", err)
	}
	findings, err := scanPackageTargetFindings(repoRoot, moves.Moves)
	if err != nil {
		return err
	}
	productionStale := packageTargetProductionStaleRows(moves.Moves, findings)
	if len(productionStale) > 0 {
		writePackageTargetObservationCounts(stderr, findings)
		writePackageTargetTestOnlyObservations(stderr, findings)
		writePackageTargetViolationCountsForFindings(stderr, productionStale, findings)
		for _, row := range productionStale {
			writeStaleProductionPackageTargetRow(stderr, row)
		}
		return fmt.Errorf(
			"[agent-factory:package-target-manifest] found %d stale production row(s) [%s]\nLINT_VIOLATION_COUNT: %d",
			len(productionStale),
			strings.Join(packageTargetStaleRowPaths(productionStale), ", "),
			len(productionStale),
		)
	}
	writePackageTargetObservationCounts(stdout, findings)
	writePackageTargetTestOnlyObservations(stdout, findings)
	writePackageTargetViolationCountsForFindings(stdout, productionStale, findings)
	fmt.Fprintf(
		stdout,
		"[agent-factory:package-target-manifest] all %d open migration row(s) hold the closed destination vocabulary and name live packages (%s)\n",
		len(moves.Moves),
		filepath.ToSlash(movesFile),
	)
	return nil
}
