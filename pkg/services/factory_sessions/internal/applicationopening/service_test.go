package applicationopening

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

type runtimeOpenerFunc func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error)

func (open runtimeOpenerFunc) OpenApplicationRuntime(ctx context.Context, request *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
	return open(ctx, request)
}

type markerSink struct{}

func (markerSink) PresentFactoryView(factoryvisualization.View) {}

type runtimeSinkOwnerStub struct {
	sinks map[factoryvisualization.RuntimeSinkID]factoryvisualization.Sink
}

func newRuntimeSinkOwnerStub(sink factoryvisualization.Sink) *runtimeSinkOwnerStub {
	return &runtimeSinkOwnerStub{sinks: map[factoryvisualization.RuntimeSinkID]factoryvisualization.Sink{"sink-1": sink}}
}

func (owner *runtimeSinkOwnerStub) RegisterRuntimeSink(sink factoryvisualization.Sink) (factoryvisualization.RuntimeSinkID, error) {
	if owner.sinks == nil {
		owner.sinks = make(map[factoryvisualization.RuntimeSinkID]factoryvisualization.Sink)
	}
	owner.sinks["sink-1"] = sink
	return "sink-1", nil
}

func (owner *runtimeSinkOwnerStub) RuntimeSink(id factoryvisualization.RuntimeSinkID) (factoryvisualization.Sink, bool) {
	sink, ok := owner.sinks[id]
	return sink, ok
}

func (owner *runtimeSinkOwnerStub) CloseRuntimeSink(id factoryvisualization.RuntimeSinkID) {
	delete(owner.sinks, id)
}

func validApplicationOpeningDependencies() (
	RuntimeInputResolver,
	runtimeOpenerFunc,
	RuntimeAdapter,
	roles.LifecyclePlanOperation,
	*runtimeSinkOwnerStub,
) {
	resolve := RuntimeInputResolver(func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
		return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
	})
	open := runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{}, nil
	})
	adapt := RuntimeAdapter(func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
		return factorysessions.BoundProcessComponents{}, nil
	})
	plan := roles.LifecyclePlanOperation(func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
		return lifecycle.Plan{}, nil
	})
	return resolve, open, adapt, plan, newRuntimeSinkOwnerStub(&markerSink{})
}

func TestNewRequiresEveryInjectedOperation(t *testing.T) {
	resolve, open, adapt, plan, owner := validApplicationOpeningDependencies()
	for name, construct := range map[string]func() error{
		"resolver":   func() error { _, err := New(nil, open, adapt, plan, owner); return err },
		"opener":     func() error { _, err := New(resolve, nil, adapt, plan, owner); return err },
		"adapter":    func() error { _, err := New(resolve, open, nil, plan, owner); return err },
		"lifecycle":  func() error { _, err := New(resolve, open, adapt, nil, owner); return err },
		"sink owner": func() error { _, err := New(resolve, open, adapt, plan, nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := construct(); err == nil {
				t.Fatalf("New() error = nil, want missing %s operation", name)
			}
		})
	}
}

func TestOpenApplicationUsesValueRequestAndTypedVisualizationOwner(t *testing.T) {
	request := &factorysessions.RuntimeOpeningRequest{}
	resolved := &factorysessions.RuntimeOpeningRequest{}
	owner := newRuntimeSinkOwnerStub(&markerSink{})
	var gotSink factoryvisualization.Sink
	var gotPlan roles.LifecyclePlanRequest
	resolve := func(_ context.Context, got *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
		if got != request {
			t.Fatalf("resolver request = %p, want %p", got, request)
		}
		return RuntimeInputs{Request: resolved}, nil
	}
	open := runtimeOpenerFunc(func(_ context.Context, got *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		if got != resolved {
			t.Fatalf("runtime opener request = %p, want %p", got, resolved)
		}
		return roles.OpenedApplicationRuntime{}, nil
	})
	adapt := RuntimeAdapter(func(_ roles.OpenedApplicationRuntime, sink factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
		gotSink = sink
		return factorysessions.BoundProcessComponents{}, nil
	})
	plan := roles.LifecyclePlanOperation(func(got roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
		gotPlan = got
		return lifecycle.Plan{}, nil
	})
	service, err := New(resolve, open, adapt, plan, owner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: request, VisualizationSinkID: "sink-1",
	}); err != nil {
		t.Fatalf("OpenApplication: %v", err)
	}
	if gotSink == nil {
		t.Fatal("typed visualization owner did not supply sink")
	}
	if gotPlan.Close != nil {
		t.Fatal("lifecycle plan unexpectedly received a completion callback")
	}
}

func TestOpenApplicationReturnsDetachedHistoricalReplay(t *testing.T) {
	inspection := &factorysessions.HistoricalReplayInspection{
		Session: factorysessions.SessionReadResult{SessionID: "recorded-session"},
	}
	resolve, _, adapt, plan, owner := validApplicationOpeningDependencies()
	open := runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{HistoricalReplay: inspection}, nil
	})
	adapt = func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
		return factorysessions.BoundProcessComponents{}, errors.New("live adapter must not run for replay")
	}
	service, err := New(resolve, open, adapt, plan, owner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	opened, err := service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{},
	})
	if err != nil {
		t.Fatalf("OpenApplication: %v", err)
	}
	if opened.HistoricalReplay != inspection {
		t.Fatalf("historical replay = %#v, want detached inspection %#v", opened.HistoricalReplay, inspection)
	}
}

func TestOpenApplicationClosesOpenedResourcesOnBindingFailure(t *testing.T) {
	closeErr := errors.New("close failed")
	bindErr := errors.New("bind failed")
	closed := 0
	resolve, open, _, plan, owner := validApplicationOpeningDependencies()
	open = runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{Resources: roles.RuntimeResources{Close: func() error {
			closed++
			return closeErr
		}}}, nil
	})
	adapt := RuntimeAdapter(func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
		return factorysessions.BoundProcessComponents{}, bindErr
	})
	service, err := New(resolve, open, adapt, plan, owner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{}, VisualizationSinkID: "sink-1",
	})
	if !errors.Is(err, bindErr) || !errors.Is(err, closeErr) || closed != 1 {
		t.Fatalf("OpenApplication error = %v, close count = %d", err, closed)
	}
}

func TestOpenApplicationRejectsUnavailableVisualizationSink(t *testing.T) {
	resolve, open, adapt, plan, owner := validApplicationOpeningDependencies()
	service, err := New(resolve, open, adapt, plan, owner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{}, VisualizationSinkID: "missing",
	})
	if err == nil || !strings.Contains(err.Error(), "Visualization sink") {
		t.Fatalf("OpenApplication error = %v, want unavailable sink", err)
	}
}
