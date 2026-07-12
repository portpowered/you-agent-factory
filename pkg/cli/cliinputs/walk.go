package cliinputs

import (
	"github.com/spf13/cobra"
)

// Walk inventories CLI arguments, flags, and parse relationships for every
// reachable command in root without mutating the tree.
func Walk(root *cobra.Command) (Inventory, error) {
	arguments := collectArgumentRecords(root)
	flags := collectFlagRecords(root)
	relationships := collectRelationshipRecords(root)

	return Inventory{
		FormatVersion: FormatVersion,
		Arguments:     arguments,
		Flags:         flags,
		Relationships: relationships,
	}, nil
}
