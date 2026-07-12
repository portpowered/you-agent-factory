package cliinputs

import (
	"github.com/spf13/cobra"
)

// Walk inventories CLI arguments, flags, and parse relationships for every
// reachable command in root without mutating the tree. Extraction is filled in
// by later stories; this entrypoint returns the stable document shape with
// empty collections until argument, flag, and relationship walkers land.
func Walk(root *cobra.Command) (Inventory, error) {
	_ = root
	return EmptyInventory(), nil
}
