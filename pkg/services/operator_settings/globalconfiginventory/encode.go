package globalconfiginventory

import (
	"bytes"
	"encoding/json"
)

// MarshalCanonicalJSON renders inventory as stable, byte-comparable JSON.
func MarshalCanonicalJSON(inventory Inventory) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(inventory)
	return buffer.Bytes(), err
}
