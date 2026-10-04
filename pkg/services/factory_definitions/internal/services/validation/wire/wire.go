// Package wire constructs the Factory Definitions validation owner from direct ports.
package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/service"
)

// NewService constructs an inert validation owner from completed collaborators.
func NewService(
	operations factorydefinitions.DefinitionValidationOperation,
	effective factorydefinitions.EffectiveDefinitionValidationOperation,
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
) validationservice.Service {
	return validationserviceimpl.New(operations, effective, loadCanonical, requiredToolChecker, orchestratorValidator)
}
