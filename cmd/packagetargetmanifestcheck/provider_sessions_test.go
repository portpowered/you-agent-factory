package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const deletedProviderSessionsServicePackagePath = "pkg/services/provider_sessions/service"

func TestVerifyProviderSessionsDualLedgerAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsDualLedgerAlignment(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsDualLedgerAlignment() error = %v", err)
	}
}

func TestVerifyProviderSessionsTopLevelInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsTopLevelInventory(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsTopLevelInventory() error = %v", err)
	}
}

func TestVerifyProviderSessionsZeroExtraPublicSiblingAbsencePassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsZeroExtraPublicSiblingAbsence(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsZeroExtraPublicSiblingAbsence() error = %v", err)
	}
}

func TestVerifyProviderSessionsINVDispositionBeyondServicePassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsINVDispositionBeyondService(repoRoot); err != nil {
		t.Fatalf("VerifyProviderSessionsINVDispositionBeyondService() error = %v", err)
	}
}

func TestProviderSessionsCommittedBaselinesAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	ownership, err := ownershipinventory.Load(repoRoot)
	if err != nil {
		t.Fatalf("ownershipinventory.Load() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(ownership.Packages))
	for _, row := range ownership.Packages {
		ownershipByPath[row.PackagePath] = row
	}

	const ownerPrefix = "pkg/services/provider_sessions/"
	for _, row := range manifest.Packages {
		if row.PackagePath != "pkg/services/provider_sessions" && !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		if row.Disposition != DispositionMove {
			continue
		}
		ownershipRow, ok := ownershipByPath[row.PackagePath]
		if !ok {
			t.Fatalf("ownership inventory missing committed manifest move row %q", row.PackagePath)
		}
		wantSuccessor := "pkg/services/" + row.Destination
		if ownershipRow.Successor != wantSuccessor {
			t.Fatalf("dual-ledger drift for %q: manifest destination %q => successor %q, ownership has %q",
				row.PackagePath, row.Destination, wantSuccessor, ownershipRow.Successor)
		}
	}
}

func TestCommittedManifestProviderSessionsRejectsRetainToOwnerRootForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	manifestByPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		manifestByPath[row.PackagePath] = row
	}

	for _, packagePath := range ownershipinventory.ProviderSessionsUnexpectedPublicSiblingPackagePaths(topLevel) {
		row, ok := manifestByPath[packagePath]
		if !ok {
			t.Fatalf("committed manifest missing unexpected public sibling %q", packagePath)
		}
		if row.Disposition == DispositionRetain && row.Destination == "provider_sessions" {
			t.Fatalf("committed manifest row retain→provider_sessions for %q", packagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Destination == "provider_sessions" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", packagePath)
		}
	}
}

func manifestPackageRow(manifest Manifest, packagePath string) (PackageMapping, bool) {
	for _, row := range manifest.Packages {
		if row.PackagePath == packagePath {
			return row, true
		}
	}
	return PackageMapping{}, false
}

func ownershipPackageRow(inventory ownershipinventory.Inventory, packagePath string) (ownershipinventory.PackageRow, bool) {
	for _, row := range inventory.Packages {
		if row.PackagePath == packagePath {
			return row, true
		}
	}
	return ownershipinventory.PackageRow{}, false
}
