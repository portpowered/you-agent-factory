package operatorsettings

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MarshalInputInventoryJSON renders the operator config input inventory as stable JSON.
func MarshalInputInventoryJSON(inventory InputInventory) ([]byte, error) {
	payload, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal operator config input inventory: %w", err)
	}

	var buffer bytes.Buffer
	buffer.Write(payload)
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}
