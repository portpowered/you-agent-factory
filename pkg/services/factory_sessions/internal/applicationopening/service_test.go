package applicationopening

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
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
	closed := 0
	open = runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{Resources: roles.RuntimeResources{
			Close: func() error {
				closed++
				return nil
			},
		}}, nil
	})
	adaptCalled := false
	adapt = RuntimeAdapter(func(roles.OpenedApplicationRuntime, factoryvisualization.Sink) (factorysessions.BoundProcessComponents, error) {
		adaptCalled = true
		return factorysessions.BoundProcessComponents{}, nil
	})
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
	if closed != 1 {
		t.Fatalf("opened runtime close count = %d, want one cleanup", closed)
	}
	if adaptCalled {
		t.Fatal("application adapter was called after visualization sink lookup failed")
	}
}

func TestOpenApplicationStopsAtRuntimeInputAndOpenFailures(t *testing.T) {
	resolveErr := errors.New("resolve failed")
	openErr := errors.New("open failed")
	tests := []struct {
		name       string
		resolveErr error
		openErr    error
	}{
		{name: "resolve", resolveErr: resolveErr},
		{name: "open", openErr: openErr},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolve, open, adapt, plan, owner := validApplicationOpeningDependencies()
			resolve = func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
				if test.resolveErr != nil {
					return RuntimeInputs{}, test.resolveErr
				}
				return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
			}
			open = func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
				return roles.OpenedApplicationRuntime{}, test.openErr
			}
			service, err := New(resolve, open, adapt, plan, owner)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
				Runtime: &factorysessions.RuntimeOpeningRequest{},
			})
			want := test.resolveErr
			if want == nil {
				want = test.openErr
			}
			if !errors.Is(err, want) {
				t.Fatalf("OpenApplication error = %v, want %v", err, want)
			}
		})
	}
}

func TestOpenApplicationClosesOpenedResourcesWhenLifecyclePlanningFails(t *testing.T) {
	planErr := errors.New("plan failed")
	closeErr := errors.New("close failed")
	closed := 0
	resolve, open, adapt, _, owner := validApplicationOpeningDependencies()
	open = runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{Resources: roles.RuntimeResources{
			Close: func() error {
				closed++
				return closeErr
			},
		}}, nil
	})
	plan := roles.LifecyclePlanOperation(func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
		return lifecycle.Plan{}, planErr
	})
	service, err := New(resolve, open, adapt, plan, owner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{},
	})
	if !errors.Is(err, planErr) || !errors.Is(err, closeErr) || closed != 1 {
		t.Fatalf("OpenApplication error = %v, close count = %d", err, closed)
	}
}

func TestOpenApplicationReturnsReadinessAndHostedInvocationCapabilities(t *testing.T) {
	ready := make(chan initializer.RuntimeHostBinding)
	resolve, open, adapt, plan, owner := validApplicationOpeningDependencies()
	open = runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{
			Process: &applicationProcessStub{ready: ready},
			HTTP: roles.RuntimeHTTPServices{
				FactorySessions: hostedServiceStub{},
			},
		}, nil
	})
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
	if opened.Ready == nil {
		t.Fatal("opened application readiness channel is nil")
	}
	if opened.Ready != ready {
		t.Fatal("opened application did not retain the process readiness channel")
	}
	if opened.HostedInvocation == nil {
		t.Fatal("opened application hosted invocation capability is nil")
	}
}

func TestOpenApplicationReturnsHistoricalReplayPlanningAndCleanupFailures(t *testing.T) {
	planErr := errors.New("historical plan failed")
	closeErr := errors.New("historical close failed")
	closed := 0
	inspection := &factorysessions.HistoricalReplayInspection{
		Session: factorysessions.SessionReadResult{SessionID: "recorded"},
	}
	resolve, open, adapt, _, owner := validApplicationOpeningDependencies()
	open = runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{
			HistoricalReplay: inspection,
			Resources: roles.RuntimeResources{
				Close: func() error {
					closed++
					return closeErr
				},
			},
		}, nil
	})
	plan := roles.LifecyclePlanOperation(func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
		return lifecycle.Plan{}, planErr
	})
	service, err := New(resolve, open, adapt, plan, owner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{
		Runtime: &factorysessions.RuntimeOpeningRequest{},
	})
	if !errors.Is(err, planErr) || !errors.Is(err, closeErr) || closed != 1 {
		t.Fatalf("OpenApplication error = %v, close count = %d", err, closed)
	}
}

func TestOpenApplicationRejectsNilService(t *testing.T) {
	var service *Service
	_, err := service.OpenApplication(context.Background(), roles.ApplicationOpeningRequest{})
	if err == nil || !strings.Contains(err.Error(), "service is required") {
		t.Fatalf("OpenApplication error = %v, want missing service", err)
	}
}

type applicationProcessStub struct {
	ready <-chan factorysessions.RuntimeHostBinding
}

func (*applicationProcessStub) Start(context.Context, context.Context) error { return nil }

func (*applicationProcessStub) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	return func(context.Context) error { return nil }, nil
}

func (*applicationProcessStub) RunTransport(context.Context, http.Handler) error { return nil }

func (*applicationProcessStub) Stop(context.Context) error { return nil }

func (process *applicationProcessStub) RuntimeHostReady() <-chan factorysessions.RuntimeHostBinding {
	return process.ready
}

type hostedServiceStub struct {
	factorysessions.Service
}
