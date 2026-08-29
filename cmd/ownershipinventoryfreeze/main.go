// Command ownershipinventoryfreeze writes the PSS-F01 ownership-inventory freeze
// and the S-05 through S-08 owner-root snapshots.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ownership-inventory-freeze: %v\n", err)
		os.Exit(1)
	}
}

func run(stdout io.Writer) error {
	root, err := ownershipinventory.FindRepositoryRoot()
	if err != nil {
		return fmt.Errorf("find repository root: %w", err)
	}
	return runAtRoot(root, stdout)
}

func runAtRoot(root string, stdout io.Writer) error {
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		return fmt.Errorf("list packages: %w", err)
	}
	inventory, err := ownershipinventory.BuildInventory(root, packages)
	if err != nil {
		return fmt.Errorf("build inventory: %w", err)
	}
	candidates, err := ownershipinventory.BuildSnapshotCandidates(root)
	if err != nil {
		return fmt.Errorf("build S-05 through S-08 snapshots: %w", err)
	}
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	if err := ownershipinventory.PublishSnapshotGroup(root, packages, inventory, freeze, candidates); err != nil {
		return fmt.Errorf("publish snapshot group: %w", err)
	}
	fmt.Fprintf(stdout, "wrote %s (%d packages)\n", ownershipinventory.InventoryRelativePath, len(inventory.Packages))
	fmt.Fprintf(stdout, "wrote %s (%d packets)\n", ownershipinventory.PathLeaseFreezeRelativePath, len(freeze.Packets))
	fmt.Fprintf(stdout, "wrote %s (%d files)\n", ownershipinventory.OperatorSettingsRootGoInventoryRelativePath, len(candidates.OperatorSettingsRootGo.Files))
	fmt.Fprintf(stdout, "wrote %s (%d directories)\n", ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath, len(candidates.OperatorSettingsTopLevel.Children))
	fmt.Fprintf(stdout, "wrote %s (%d files)\n", ownershipinventory.ProviderSessionsRootGoInventoryRelativePath, len(candidates.ProviderSessionsRootGo.Files))
	fmt.Fprintf(stdout, "wrote %s (%d directories)\n", ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath, len(candidates.ProviderSessionsTopLevel.Children))
	return nil
}
