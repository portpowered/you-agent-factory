package wire

import (
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
)

// DefinitionLoader is the Factory Definitions loading implementation selected
// by the application composition root.
type DefinitionLoader = compilationloading.Loader
