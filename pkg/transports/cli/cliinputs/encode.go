package cliinputs

import "encoding/json"

// MarshalInventory serializes inventory using the canonical JSON encoding used
// for committed CLI inputs baselines.
func MarshalInventory(inv Inventory) ([]byte, error) {
	return json.Marshal(inv)
}
