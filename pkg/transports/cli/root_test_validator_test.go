package cli

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// uncheckedPersistenceValidator is supplied only where a transport fixture
// intentionally bypasses validation to seed malformed persisted input. Layout
// pruning is the sole operation used while preparing that fixture; every
// validation method panics so this cannot silently become a validation path.
type uncheckedPersistenceValidator struct{}

// validPersistenceValidator is used only to seed a valid persisted Factory for
// CLI routing and rendering tests. Factory Definitions owner suites retain the
// actual validation semantics.
type validPersistenceValidator struct {
	uncheckedPersistenceValidator
}

func (validPersistenceValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}

func (validPersistenceValidator) ValidateBlockingLoad(
	context.Context,
	*factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}

func (uncheckedPersistenceValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	panic("unexpected Factory Definition Validate call")
}

func (uncheckedPersistenceValidator) ValidateBlockingLoad(
	context.Context,
	*factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	panic("unexpected Factory Definition ValidateBlockingLoad call")
}

func (uncheckedPersistenceValidator) ValidateTopology(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	panic("unexpected Factory Definition ValidateTopology call")
}

func (uncheckedPersistenceValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	panic("unexpected Factory Definition WorkerWorkstationBehaviorCompatibility call")
}

func (uncheckedPersistenceValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorydefinitions.FactoryConfig,
	bool,
) []factorydefinitions.ValidationTarget {
	panic("unexpected Factory Definition WorkTypeHandlingBehavior call")
}

func (uncheckedPersistenceValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
