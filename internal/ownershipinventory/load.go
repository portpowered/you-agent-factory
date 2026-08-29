package ownershipinventory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads the frozen ownership-inventory artifact from the repository root,
// attaches the consolidated open-move ledger as its package rows, and derives
// the destination vocabulary from the live pkg/services tree.
//
// The inventory document itself no longer carries package rows: the only rows
// that survive are open moves, and those live in one consolidated ledger shared
// with the packaged-service-structure checker. It does not carry the destination
// vocabulary either — that is derived at load time, so adding a service does not
// require regenerating this artifact.
func Load(root string) (Inventory, error) {
	path := filepath.Join(root, filepath.FromSlash(InventoryRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return Inventory{}, fmt.Errorf("read ownership inventory %s: %w", InventoryRelativePath, err)
	}
	var inventory Inventory
	if err := json.Unmarshal(payload, &inventory); err != nil {
		return Inventory{}, fmt.Errorf("parse ownership inventory %s: %w", InventoryRelativePath, err)
	}
	moves, err := LoadUnfinishedMoves(root)
	if err != nil {
		return Inventory{}, err
	}
	vocabulary, err := DiscoverDestinationVocabulary(root)
	if err != nil {
		return Inventory{}, err
	}
	inventory.Destinations = vocabulary
	inventory.UnfinishedMoves = moves
	inventory.Packages = moves.PackageRows()
	return inventory, nil
}

// WriteInventory writes the ownership-inventory freeze artifact.
func WriteInventory(root string, inventory Inventory) error {
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(inventory); err != nil {
		return fmt.Errorf("marshal ownership inventory: %w", err)
	}
	return writeWithFileSystem(osFileSystem{}, root, InventoryRelativePath, payload.Bytes(), 0o644, "mkdir ownership inventory", "write ownership inventory")
}
