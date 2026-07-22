package globalconfiginventory

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MarshalCanonicalJSON renders inventory as stable, byte-comparable JSON.
func MarshalCanonicalJSON(inventory Inventory) ([]byte, error) {
	return marshalCanonicalJSON(inventory)
}

func marshalCanonicalJSON(value any) ([]byte, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal global config topology inventory: %w", err)
	}

	var buffer bytes.Buffer
	buffer.Write(payload)
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}
