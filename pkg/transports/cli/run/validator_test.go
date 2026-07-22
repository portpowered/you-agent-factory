package run

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// uncheckedPersistenceValidator permits only layout pruning while an invalid
// persisted fixture is authored. Any validation call is unexpected because
// the run transport test exercises bootstrap routing, not validation policy.
type uncheckedPersistenceValidator struct{}

func (uncheckedPersistenceValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
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
