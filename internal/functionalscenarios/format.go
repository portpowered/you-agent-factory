package functionalscenarios

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MarshalCanonicalJSON renders a projection as stable, byte-comparable JSON.
func MarshalCanonicalJSON(projection *Projection) ([]byte, error) {
	if projection == nil {
		return nil, fmt.Errorf("functional scenario component projection is nil")
	}
	payload, err := json.MarshalIndent(projection, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal functional scenario component projection: %w", err)
	}
	var output bytes.Buffer
	output.Write(payload)
	output.WriteByte('\n')
	return output.Bytes(), nil
}

// MarshalCanonicalManifestJSON renders a reviewed manifest as stable JSON.
func MarshalCanonicalManifestJSON(manifest *Manifest) ([]byte, error) {
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal functional scenario manifest: %w", err)
	}
	return append(payload, '\n'), nil
}
