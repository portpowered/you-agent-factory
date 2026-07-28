package identityinventory_test

import (
	"testing"

	documentidentityinventory "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/identityinventory"
	identityinventory "github.com/portpowered/infinite-you/pkg/services/operator_settings/identityinventory"
)

// Keep the public shim package exercised by delegating to the document owner inventory.
func TestShimProjectInputInventoryMatchesDocumentOwner(t *testing.T) {
	shimInventory := identityinventory.ProjectInputInventory()
	ownerInventory := documentidentityinventory.ProjectInputInventory()
	if shimInventory.FormatVersion != ownerInventory.FormatVersion {
		t.Fatalf("formatVersion = %q, want %q", shimInventory.FormatVersion, ownerInventory.FormatVersion)
	}
	if len(shimInventory.Cases) != len(ownerInventory.Cases) {
		t.Fatalf("cases = %d, want %d", len(shimInventory.Cases), len(ownerInventory.Cases))
	}
}
