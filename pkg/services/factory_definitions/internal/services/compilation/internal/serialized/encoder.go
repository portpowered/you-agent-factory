package serialized

import (
	"encoding/json"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Encoder uses immutable canonical output for exact normalized definitions.
// Customer edits and runtime-only metadata keep the original mapping path.
type Encoder struct {
	source   factorydefinitions.CanonicalFactoryConfigReader
	fallback factorydefinitions.FactoryConfigJSONEncoder
}

// NewEncoder binds the selected publication without reading or preparing it.
func NewEncoder(source factorydefinitions.CanonicalFactoryConfigReader, fallback factorydefinitions.FactoryConfigJSONEncoder) *Encoder {
	return &Encoder{source: source, fallback: fallback}
}

// Encode keeps validation and authored normalization with its callers.
func (encoder *Encoder) Encode(config *factorydefinitions.FactoryConfig) ([]byte, error) {
	if encoder.source != nil && serializableCanonicalInput(config) {
		input, err := json.Marshal(config)
		if err == nil {
			data, err := encoder.source(input)
			if err == nil && len(data) > 0 && data[0] == '{' && json.Valid(data) {
				return append([]byte(nil), data...), nil
			}
		}
	}
	if encoder.fallback == nil {
		return nil, fmt.Errorf("canonical Factory config encoder is required")
	}
	return encoder.fallback(config)
}

func serializableCanonicalInput(config *factorydefinitions.FactoryConfig) bool {
	if config == nil || len(config.IgnoredJSONPaths()) != 0 {
		return false
	}
	for _, worker := range config.Workers {
		if worker.PromptSourcePath != "" || worker.SessionID != "" || worker.Concurrency != 0 || worker.RuntimeDefaultModelProvider != "" || worker.RuntimeDefaultModel != "" {
			return false
		}
	}
	for _, station := range config.Workstations {
		if station.PromptSourcePath != "" || station.PromptSourceIsTemplate {
			return false
		}
	}
	return true
}
