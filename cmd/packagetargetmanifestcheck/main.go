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
	root                   string
	movesPath              string
	testOnlyBaselinePath   string
	createTestOnlyBaseline bool
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root containing the open-move ledger")
	flag.StringVar(&cfg.movesPath, "moves", unfinishedMovesRelativePath, "repository-relative path to the consolidated unfinished-package-move ledger")
	flag.StringVar(&cfg.testOnlyBaselinePath, "test-only-baseline", packageTargetTestOnlyBaselineRelativePath, "repository-relative path to the exact test-only package-target baseline")
	flag.BoolVar(&cfg.createTestOnlyBaseline, "create-test-only-baseline", false, "create the deletion-only test-only package-target baseline; fails if the file already exists")
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
	testOnlyBaselinePath := cfg.testOnlyBaselinePath
	if testOnlyBaselinePath == "" {
		testOnlyBaselinePath = packageTargetTestOnlyBaselineRelativePath
	}

	findings, err := scanPackageTargetFindings(repoRoot, moves.Moves)
	if err != nil {
		return err
	}
	if cfg.createTestOnlyBaseline {
		return createPackageTargetTestOnlyBaseline(
			resolveRepoPath(repoRoot, testOnlyBaselinePath),
			findings,
			stdout,
		)
	}
	baselinePath := resolveRepoPath(repoRoot, testOnlyBaselinePath)
	baseline, err := loadPackageTargetTestOnlyBaseline(baselinePath)
	if err != nil {
		return err
	}
	productionStale, testOnlyUnrecorded, testOnlyStale, err := partitionPackageTargetFindings(findings, moves.Moves, baseline)
	if err != nil {
		return err
	}
	if len(productionStale) > 0 || len(testOnlyUnrecorded) > 0 || len(testOnlyStale) > 0 {
		writePackageTargetObservationCounts(stderr, findings)
		writePackageTargetViolationCounts(stderr, productionStale, testOnlyUnrecorded, testOnlyStale)
		for _, row := range productionStale {
			writeStaleProductionPackageTargetRow(stderr, row)
		}
		for _, finding := range testOnlyUnrecorded {
			writePackageTargetFinding(stderr, "new test-only package-target observation", finding)
		}
		for _, entry := range testOnlyStale {
			writeStalePackageTargetTestOnlyBaselineEntry(stderr, entry, filepath.ToSlash(testOnlyBaselinePath))
		}
		return fmt.Errorf(
			"[agent-factory:package-target-manifest] found %d stale production row(s) [%s], %d new test-only observation(s), and %d stale test-only baseline entry/entries\nLINT_VIOLATION_COUNT: %d",
			len(productionStale),
			strings.Join(packageTargetStaleRowPaths(productionStale), ", "),
			len(testOnlyUnrecorded),
			len(testOnlyStale),
			len(productionStale)+len(testOnlyUnrecorded)+len(testOnlyStale),
		)
	}
	writePackageTargetObservationCounts(stdout, findings)
	writePackageTargetViolationCounts(stdout, productionStale, testOnlyUnrecorded, testOnlyStale)
	fmt.Fprintf(
		stdout,
		"[agent-factory:package-target-manifest] all %d open migration row(s) hold the closed destination vocabulary and name live packages (%s); test-only baseline=%d exact deletion-only edge(s) (%s)\n",
		len(moves.Moves),
		filepath.ToSlash(movesFile),
		len(baseline.Entries),
		filepath.ToSlash(baselinePath),
	)
	return nil
}
