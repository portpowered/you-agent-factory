package mockworkers

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MarshalCanonicalJSON renders inventory as stable, byte-comparable JSON.
func MarshalCanonicalJSON(inventory Inventory) ([]byte, error) {
	payload, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal mock workers topology inventory: %w", err)
	}

	var buffer bytes.Buffer
	buffer.Write(payload)
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}
