// Command ownershipinventoryfreeze writes the PSS-F01 ownership-inventory freeze.
package main

import (
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func main() {
	root, err := ownershipinventory.FindRepositoryRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ownership-inventory-freeze: %v\n", err)
		os.Exit(1)
	}
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ownership-inventory-freeze: list packages: %v\n", err)
		os.Exit(1)
	}
	inventory, err := ownershipinventory.BuildInventory(root, packages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ownership-inventory-freeze: build inventory: %v\n", err)
		os.Exit(1)
	}
	if err := ownershipinventory.WriteInventory(root, inventory); err != nil {
		fmt.Fprintf(os.Stderr, "ownership-inventory-freeze: write inventory: %v\n", err)
		os.Exit(1)
	}
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	if err := ownershipinventory.WritePathLeaseFreeze(root, freeze); err != nil {
		fmt.Fprintf(os.Stderr, "ownership-inventory-freeze: write path-lease freeze: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d packages)\n", ownershipinventory.InventoryRelativePath, len(inventory.Packages))
	fmt.Printf("wrote %s (%d packets)\n", ownershipinventory.PathLeaseFreezeRelativePath, len(freeze.Packets))
}
