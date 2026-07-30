package activationlifecycle_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
)

func TestActivationLifecycleLifecycleSurfaceUsesVisualizationOwnedObservation(t *testing.T) {
	t.Parallel()

	var source activationlifecycle.EventSource = observationSourceProbe{}
	var sink activationlifecycle.ViewSink = observationSinkProbe{}
	_ = source
	_ = sink

	var _ activationlifecycle.EngineObservation
	var _ activationlifecycle.ActivateRequest
	var _ activationlifecycle.ActivateResult
	var _ activationlifecycle.JoinRequest
	var _ activationlifecycle.JoinResult
	var _ activationlifecycle.StopDrainRequest
	var _ activationlifecycle.StopDrainResult
	var _ activationlifecycle.LifecycleError
}

type observationSourceProbe struct{}

func (observationSourceProbe) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return nil, nil
}

func (observationSourceProbe) GetEngineObservation(
	context.Context,
) (*activationlifecycle.EngineObservation, error) {
	return &activationlifecycle.EngineObservation{}, nil
}

type observationSinkProbe struct{}

func (observationSinkProbe) PresentFactoryView(view activationlifecycle.View) {
	_ = view.EngineObservation
}
