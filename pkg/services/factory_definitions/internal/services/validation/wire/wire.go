// Package wire constructs the Factory Definitions validation subservice from
// exact injected validation and canonical-load ports.
package wire

import (
	"fmt"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/impl"
	validationserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/service"
)

func NewValidationOperations(
	orchestrators factorydefinitions.OrchestratorDefinitionValidator,
	loadCanonical ...factorydefinitions.CanonicalFactoryJSONLoader,
) factoryeffects.ValidationOperations {
	return validationimpl.New(orchestrators, loadCanonical...)
}

// New exposes the owner-local validator constructor through validation wire
// for focused tests and owner composition. The implementation type remains
// private to validation's internal tree.
func New(
	orchestrators factorydefinitions.OrchestratorDefinitionValidator,
	loadCanonical ...factorydefinitions.CanonicalFactoryJSONLoader,
) *validationimpl.Service {
	return validationimpl.New(orchestrators, loadCanonical...)
}

var (
	ValidateFactoryDefinition                                     = validationimpl.Validate
	ValidateBlockingFactoryLoad                                   = validationimpl.ValidateBlockingLoad
	ValidatePortableResourceManifestOnPathWithSourceResolver      = validationimpl.ValidatePortableResourceManifestOnPathWithSourceResolver
	ValidatePortableBundledFilesForExpandOnPathWithSourceResolver = validationimpl.ValidatePortableBundledFilesForExpandOnPathWithSourceResolver
)

const (
	CodeOrchestratorJavaScriptMissingSource = validationimpl.CodeOrchestratorJavaScriptMissingSource
	CodeOrchestratorUnsupportedKind         = validationimpl.CodeOrchestratorUnsupportedKind
	CodeRequiredToolMissing                 = validationimpl.CodeRequiredToolMissing
	CodeRequiredToolVersionProbe            = validationimpl.CodeRequiredToolVersionProbe
	CodeDanglingPlaceReference              = validationimpl.CodeDanglingPlaceReference
	CodeDanglingWorkerReference             = validationimpl.CodeDanglingWorkerReference
	CodeDuplicateIdentifier                 = validationimpl.CodeDuplicateIdentifier
)

// NewService constructs the private validation subservice from exact injected
// validation-operation and canonical-load ports. Callers must supply
// Dependencies; this constructor does not select Runtime/Petri implementations
// or take Wire/root construction ownership.
func NewService(deps validationservice.Dependencies) (validationservice.Service, error) {
	if deps.Operations == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: definition validation operation is required")
	}
	if deps.Effective == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: effective definition validation operation is required")
	}
	if deps.LoadCanonical == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: canonical Factory loader is required")
	}
	service := validationserviceimpl.New(
		deps.Operations,
		deps.Effective,
		deps.LoadCanonical,
		deps.RequiredToolChecker,
		deps.OrchestratorValidator,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: implementation rejected its dependencies")
	}
	return service, nil
}
