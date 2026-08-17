package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestValidatePackageCoverageRejectsDuplicateAndUnsortedRows(t *testing.T) {
	t.Parallel()

	alpha := PackageMapping{
		PackagePath: "pkg/services/work/alpha",
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
	}
	beta := PackageMapping{
		PackagePath: "pkg/services/work/beta",
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
	}

	duplicate := UnfinishedMoves{Moves: []PackageMapping{alpha, alpha, beta}}
	if err := validatePackageCoverage(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate moves error = %v, want duplicate rejection", err)
	}

	unsorted := UnfinishedMoves{Moves: []PackageMapping{beta, alpha}}
	if err := validatePackageCoverage(unsorted); err == nil || !strings.Contains(err.Error(), "stable-sorted") {
		t.Fatalf("unsorted moves error = %v, want sort rejection", err)
	}

	sorted := UnfinishedMoves{Moves: []PackageMapping{alpha, beta}}
	if err := validatePackageCoverage(sorted); err != nil {
		t.Fatalf("sorted, duplicate-free moves error = %v", err)
	}
}

// TestValidatePackageCoverageAcceptsNoRows is the shrink-to-zero end state: the
// ledger tracks unfinished migration intent, so an empty list is success, not
// a missing-inventory failure.
func TestValidatePackageCoverageAcceptsNoRows(t *testing.T) {
	t.Parallel()

	if err := validatePackageCoverage(UnfinishedMoves{}); err != nil {
		t.Fatalf("validatePackageCoverage() on an empty row list error = %v", err)
	}
}

func TestValidateMoveLedgerRejectsInvalidDestination(t *testing.T) {
	t.Parallel()

	invalidDestination := moveLedgerFixture(PackageMapping{
		PackagePath: "pkg/services/work/alpha",
		Destination: "not_a_closed_destination",
		Successor:   "pkg/services/work/internal",
	})
	if err := validateUnfinishedMovesSchema(invalidDestination); err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("invalid destination error = %v", err)
	}
}

func TestCommittedLedgerTracksOnlyUnfinishedMigrationIntent(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	moves, err := loadUnfinishedMoves(filepath.Join(repoRoot, unfinishedMovesRelativePath))
	if err != nil {
		t.Fatalf("loadUnfinishedMoves() error = %v", err)
	}
	if err := validateManifestAt(repoRoot, manifest, moves); err != nil {
		t.Fatalf("committed manifest validateManifestAt() error = %v", err)
	}

	live, err := ownershipinventory.ListProductionPackages(repoRoot)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	if len(moves.Moves) >= len(live) {
		t.Fatalf(
			"committed move ledger still enumerates the tree: %d rows for %d live packages",
			len(moves.Moves),
			len(live),
		)
	}
}
