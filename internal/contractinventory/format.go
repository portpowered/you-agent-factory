package contractinventory

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MarshalCanonicalJSON renders inventory as stable, byte-comparable JSON.
func MarshalCanonicalJSON(inventory *Inventory) ([]byte, error) {
	if inventory == nil {
		return nil, fmt.Errorf("inventory is nil")
	}

	payload, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal inventory json: %w", err)
	}

	var buffer bytes.Buffer
	buffer.Write(payload)
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}
