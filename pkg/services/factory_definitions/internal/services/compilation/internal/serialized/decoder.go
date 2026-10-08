// Package serialized loads build-generated mappings of immutable packaged
// Factory payloads while leaving each caller's definition independently owned.
package serialized

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Decoder consumes an immutable conversion publication and the canonical mapper
// for arbitrary customer payloads or unusable optional cache entries.
type Decoder struct {
	source   factorydefinitions.SerializedFactoryConfigReader
	fallback factorydefinitions.FactoryConfigJSONDecoder
}

// NewDecoder selects the conversion publication without reading or preparing it.
func NewDecoder(source factorydefinitions.SerializedFactoryConfigReader, fallback factorydefinitions.FactoryConfigJSONDecoder) *Decoder {
	return &Decoder{source: source, fallback: fallback}
}

// Decode returns a fresh canonical value. Every validation and pruning caller
// can mutate this value without changing the conversion or another session.
func (decoder *Decoder) Decode(payload []byte) (*factorydefinitions.FactoryConfig, error) {
	if decoder.source != nil {
		data, err := decoder.source(payload)
		if err == nil {
			if config, ok := decodeConversion(payload, data); ok {
				return config, nil
			}
		}
	}
	if decoder.fallback == nil {
		return nil, fmt.Errorf("canonical Factory config decoder is required")
	}
	return decoder.fallback(payload)
}

func decodeConversion(payload, data []byte) (*factorydefinitions.FactoryConfig, bool) {
	var conversion factorydefinitions.SerializedFactoryConfig
	if err := json.Unmarshal(data, &conversion); err != nil {
		return nil, false
	}
	hash := sha256.Sum256(payload)
	if conversion.Version != factorydefinitions.SerializedFactoryConfigVersion || conversion.PayloadSHA256 != hex.EncodeToString(hash[:]) {
		return nil, false
	}
	var config *factorydefinitions.FactoryConfig
	if err := json.Unmarshal(conversion.Config, &config); err != nil || config == nil {
		return nil, false
	}
	return config, true
}
