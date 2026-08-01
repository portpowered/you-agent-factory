// Package canonical owns compilation-local canonical Factory encode behavior.
//
// Decode, normalize, and encode for authored/canonical loading remain on
// injected owner ports composed from process Wire. This package only preserves
// the compilation test seam; it does not select a transport codec.
package canonical

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// MarshalFactoryConfig serializes one effective Factory definition into canonical
// JSON bytes used for compilation content identity.
func MarshalFactoryConfig(
	encode factorydefinitions.FactoryConfigJSONEncoder,
	cfg *factorydefinitions.FactoryConfig,
) ([]byte, error) {
	return encode(cfg)
}

// EncodeFactoryPort is the contracts encoder port bound by compilation wire.
func EncodeFactoryPort(
	encode factorydefinitions.FactoryConfigJSONEncoder,
) factorydefinitions.FactoryConfigJSONEncoder {
	return func(cfg *factorydefinitions.FactoryConfig) ([]byte, error) {
		return MarshalFactoryConfig(encode, cfg)
	}
}
