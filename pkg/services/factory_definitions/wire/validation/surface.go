// Package validation exposes owner-wire validation helpers without importing the
// full factory_definitions/wire composition graph. Catalog and other internal
// consumers that must not create wire→internal→distribution cycles should depend
// on this package instead of factory_definitions/wire.
package validation

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/contracts"
	validationimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
)

// validationOperations is the private validation implementation surface
// needed by owner composition. It is intentionally not re-exported through
// factory_definitions.
type validationOperations interface {
	Validate(
		context.Context,
		*factorydefinitions.FactoryConfig,
		validationcontracts.WorkflowSourceReader,
	) factorydefinitions.ValidationResult
	ValidateBlockingLoad(context.Context, *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult
	ValidateTopology(
		context.Context,
		*factorydefinitions.FactoryConfig,
		validationcontracts.RequiredToolChecker,
	) factorydefinitions.TopologyValidationResult
	WorkerWorkstationBehaviorCompatibility(
		context.Context,
		*factorydefinitions.FactoryConfig,
	) []factorydefinitions.ValidationTarget
	WorkTypeHandlingBehavior(
		context.Context,
		*factorydefinitions.FactoryConfig,
		bool,
	) []factorydefinitions.ValidationTarget
	PruneLayout(
		context.Context,
		*factorydefinitions.FactoryConfig,
		factorydefinitions.PendingFactoryGraphTopology,
	) factorydefinitions.ValidationResult
}

// NewValidationOperations constructs the owner validation implementation from
// injected orchestrator and canonical-load ports.
func NewValidationOperations(
	orchestrators validationcontracts.OrchestratorDefinitionValidator,
	loadCanonical ...validationcontracts.CanonicalFactoryLoader,
) validationOperations {
	return validationimpl.New(orchestrators, loadCanonical...)
}

var (
	ValidateFactoryDefinition                                     = validationimpl.Validate
	ValidateBlockingFactoryLoad                                   = validationimpl.ValidateBlockingLoad
	ValidatePortableResourceManifestOnPathWithSourceResolver      = validationimpl.ValidatePortableResourceManifestOnPathWithSourceResolver
	ValidatePortableBundledFilesForExpandOnPathWithSourceResolver = validationimpl.ValidatePortableBundledFilesForExpandOnPathWithSourceResolver
)
