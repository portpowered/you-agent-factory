package cliinputs

import (
	"github.com/spf13/cobra"
)

// Walk inventories CLI arguments, flags, and parse relationships for every
// reachable command in root without mutating the tree. Argument extraction is
// implemented in story 002; flag and relationship walkers land in later stories.
func Walk(root *cobra.Command) (Inventory, error) {
	arguments := collectArgumentRecords(root)

	return Inventory{
		FormatVersion: FormatVersion,
		Arguments:     arguments,
		Flags:         []FlagRecord{},
		Relationships: []RelationshipRecord{},
	}, nil
}
