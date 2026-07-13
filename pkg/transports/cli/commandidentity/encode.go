package commandidentity

import "encoding/json"

// MarshalInventory serializes inventory using the canonical JSON encoding used
// for committed command identity baselines.
func MarshalInventory(inv Inventory) ([]byte, error) {
	return json.Marshal(inv)
}
