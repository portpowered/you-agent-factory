package recordings_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-REC story 005 proves emptied transitional packages are gone, structure and
// ownership debt for the deleted paths is reduced, and repository structure
// verification passes. Wire behavioral proofs live in wire/fold_behavior_preservation_test.go.
func TestDelRecProofGate_RootShapeAndStructureOwnershipDebtReduced(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)

	t.Run("ownership_top_level_inventory", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyRecordingsTopLevelInventory(root); err != nil {
			t.Fatalf("VerifyRecordingsTopLevelInventory() error = %v", err)
		}
	})

	t.Run("ownership_deleted_transitional_public_paths_absent", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyRecordingsDeletedTransitionalPublicPathsAbsent(root); err != nil {
			t.Fatalf("VerifyRecordingsDeletedTransitionalPublicPathsAbsent() error = %v", err)
		}
	})

	t.Run("ownership_post_del_transitional_debt_burn_down", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyRecordingsPostDelTransitionalDebtBurnDown(root); err != nil {
			t.Fatalf("VerifyRecordingsPostDelTransitionalDebtBurnDown() error = %v", err)
		}
	})
}
