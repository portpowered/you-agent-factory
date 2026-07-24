package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidatePackageCoverageRequiresExactOneDestinationPerInventoryPath(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoPackage(t, repoRoot, "pkg/alpha", "package alpha\n")
	writeGoPackage(t, repoRoot, "pkg/beta", "package beta\n")

	base := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		FutureDebt: []FutureDebt{edgesFutureDebtEntry()},
		Inventory:  []string{"pkg/alpha", "pkg/beta"},
	}

	// Representative fixture: one deliberately unmapped inventory path fails.
	unmapped := base
	unmapped.Packages = mustResidualPackages(t, []string{"pkg/alpha"})
	err := validateManifestAt(repoRoot, unmapped)
	if err == nil {
		t.Fatal("validateManifestAt() error = nil, want missing mapping failure")
	}
	if !strings.Contains(err.Error(), "pkg/beta") {
		t.Fatalf("validateManifestAt() error = %v, want unmapped path pkg/beta", err)
	}

	duplicate := base
	duplicate.Packages = []PackageMapping{
		mustResidualPackages(t, []string{"pkg/alpha"})[0],
		mustResidualPackages(t, []string{"pkg/alpha"})[0],
		mustResidualPackages(t, []string{"pkg/beta"})[0],
	}
	if err := validatePackageCoverage(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate packages error = %v", err)
	}

	unsorted := base
	unsorted.Packages = mustResidualPackages(t, []string{"pkg/beta", "pkg/alpha"})
	if err := validatePackageCoverage(unsorted); err == nil || !strings.Contains(err.Error(), "stable-sorted") {
		t.Fatalf("unsorted packages error = %v", err)
	}

	invalidDestination := base
	invalidDestination.Packages = mustResidualPackages(t, []string{"pkg/alpha", "pkg/beta"})
	invalidDestination.Packages[0].Disposition = DispositionRetain
	invalidDestination.Packages[0].Destination = "not_a_closed_destination"
	invalidDestination.Packages[0].DeletionSuccessor = ""
	invalidDestination.Packages[0].DeletionCondition = ""
	if err := validateManifest(invalidDestination); err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("invalid destination error = %v", err)
	}

	incompleteDelete := base
	incompleteDelete.Packages = mustResidualPackages(t, []string{"pkg/alpha", "pkg/beta"})
	incompleteDelete.Packages[0].Disposition = DispositionDelete
	incompleteDelete.Packages[0].Destination = "platform"
	incompleteDelete.Packages[0].DeletionSuccessor = ""
	incompleteDelete.Packages[0].DeletionCondition = "delete when no importers remain"
	if err := validateManifest(incompleteDelete); err == nil || !strings.Contains(err.Error(), "deletionSuccessor") {
		t.Fatalf("incomplete delete error = %v", err)
	}

	// Restoring the missing mapping makes validation pass.
	complete := base
	complete.Packages = mustResidualPackages(t, []string{"pkg/alpha", "pkg/beta"})
	if err := validateManifestAt(repoRoot, complete); err != nil {
		t.Fatalf("complete coverage validateManifestAt() error = %v", err)
	}
	if err := validatePackageCoverage(complete); err != nil {
		t.Fatalf("complete coverage validatePackageCoverage() error = %v", err)
	}
}

func TestCommittedManifestHasExactOneDestinationCoverageAsLedgerSeed(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	if err := validateManifestAt(repoRoot, manifest); err != nil {
		t.Fatalf("committed manifest validateManifestAt() error = %v", err)
	}
	if err := validatePackageCoverage(manifest); err != nil {
		t.Fatalf("committed package coverage error = %v", err)
	}
	if !slices.Equal(manifest.Inventory, packagePaths(manifest.Packages)) {
		t.Fatalf("committed packages packagePath order diverges from inventory ledger seed")
	}
	if len(manifest.Inventory) == 0 {
		t.Fatal("committed inventory ledger seed is empty")
	}
}

func packagePaths(rows []PackageMapping) []string {
	paths := make([]string, len(rows))
	for i, row := range rows {
		paths[i] = row.PackagePath
	}
	return paths
}
