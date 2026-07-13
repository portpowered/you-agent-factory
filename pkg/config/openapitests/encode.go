package openapitests

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MarshalParityInventoryJSON renders the Factory/OpenAPI parity inventory as stable JSON.
func MarshalParityInventoryJSON(inventory ParityInventory) ([]byte, error) {
	payload, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal factory openapi parity inventory: %w", err)
	}

	var buffer bytes.Buffer
	buffer.Write(payload)
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}
