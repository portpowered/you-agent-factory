package factorydefinitions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// SerializedFactoryConfigVersion identifies the generated conversion cache.
// Regeneration checks bind its bytes to the current canonical mapper.
const SerializedFactoryConfigVersion = "factory-config-native/v1"

// SerializedFactoryConfig caches the canonical mapping of one exact published
// JSON payload. The generator owns this serialization boundary; installation
// still owns validation, normalization and filesystem publication.
type SerializedFactoryConfig struct {
	Version       string          `json:"version"`
	PayloadSHA256 string          `json:"payloadSHA256"`
	Config        json.RawMessage `json:"config"`
}

// SerializedFactoryConfigReader reads a detached conversion from the validated
// immutable publication selected by application composition.
type SerializedFactoryConfigReader func([]byte) ([]byte, error)

// SerializedFactoryConfigInput identifies the exact source of a cached conversion.
type SerializedFactoryConfigInput []byte

// Path addresses a conversion by its full input bytes.
func (payload SerializedFactoryConfigInput) Path() string {
	hash := sha256.Sum256(payload)
	return "generated/native-config-v1/" + hex.EncodeToString(hash[:]) + ".json"
}
