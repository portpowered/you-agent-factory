package root_composition_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

type applicationOpeningRuntimeStub struct {
	closed int
}

func (stub *applicationOpeningRuntimeStub) OpenApplicationRuntime(context.Context, *factorysessions.RuntimeOpeningRequest) (factorysessionwire.OpenedApplicationRuntime, error) {
	return factorysessionwire.OpenedApplicationRuntime{
		Resources: factorysessionwire.RuntimeResources{
			Close: func() error {
				stub.closed++
				return nil
			},
		},
	}, nil
}

type unavailableVisualizationSinkOwner struct{}

func (unavailableVisualizationSinkOwner) RegisterRuntimeSink(factoryvisualization.Sink) (factoryvisualization.RuntimeSinkID, error) {
	return "", errors.New("registration is not part of this test")
}

func (unavailableVisualizationSinkOwner) RuntimeSink(factoryvisualization.RuntimeSinkID) (factoryvisualization.Sink, bool) {
	return nil, false
}

func (unavailableVisualizationSinkOwner) CloseRuntimeSink(factoryvisualization.RuntimeSinkID) {}

// TestApplicationOpeningClosesRuntimeWhenVisualizationSinkIsUnavailable proves
// that the application opener closes an already-opened runtime when a stale
// typed visualization-sink ID cannot be resolved at the service boundary.
// Normal CLI execution registers a valid sink before opening the application;
// this focused boundary test supplies the invalid opaque ID directly so the
// cleanup behavior remains observable in the functional coverage lane.
func TestApplicationOpeningClosesRuntimeWhenVisualizationSinkIsUnavailable(t *testing.T) {
	t.Parallel()

	runtime := &applicationOpeningRuntimeStub{}
	resolve := factorysessionwire.ApplicationRuntimeInputResolver(func(context.Context, *factorysessions.RuntimeOpeningRequest) (factorysessionwire.ApplicationRuntimeInputs, error) {
		return factorysessionwire.ApplicationRuntimeInputs{
			Request: &factorysessions.RuntimeOpeningRequest{},
		}, nil
	})
	adaptCalled := false
	adapt := factorysessionwire.RuntimeAdapter(func(factorysessionwire.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
		adaptCalled = true
		return factorysessions.BoundProcessComponents{}, nil
	})

	service, err := factorysessionwire.NewApplicationService(
		resolve,
		runtime,
		adapt,
		factorysessionwire.NewLifecyclePlanOperation(),
		unavailableVisualizationSinkOwner{},
	)
	if err != nil {
		t.Fatalf("NewApplicationService() error = %v", err)
	}

	_, err = service.OpenApplication(context.Background(), &factorysessions.RuntimeOpeningRequest{}, "stale-sink")
	if err == nil || !strings.Contains(err.Error(), `Visualization sink "stale-sink" is unavailable`) {
		t.Fatalf("OpenApplication() error = %v, want unavailable visualization sink", err)
	}
	if runtime.closed != 1 {
		t.Fatalf("runtime close count = %d, want 1", runtime.closed)
	}
	if adaptCalled {
		t.Fatal("runtime adapter was called after visualization sink resolution failed")
	}
}
