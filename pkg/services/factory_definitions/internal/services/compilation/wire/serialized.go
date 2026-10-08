package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/serialized"
)

// NewSerializedFactoryConfigDecoder binds optional immutable conversions to the canonical decoder.
func NewSerializedFactoryConfigDecoder(source factorydefinitions.SerializedFactoryConfigReader, fallback factorydefinitions.FactoryConfigJSONDecoder) factorydefinitions.FactoryConfigJSONDecoder {
	return serialized.NewDecoder(source, fallback).Decode
}

// NewCanonicalFactoryConfigEncoder binds exact normalized input to immutable output.
func NewCanonicalFactoryConfigEncoder(source factorydefinitions.CanonicalFactoryConfigReader, fallback factorydefinitions.FactoryConfigJSONEncoder) factorydefinitions.FactoryConfigJSONEncoder {
	return serialized.NewEncoder(source, fallback).Encode
}
