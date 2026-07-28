package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyWorkTopLevelInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyWorkTopLevelInventory(root); err != nil {
		t.Fatalf("VerifyWorkTopLevelInventory() error = %v", err)
	}
}

func TestVerifyWorkDeletedTransitionalPublicPathsAbsentPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyWorkDeletedTransitionalPublicPathsAbsent(root); err != nil {
		t.Fatalf("VerifyWorkDeletedTransitionalPublicPathsAbsent() error = %v", err)
	}
}

func TestVerifyWorkPostDelTransitionalDebtBurnDownPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyWorkPostDelTransitionalDebtBurnDown(root); err != nil {
		t.Fatalf("VerifyWorkPostDelTransitionalDebtBurnDown() error = %v", err)
	}
}

func TestVerifyWorkUnexpectedPublicSiblingRemapsPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyWorkUnexpectedPublicSiblingRemaps(root); err != nil {
		t.Fatalf("VerifyWorkUnexpectedPublicSiblingRemaps() error = %v", err)
	}
}

func TestVerifyWorkRootGoInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyWorkRootGoInventory(root); err != nil {
		t.Fatalf("VerifyWorkRootGoInventory() error = %v", err)
	}
}
