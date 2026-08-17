package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestCommittedManifestOperatorSettingsRejectsRetainToOwnerRootForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	manifestByPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		manifestByPath[row.PackagePath] = row
	}

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		row, ok := manifestByPath[packagePath]
		if !ok {
			t.Fatalf("committed manifest missing unexpected public sibling %q", packagePath)
		}
		if row.Disposition == DispositionRetain && row.Destination == "operator_settings" {
			t.Fatalf("committed manifest row retain→operator_settings for %q", packagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Destination == "operator_settings" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", packagePath)
		}
	}
}

func TestVerifyOperatorSettingsDualLedgerAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsDualLedgerAlignment(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsDualLedgerAlignment() error = %v", err)
	}
}

func TestVerifyOperatorSettingsTopLevelInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsTopLevelInventory(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsTopLevelInventory() error = %v", err)
	}
}

func TestVerifyOperatorSettingsRootGoInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsRootGoInventory(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsRootGoInventory() error = %v", err)
	}
}

func TestVerifyOperatorSettingsCommittedRootContractInventoryAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsCommittedRootContractInventoryAlignment(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsCommittedRootContractInventoryAlignment() error = %v", err)
	}
}

func TestVerifyOperatorSettingsRootReconciliationPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsRootReconciliation(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsRootReconciliation() error = %v", err)
	}
}

func TestOperatorSettingsCommittedBaselinesAlignMoveDestinations(t *testing.T) {
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

	const ownerPrefix = "pkg/services/operator_settings/"
	for _, row := range manifest.Packages {
		if row.PackagePath != "pkg/services/operator_settings" && !strings.HasPrefix(row.PackagePath, ownerPrefix) {
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

func operatorSettingsCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/document"):
		return true
	case strings.HasPrefix(rest, "internal/services/resolution"):
		return true
	default:
		return false
	}
}
