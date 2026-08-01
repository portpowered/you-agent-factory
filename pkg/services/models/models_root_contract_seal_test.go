package models_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestModelsRootContractInventorySeal(t *testing.T) {
	t.Parallel()

	if err := ownershipinventory.VerifyModelsRootContractInventory(modelsRepositoryRoot(t)); err != nil {
		t.Fatalf("VerifyModelsRootContractInventory() error = %v", err)
	}
}
