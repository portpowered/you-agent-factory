package providers_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestProvidersRootContractInventorySeal(t *testing.T) {
	t.Parallel()

	if err := ownershipinventory.VerifyProvidersRootContractInventory(providersRepositoryRoot(t)); err != nil {
		t.Fatalf("VerifyProvidersRootContractInventory() error = %v", err)
	}
}

func providersRepositoryRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
