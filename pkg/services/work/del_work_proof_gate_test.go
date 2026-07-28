package work_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-WORK story 005 proves emptied transitional packages are gone, structure and
// ownership debt for the deleted paths is reduced, and repository structure
// verification passes. Wire behavioral proofs live in
// wire_behavioral_proof_test.go; each subtest asserts one observable completion
// invariant for reviewers.
func TestDelWorkProofGate_RootShapeAndStructureOwnershipDebtReduced(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)

	t.Run("ownership_dual_ledger_alignment", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyWorkDualLedgerAlignment(root); err != nil {
			t.Fatalf("VerifyWorkDualLedgerAlignment() error = %v", err)
		}
	})

	t.Run("ownership_top_level_inventory", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyWorkTopLevelInventory(root); err != nil {
			t.Fatalf("VerifyWorkTopLevelInventory() error = %v", err)
		}
	})

	t.Run("ownership_deleted_transitional_public_paths_absent", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyWorkDeletedTransitionalPublicPathsAbsent(root); err != nil {
			t.Fatalf("VerifyWorkDeletedTransitionalPublicPathsAbsent() error = %v", err)
		}
	})

	t.Run("ownership_post_del_transitional_debt_burn_down", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyWorkPostDelTransitionalDebtBurnDown(root); err != nil {
			t.Fatalf("VerifyWorkPostDelTransitionalDebtBurnDown() error = %v", err)
		}
	})

	t.Run("ownership_unexpected_public_sibling_remaps", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyWorkUnexpectedPublicSiblingRemaps(root); err != nil {
			t.Fatalf("VerifyWorkUnexpectedPublicSiblingRemaps() error = %v", err)
		}
	})

	t.Run("ownership_root_go_inventory", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyWorkRootGoInventory(root); err != nil {
			t.Fatalf("VerifyWorkRootGoInventory() error = %v", err)
		}
	})
}
