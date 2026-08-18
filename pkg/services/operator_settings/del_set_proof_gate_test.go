package operatorsettings_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-SET story 005 proves emptied transitional packages are gone, structure and
// ownership debt for the deleted paths is reduced, and repository structure
// verification passes. Wire behavioral proofs live in
// root_wire_behavioral_boundary_test.go; each subtest asserts one observable
// completion invariant for reviewers.
func TestDelSetProofGate_RootShapeAndStructureOwnershipDebtReduced(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)

	// There is no longer a second package ledger to align against: open moves
	// live in one consolidated file and settled packages derive their owner from
	// the tree, so the cross-ledger alignment subtest was retired with it.

	t.Run("ownership_top_level_inventory", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyOperatorSettingsTopLevelInventory(root); err != nil {
			t.Fatalf("VerifyOperatorSettingsTopLevelInventory() error = %v", err)
		}
	})

	t.Run("ownership_unexpected_public_sibling_remaps", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyOperatorSettingsUnexpectedPublicSiblingRemaps(root); err != nil {
			t.Fatalf("VerifyOperatorSettingsUnexpectedPublicSiblingRemaps() error = %v", err)
		}
	})

	t.Run("ownership_root_go_inventory", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyOperatorSettingsRootGoInventory(root); err != nil {
			t.Fatalf("VerifyOperatorSettingsRootGoInventory() error = %v", err)
		}
	})

	t.Run("ownership_root_contract_inventory_alignment", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyOperatorSettingsCommittedRootContractInventoryAlignment(root); err != nil {
			t.Fatalf("VerifyOperatorSettingsCommittedRootContractInventoryAlignment() error = %v", err)
		}
	})

	t.Run("ownership_root_reconciliation", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyOperatorSettingsRootReconciliation(root); err != nil {
			t.Fatalf("VerifyOperatorSettingsRootReconciliation() error = %v", err)
		}
	})
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return dir
}
