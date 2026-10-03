package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
	wirevalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/validation"
)

// NewValidationOperations constructs the owner validation implementation from
// injected orchestrator and canonical-load ports.
func NewValidationOperations(
	orchestrators factorydefinitions.OrchestratorDefinitionValidator,
	loadCanonical ...factorydefinitions.CanonicalFactoryJSONLoader,
) factorydefinitions.ValidationOperations {
	return wirevalidation.NewValidationOperations(orchestrators, loadCanonical...)
}

var (
	ValidateFactoryDefinition                                     = wirevalidation.ValidateFactoryDefinition
	ValidateBlockingFactoryLoad                                   = wirevalidation.ValidateBlockingFactoryLoad
	ValidatePortableResourceManifestOnPathWithSourceResolver      = wirevalidation.ValidatePortableResourceManifestOnPathWithSourceResolver
	ValidatePortableBundledFilesForExpandOnPathWithSourceResolver = wirevalidation.ValidatePortableBundledFilesForExpandOnPathWithSourceResolver
)

// Validation is the completed private validation owner supplied to Definitions.
type Validation = validationservice.Service

// NewValidationService binds the validation owner without loading or validating a Factory.
func NewValidationService(
	operations factorydefinitions.DefinitionValidationOperation,
	effective factorydefinitions.EffectiveDefinitionValidationOperation,
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
) Validation {
	return validationwire.NewService(operations, effective, loadCanonical, requiredToolChecker, orchestratorValidator)
}
