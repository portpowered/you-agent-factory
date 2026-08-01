// Package canonical owns compilation-local canonical Factory encode behavior.
//
// Decode and normalize for authored/canonical loading remain on injected loader
// ports composed from process wire; only the effective-source content-identity
// encoder lives here so Factory Definitions wire does not bind compilation to
// transport-mapping codecs directly.
package canonical

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// MarshalFactoryConfig serializes one effective Factory definition into canonical
// JSON bytes used for compilation content identity.
func MarshalFactoryConfig(cfg *factorydefinitions.FactoryConfig) ([]byte, error) {
	return factorymapping.MarshalCanonicalFactoryConfig(cfg)
}

// EncodeFactoryPort is the contracts encoder port bound by compilation wire.
func EncodeFactoryPort() factorydefinitions.FactoryConfigJSONEncoder {
	return factorydefinitions.FactoryConfigJSONEncoder(MarshalFactoryConfig)
}
