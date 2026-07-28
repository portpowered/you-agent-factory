package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyOperatorSettingsUnexpectedPublicSiblingRemapsPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsUnexpectedPublicSiblingRemaps(root); err != nil {
		t.Fatalf("VerifyOperatorSettingsUnexpectedPublicSiblingRemaps() error = %v", err)
	}
}

func TestFrozenInventoryOperatorSettingsRejectsRetainToOwnerRootForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(inventory.Packages))
	for _, row := range inventory.Packages {
		ownershipByPath[row.PackagePath] = row
	}

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		row, ok := ownershipByPath[packagePath]
		if !ok {
			t.Fatalf("frozen ownership inventory missing unexpected public sibling %q", packagePath)
		}
		if row.Disposition == ownershipinventory.DispositionRetain && row.Destination == "operator_settings" {
			t.Fatalf("frozen ownership inventory row retain→operator_settings for %q", packagePath)
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("frozen ownership inventory row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Successor == "" || row.DeletionCondition == "" {
			t.Fatalf("frozen ownership inventory row %q missing successor/deletionCondition: %#v", packagePath, row)
		}
	}
}

func TestOperatorSettingsCommittedBaselinesAlignMoveDestinationsForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	manifest, err := loadPackageTargetManifest(root)
	if err != nil {
		t.Fatalf("loadPackageTargetManifest() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(inventory.Packages))
	for _, row := range inventory.Packages {
		ownershipByPath[row.PackagePath] = row
	}
	manifestByPath := make(map[string]packageTargetManifestRow, len(manifest.Packages))
	for _, row := range manifest.Packages {
		manifestByPath[row.PackagePath] = row
	}

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		manifestRow, ok := manifestByPath[packagePath]
		if !ok {
			t.Fatalf("package-target manifest missing unexpected public sibling %q", packagePath)
		}
		if manifestRow.Disposition == ownershipinventory.DispositionRetain && manifestRow.Destination == "operator_settings" {
			t.Fatalf("committed manifest row retain→operator_settings for %q", packagePath)
		}
		if manifestRow.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", packagePath, manifestRow.Disposition)
		}
		if manifestRow.Destination == "operator_settings" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", packagePath)
		}

		ownershipRow, ok := ownershipByPath[packagePath]
		if !ok {
			t.Fatalf("ownership inventory missing committed manifest move row %q", packagePath)
		}
		wantSuccessor := "pkg/services/" + manifestRow.Destination
		if ownershipRow.Successor != wantSuccessor {
			t.Fatalf("dual-ledger drift for %q: manifest destination %q => successor %q, ownership has %q",
				packagePath, manifestRow.Destination, wantSuccessor, ownershipRow.Successor)
		}
	}
}
