package symbolidentity

import "encoding/json"

// MarshalInventory serializes inventory using the canonical JSON encoding used
// for committed JavaScript runtime symbol identity baselines.
func MarshalInventory(inv Inventory) ([]byte, error) {
	return json.Marshal(inv)
}
