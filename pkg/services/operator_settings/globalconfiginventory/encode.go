package globalconfiginventory

import (
	"bytes"
	"encoding/json"
)

// MarshalCanonicalJSON renders inventory as stable, byte-comparable JSON.
func MarshalCanonicalJSON(inventory Inventory) ([]byte, error) {
	payload, err := json.MarshalIndent(inventory, "", "  ")

	var buffer bytes.Buffer
	buffer.Write(payload)
	buffer.WriteByte('\n')
	return buffer.Bytes(), err
}
