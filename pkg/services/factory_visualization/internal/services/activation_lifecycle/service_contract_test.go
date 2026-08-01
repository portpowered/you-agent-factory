package activationlifecycle_test

import (
	"testing"

	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
)

func TestActivationLifecycleLifecycleSurfaceUsesVisualizationOwnedObservation(t *testing.T) {
	t.Parallel()

	var _ activationlifecycle.EngineObservation
	var _ activationlifecycle.ActivateRequest
	var _ activationlifecycle.ActivateResult
	var _ activationlifecycle.JoinRequest
	var _ activationlifecycle.JoinResult
	var _ activationlifecycle.StopDrainRequest
	var _ activationlifecycle.StopDrainResult
	var _ activationlifecycle.LifecycleError
}
