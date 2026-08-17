package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePackageCoverageRejectsDuplicateAndUnsortedRows(t *testing.T) {
	t.Parallel()

	alpha := PackageMapping{
		PackagePath: "pkg/services/work/alpha",
		Disposition: DispositionMove,
		Destination: "work/internal",
	}
	beta := PackageMapping{
		PackagePath: "pkg/services/work/beta",
		Disposition: DispositionMove,
		Destination: "work/internal",
	}

	duplicate := Manifest{Packages: []PackageMapping{alpha, alpha, beta}}
	if err := validatePackageCoverage(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate packages error = %v, want duplicate rejection", err)
	}

	unsorted := Manifest{Packages: []PackageMapping{beta, alpha}}
	if err := validatePackageCoverage(unsorted); err == nil || !strings.Contains(err.Error(), "stable-sorted") {
		t.Fatalf("unsorted packages error = %v, want sort rejection", err)
	}

	sorted := Manifest{Packages: []PackageMapping{alpha, beta}}
	if err := validatePackageCoverage(sorted); err != nil {
		t.Fatalf("sorted, duplicate-free packages error = %v", err)
	}
}

// TestValidatePackageCoverageAcceptsNoRows is the shrink-to-zero end state: the
// manifest tracks unfinished migration intent, so an empty list is success, not
// a missing-inventory failure.
func TestValidatePackageCoverageAcceptsNoRows(t *testing.T) {
	t.Parallel()

	if err := validatePackageCoverage(Manifest{}); err != nil {
		t.Fatalf("validatePackageCoverage() on an empty row list error = %v", err)
	}
}

func TestValidateManifestRejectsInvalidDestinationAndIncompleteDelete(t *testing.T) {
	t.Parallel()

	base := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		FutureDebt: []FutureDebt{edgesFutureDebtEntry()},
	}

	invalidDestination := base
	invalidDestination.Packages = []PackageMapping{{
		PackagePath: "pkg/services/work/alpha",
		Disposition: DispositionMove,
		Destination: "not_a_closed_destination",
	}}
	if err := validateManifest(invalidDestination); err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("invalid destination error = %v", err)
	}

	incompleteDelete := base
	incompleteDelete.Packages = []PackageMapping{{
		PackagePath:       "pkg/services/work/alpha",
		Disposition:       DispositionDelete,
		Destination:       "platform",
		DeletionCondition: "delete when no importers remain",
	}}
	if err := validateManifest(incompleteDelete); err == nil || !strings.Contains(err.Error(), "deletionSuccessor") {
		t.Fatalf("incomplete delete error = %v", err)
	}
}

func TestCommittedManifestTracksOnlyUnfinishedMigrationIntent(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	if err := validateManifestAt(repoRoot, manifest); err != nil {
		t.Fatalf("committed manifest validateManifestAt() error = %v", err)
	}

	live, err := listProductionPkgPackages(repoRoot)
	if err != nil {
		t.Fatalf("listProductionPkgPackages() error = %v", err)
	}
	if len(manifest.Packages) >= len(live) {
		t.Fatalf(
			"committed manifest still enumerates the tree: %d rows for %d live packages",
			len(manifest.Packages),
			len(live),
		)
	}
	for _, row := range manifest.Packages {
		if row.Disposition == DispositionRetain {
			t.Fatalf("committed manifest still carries a derivable retain row for %q", row.PackagePath)
		}
	}
}
