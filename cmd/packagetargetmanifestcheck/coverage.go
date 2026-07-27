package main

import (
	"fmt"
	"slices"
	"strings"
)

// validatePackageCoverage proves every inventory ledger-seed path has exactly
// one packages[] destination/deletion row, packages are stable-sorted, and no
// duplicate or inventory-extra paths exist. Success means the manifest can be
// consumed unchanged as the exact deletion-only nonconformance ledger seed.
func validatePackageCoverage(manifest Manifest) error {
	if len(manifest.Inventory) == 0 {
		return fmt.Errorf("package coverage requires a non-empty inventory ledger seed")
	}

	inventorySet := make(map[string]struct{}, len(manifest.Inventory))
	for _, packagePath := range manifest.Inventory {
		inventorySet[packagePath] = struct{}{}
	}

	seen := make(map[string]struct{}, len(manifest.Packages))
	for i, row := range manifest.Packages {
		packagePath := row.PackagePath
		if _, dup := seen[packagePath]; dup {
			return fmt.Errorf("packages contains duplicate package path %q", packagePath)
		}
		seen[packagePath] = struct{}{}
		if _, inInventory := inventorySet[packagePath]; !inInventory {
			return fmt.Errorf("packages[%d] %q is not present in inventory", i, packagePath)
		}
	}

	if !slices.IsSortedFunc(manifest.Packages, func(a, b PackageMapping) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	}) {
		return fmt.Errorf("packages must be stable-sorted by packagePath")
	}

	for _, packagePath := range manifest.Inventory {
		if _, ok := seen[packagePath]; !ok {
			return fmt.Errorf("packages missing mapping for inventory path %q", packagePath)
		}
	}

	if len(manifest.Packages) != len(manifest.Inventory) {
		return fmt.Errorf("packages length %d does not match inventory length %d", len(manifest.Packages), len(manifest.Inventory))
	}
	return nil
}
