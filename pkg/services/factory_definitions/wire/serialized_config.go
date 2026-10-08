package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// FactoryConfigDecoder binds the immutable packaged conversion publication to
// the canonical mapper used for all other Factory definitions.
func FactoryConfigDecoder(source factorydefinitions.SerializedFactoryConfigReader) factorydefinitions.FactoryConfigJSONDecoder {
	mapper := factorymapping.NewFactoryConfigMapper()
	return compilationwire.NewSerializedFactoryConfigDecoder(source, mapper.Expand)
}
