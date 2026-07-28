package factorydefinitions

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// ReplayRuntimeConfigDecoder binds replay lookup reconstruction to the
// canonical Factory representation decoder.
func ReplayRuntimeConfigDecoder() contracts.ReplayRuntimeConfigDecoder {
	return factorydefinitionswire.ReplayRuntimeConfigDecoder()
}
