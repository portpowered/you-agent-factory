package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
