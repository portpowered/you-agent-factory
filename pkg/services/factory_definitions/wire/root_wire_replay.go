package wire

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	internalreplayconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// ReplayRuntimeConfigDecoder binds replay lookup reconstruction to the
// canonical Factory representation decoder.
func ReplayRuntimeConfigDecoder() contracts.ReplayRuntimeConfigDecoder {
	return func(
		snapshot *contracts.FactorySnapshot,
	) (contracts.ReplayRuntimeConfig, error) {
		return internalreplayconfig.Decode(snapshot, factorymapping.FactoryConfigFromOpenAPIJSON)
	}
}
