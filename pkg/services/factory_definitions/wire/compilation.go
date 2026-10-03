package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/canonical"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
)

// Compilation is the completed private owner supplied to the Definitions root.
type Compilation = compilationservice.Service

// FactoryConfigJSONEncoder supplies the canonical serialization port.
func FactoryConfigJSONEncoder() factorydefinitions.FactoryConfigJSONEncoder {
	return compilationcanonical.EncodeFactoryPort()
}

// NewCompilationService constructs only the inert compilation owner.
func NewCompilationService(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFromFactoryDir factorydefinitions.LoadedFactoryLoader,
	encodeFactory factorydefinitions.FactoryConfigJSONEncoder,
) Compilation {
	return compilationwire.NewService(loadCanonical, loadFromFactoryDir, encodeFactory)
}
