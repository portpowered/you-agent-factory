package contractjoiner

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MarshalCanonicalJSON serializes a joined JSON value with recursively sorted
// object keys, two-space indentation, and exactly one trailing newline. The
// encoding/json package guarantees sorted map keys; array order and scalar
// values are preserved.
func MarshalCanonicalJSON(value any) ([]byte, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal joined contract: %w", err)
	}

	var result bytes.Buffer
	result.Grow(len(payload) + 1)
	result.Write(payload)
	result.WriteByte('\n')
	return result.Bytes(), nil
}
