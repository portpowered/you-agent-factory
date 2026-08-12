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

func (stub *applicationOpeningRuntimeStub) OpenApplicationRuntime(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
) (factorysessionwire.OpenedApplicationRuntime, error) {
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

// TestApplicationOpeningClosesRuntimeWhenVisualizationSinkIsUnavailable
// proves the owner-bound application opener rejects a stale typed sink ID and
// closes the already-opened runtime before returning the boundary error.
func TestApplicationOpeningClosesRuntimeWhenVisualizationSinkIsUnavailable(t *testing.T) {
	t.Parallel()

	runtime := &applicationOpeningRuntimeStub{}
	resolve := factorysessionwire.ApplicationRuntimeInputResolver(func(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
	) (factorysessionwire.ApplicationRuntimeInputs, error) {
		return factorysessionwire.ApplicationRuntimeInputs{
			Request: &factorysessions.RuntimeOpeningRequest{},
		}, nil
	})
	adaptCalled := false
	adapt := factorysessionwire.RuntimeAdapter(func(
		factorysessionwire.OpenedApplicationRuntime,
		factoryvisualization.Sink,
	) (factorysessions.BoundProcessComponents, error) {
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
		t.Fatalf("NewApplicationService: %v", err)
	}

	_, err = service.OpenApplication(context.Background(), factorysessionwire.ApplicationOpeningRequest{
		Runtime:             &factorysessions.RuntimeOpeningRequest{},
		VisualizationSinkID: "stale-sink",
	})
	if err == nil || !strings.Contains(err.Error(), `Visualization sink "stale-sink" is unavailable`) {
		t.Fatalf("OpenApplication error = %v, want unavailable visualization sink", err)
	}
	if runtime.closed != 1 {
		t.Fatalf("opened runtime close count = %d, want one cleanup", runtime.closed)
	}
	if adaptCalled {
		t.Fatal("application adapter was called after visualization sink lookup failed")
	}
}
