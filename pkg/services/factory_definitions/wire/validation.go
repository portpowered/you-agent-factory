package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
)

// NewValidationOperations constructs the owner validation implementation from
// injected orchestrator and canonical-load ports.
func NewValidationOperations(
	orchestrators factorydefinitions.OrchestratorDefinitionValidator,
	loadCanonical ...factorydefinitions.CanonicalFactoryJSONLoader,
) factorydefinitions.ValidationOperations {
	return validationimpl.New(orchestrators, loadCanonical...)
}

var (
	ValidateFactoryDefinition = validationimpl.Validate
	ValidateBlockingFactoryLoad = validationimpl.ValidateBlockingLoad
	ValidatePortableResourceManifestOnPathWithSourceResolver = validationimpl.ValidatePortableResourceManifestOnPathWithSourceResolver
	ValidatePortableBundledFilesForExpandOnPathWithSourceResolver = validationimpl.ValidatePortableBundledFilesForExpandOnPathWithSourceResolver
)
