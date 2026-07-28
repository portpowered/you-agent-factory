package factory

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ObservationHasActiveWork reports whether a plain observation still has
// in-flight dispatches or non-terminal Work categories.
func ObservationHasActiveWork(observation Observation) bool {
	if observation.Progress.InFlightDispatchCount > 0 || len(observation.InFlightDispatches) > 0 {
		return true
	}
	categories := observation.Progress.WorkCategories
	return categories.Processing > 0 || categories.Initial > 0
}

// RequireIdleRuntimeFromObservation validates the shared definition-activation
// precondition using orchestration-neutral observation vocabulary.
func RequireIdleRuntimeFromObservation(observation Observation) error {
	if observation.Status == "" {
		return fmt.Errorf("%w: runtime observation is unavailable", interfaces.ErrFactoryActivationRequiresIdle)
	}
	if observation.Status != ObservationStatusIdle {
		return fmt.Errorf(
			"%w: current runtime status is %s",
			interfaces.ErrFactoryActivationRequiresIdle,
			observation.Status,
		)
	}
	if ObservationHasActiveWork(observation) {
		return fmt.Errorf("%w: current runtime has active work", interfaces.ErrFactoryActivationRequiresIdle)
	}
	return nil
}
