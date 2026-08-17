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
	moveByPath := committedMoveLedger(t, root)

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		moveRow, ok := moveByPath[packagePath]
		if !ok {
			t.Fatalf("move ledger missing unexpected public sibling %q; it must still carry an open move", packagePath)
		}
		if moveRow.Destination == "operator_settings" {
			t.Fatalf("move ledger row %q destination = owner root, want nested plan path", packagePath)
		}
		wantSuccessor := "pkg/services/" + moveRow.Destination
		if moveRow.Successor != wantSuccessor {
			t.Fatalf("move ledger drift for %q: destination %q => successor %q, ledger has %q",
				packagePath, moveRow.Destination, wantSuccessor, moveRow.Successor)
		}
	}
}
