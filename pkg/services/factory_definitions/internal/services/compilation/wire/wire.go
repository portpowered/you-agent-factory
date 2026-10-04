// Package wire constructs the Factory Definitions compilation subservice from
// exact injected load and encode ports.
package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/service"
)

// NewService constructs the private compilation subservice from exact injected
// canonical/directory load and encode ports. The caller supplies normalized,
// required collaborators; construction performs no loading or encoding.
func NewService(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFromFactoryDir factorydefinitions.LoadedFactoryLoader,
	encodeFactory factorydefinitions.FactoryConfigJSONEncoder,
) compilationservice.Service {
	return compilationserviceimpl.New(loadCanonical, loadFromFactoryDir, encodeFactory)
}
