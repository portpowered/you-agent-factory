package providersessions_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-PSES story 005 proves emptied transitional packages are gone, structure and
// ownership debt for the deleted paths is reduced, and repository structure
// verification passes. Wire behavioral proofs live in
// root_wire_behavioral_boundary_test.go; each subtest asserts one observable
// completion invariant for reviewers.

func TestDelPsesProofGate_RootShapeAndStructureOwnershipDebtReduced(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)

	t.Run("ownership_dual_ledger_alignment", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyProviderSessionsDualLedgerAlignment(root); err != nil {
			t.Fatalf("VerifyProviderSessionsDualLedgerAlignment() error = %v", err)
		}
	})

	t.Run("ownership_top_level_inventory", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyProviderSessionsTopLevelInventory(root); err != nil {
			t.Fatalf("VerifyProviderSessionsTopLevelInventory() error = %v", err)
		}
	})

	t.Run("ownership_zero_extra_public_sibling_absence", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyProviderSessionsZeroExtraPublicSiblingAbsence(root); err != nil {
			t.Fatalf("VerifyProviderSessionsZeroExtraPublicSiblingAbsence() error = %v", err)
		}
	})

	t.Run("ownership_inv_disposition_beyond_service", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyProviderSessionsINVDispositionBeyondService(root); err != nil {
			t.Fatalf("VerifyProviderSessionsINVDispositionBeyondService() error = %v", err)
		}
	})

	t.Run("ownership_unexpected_public_sibling_remaps", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyProviderSessionsUnexpectedPublicSiblingRemaps(root); err != nil {
			t.Fatalf("VerifyProviderSessionsUnexpectedPublicSiblingRemaps() error = %v", err)
		}
	})

	t.Run("ownership_root_go_inventory", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyProviderSessionsRootGoInventory(root); err != nil {
			t.Fatalf("VerifyProviderSessionsRootGoInventory() error = %v", err)
		}
	})
}
