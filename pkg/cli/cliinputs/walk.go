package cliinputs

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Walk inventories CLI arguments, flags, and parse relationships for every
// reachable command in root without mutating the tree. Collections are sorted
// using the key order documented in sort.go before return.
func Walk(root *cobra.Command) (Inventory, error) {
	before := captureCommandInputsTreeState(root)

	inv := Inventory{
		FormatVersion: FormatVersion,
		Arguments:     collectArgumentRecords(root),
		Flags:         collectFlagRecords(root),
		Relationships: collectRelationshipRecords(root),
	}
	sortInventoryCollections(&inv)

	after := captureCommandInputsTreeState(root)
	if err := commandInputsTreeStatesEqual(before, after); err != nil {
		return Inventory{}, fmt.Errorf("command tree mutated during walk: %w", err)
	}

	return inv, nil
}
