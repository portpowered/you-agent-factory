// Package validation exposes owner-wire validation helpers without importing the
// full factory_definitions/wire composition graph. Catalog and other internal
// consumers that must not create wire→internal→distribution cycles should depend
// on this package instead of factory_definitions/wire.
package validation

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
)

// NewValidationOperations constructs the owner validation implementation from
// injected orchestrator and canonical-load ports.
func NewValidationOperations(
	orchestrators factorydefinitions.OrchestratorDefinitionValidator,
	loadCanonical ...factorydefinitions.CanonicalFactoryJSONLoader,
) factoryeffect.ValidationOperations {
	return validationwire.NewValidationOperations(orchestrators, loadCanonical...)
}

// New constructs the public validation port through the owner wire boundary.
func New(
	orchestrators factorydefinitions.OrchestratorDefinitionValidator,
	loadCanonical ...factorydefinitions.CanonicalFactoryJSONLoader,
) factorydefinitions.Validator {
	return validationwire.New(orchestrators, loadCanonical...)
}

var (
	ValidateFactoryDefinition                                     = validationwire.ValidateFactoryDefinition
	ValidateBlockingFactoryLoad                                   = validationwire.ValidateBlockingFactoryLoad
	ValidatePortableResourceManifestOnPathWithSourceResolver      = validationwire.ValidatePortableResourceManifestOnPathWithSourceResolver
	ValidatePortableBundledFilesForExpandOnPathWithSourceResolver = validationwire.ValidatePortableBundledFilesForExpandOnPathWithSourceResolver
)
