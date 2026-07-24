package ownershipinventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads the frozen ownership-inventory artifact from the repository root.
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
	return inventory, nil
}

// LoadEffective returns the ownership inventory with package rows replaced by
// the FND-01 seed when that seed artifact is present.
func LoadEffective(root string) (Inventory, bool, error) {
	inventory, err := Load(root)
	if err != nil {
		return Inventory{}, false, err
	}
	seed, ok, err := loadFND01Seed(root)
	if err != nil {
		return Inventory{}, false, err
	}
	if !ok {
		return inventory, false, nil
	}
	inventory.Packages = seed.Packages
	return inventory, true, nil
}

func loadFND01Seed(root string) (PackageTargetManifest, bool, error) {
	path := filepath.Join(root, filepath.FromSlash(FND01SeedRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PackageTargetManifest{}, false, nil
		}
		return PackageTargetManifest{}, false, fmt.Errorf("read FND-01 seed %s: %w", FND01SeedRelativePath, err)
	}
	var seed PackageTargetManifest
	if err := json.Unmarshal(payload, &seed); err != nil {
		return PackageTargetManifest{}, false, fmt.Errorf("parse FND-01 seed %s: %w", FND01SeedRelativePath, err)
	}
	if len(seed.Packages) == 0 {
		return PackageTargetManifest{}, false, fmt.Errorf("FND-01 seed %s has no packages", FND01SeedRelativePath)
	}
	return seed, true, nil
}

// WriteInventory writes the ownership-inventory freeze artifact.
func WriteInventory(root string, inventory Inventory) error {
	path := filepath.Join(root, filepath.FromSlash(InventoryRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir ownership inventory: %w", err)
	}
	payload, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ownership inventory: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write ownership inventory: %w", err)
	}
	return nil
}
