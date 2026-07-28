package factorydefinitions

import (
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ValidationOperations binds Factory Definitions validation construction to the
// owner wire surface selected by process Wire.
func ValidationOperations(
	orchestratorValidator contracts.OrchestratorDefinitionValidator,
	loadCanonical contracts.CanonicalFactoryJSONLoader,
) contracts.ValidationOperations {
	return factorydefinitionswire.NewValidationOperations(orchestratorValidator, loadCanonical)
}
