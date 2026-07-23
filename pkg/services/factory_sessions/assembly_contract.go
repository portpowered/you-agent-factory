package factorysessions

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// RuntimeSidecars owns runtime-scoped background services without exposing
// their concrete host implementation to Factory Sessions consumers.
type RuntimeSidecars = factoryruntime.Sidecars

type DefinitionHost = factorydefinitions.SessionHost
