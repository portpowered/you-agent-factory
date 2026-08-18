package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyProviderSessionsUnexpectedPublicSiblingRemapsPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsUnexpectedPublicSiblingRemaps(root); err != nil {
		t.Fatalf("VerifyProviderSessionsUnexpectedPublicSiblingRemaps() error = %v", err)
	}
}

func TestFrozenInventoryProviderSessionsRejectsRetainToOwnerRootForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	topLevel, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(inventory.Packages))
	for _, row := range inventory.Packages {
		ownershipByPath[row.PackagePath] = row
	}

	for _, packagePath := range ownershipinventory.ProviderSessionsUnexpectedPublicSiblingPackagePaths(topLevel) {
		row, ok := ownershipByPath[packagePath]
		if !ok {
			t.Fatalf("frozen ownership inventory missing unexpected public sibling %q", packagePath)
		}
		if row.Disposition == ownershipinventory.DispositionRetain && row.Destination == "provider_sessions" {
			t.Fatalf("frozen ownership inventory row retain→provider_sessions for %q", packagePath)
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("frozen ownership inventory row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Successor == "" || row.DeletionCondition == "" {
			t.Fatalf("frozen ownership inventory row %q missing successor/deletionCondition: %#v", packagePath, row)
		}
	}
}

func TestProviderSessionsCommittedBaselinesAlignMoveDestinationsForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	topLevel, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	moveByPath := committedMoveLedger(t, root)

	for _, packagePath := range ownershipinventory.ProviderSessionsUnexpectedPublicSiblingPackagePaths(topLevel) {
		moveRow, ok := moveByPath[packagePath]
		if !ok {
			t.Fatalf("move ledger missing unexpected public sibling %q; it must still carry an open move", packagePath)
		}
		if moveRow.Destination == "provider_sessions" {
			t.Fatalf("move ledger row %q destination = owner root, want nested plan path", packagePath)
		}
		wantSuccessor := "pkg/services/" + moveRow.Destination
		if moveRow.Successor != wantSuccessor {
			t.Fatalf("move ledger drift for %q: destination %q => successor %q, ledger has %q",
				packagePath, moveRow.Destination, wantSuccessor, moveRow.Successor)
		}
	}
}
