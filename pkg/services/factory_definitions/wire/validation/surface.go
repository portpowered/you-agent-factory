// Package validation exposes owner-wire validation helpers without importing the
// full factory_definitions/wire composition graph. Catalog and other internal
// consumers that must not create wire→internal→distribution cycles should depend
// on this package instead of factory_definitions/wire.
package validation

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
