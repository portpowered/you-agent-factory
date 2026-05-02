package testutil

import "github.com/portpowered/infinite-you/internal/handwrittensourceguard"

type HandwrittenSourcePathClass = handwrittensourceguard.PathClass

const (
	HandwrittenSourcePathClassScanHandwritten   = handwrittensourceguard.PathClassScanHandwritten
	HandwrittenSourcePathClassExcludeGenerated  = handwrittensourceguard.PathClassExcludeGenerated
	HandwrittenSourcePathClassExcludeHiddenRoot = handwrittensourceguard.PathClassExcludeHiddenRoot
)

type HandwrittenSourcePathRule = handwrittensourceguard.PathRule

type HandwrittenSourceGuardInventoryEntry = handwrittensourceguard.InventoryEntry

// HandwrittenSourceGuardInventory is the source of truth for broad
// filesystem-walking handwritten-source guards that need consistent skip policy.
func HandwrittenSourceGuardInventory() []HandwrittenSourceGuardInventoryEntry {
	return handwrittensourceguard.Inventory()
}
