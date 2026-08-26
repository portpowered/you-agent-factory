package applicationopening

import (
	"context"
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

type runtimeOpenerFunc func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error)

func (open runtimeOpenerFunc) OpenApplicationRuntime(ctx context.Context, request *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
	return open(ctx, request)
}

func validApplicationOpeningDependencies() (
	RuntimeInputResolver,
	runtimeOpenerFunc,
	RuntimeAdapter,
	roles.LifecyclePlanOperation,
) {
	resolve := RuntimeInputResolver(func(context.Context, *factorysessions.RuntimeOpeningRequest) (RuntimeInputs, error) {
		return RuntimeInputs{Request: &factorysessions.RuntimeOpeningRequest{}}, nil
	})
	open := runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{}, nil
	})
	adapt := RuntimeAdapter(func(roles.OpenedApplicationRuntime, factorysessions.VisualizationSinkID) (factorysessions.BoundProcessComponents, error) {
		return factorysessions.BoundProcessComponents{}, nil
	})
	plan := roles.LifecyclePlanOperation(func(roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
		return lifecycle.Plan{}, nil
	})
	return resolve, open, adapt, plan
}

func TestNewRequiresEveryInjectedOperation(t *testing.T) {
	resolve, open, adapt, plan := validApplicationOpeningDependencies()
	for name, construct := range map[string]func() error{
		"resolver":  func() error { _, err := New(nil, open, adapt, plan); return err },
		"opener":    func() error { _, err := New(resolve, nil, adapt, plan); return err },
		"adapter":   func() error { _, err := New(resolve, open, nil, plan); return err },
		"lifecycle": func() error { _, err := New(resolve, open, adapt, nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := construct(); err == nil {
				t.Fatalf("New() error = nil, want missing %s operation", name)
			}
		})
	}
}

func TestOpenApplicationWithCancellationPublishesInvocationAuthority(t *testing.T) {
	resolve, open, _, plan := validApplicationOpeningDependencies()
	want := &applicationOpeningCancellationStub{}
	var got initializer.InvocationCancellation
	adapt := RuntimeAdapter(func(opened roles.OpenedApplicationRuntime, _ factorysessions.VisualizationSinkID) (factorysessions.BoundProcessComponents, error) {
		got = opened.Cancellation
		return factorysessions.BoundProcessComponents{}, nil
	})
	service, err := New(resolve, open, adapt, plan)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := service.OpenApplicationWithCancellation(
		context.Background(), &factorysessions.RuntimeOpeningRequest{}, want, "",
	); err != nil {
		t.Fatalf("OpenApplicationWithCancellation: %v", err)
	}
	if got != want {
		t.Fatalf("opened cancellation = %p, want %p", got, want)
	}
}

type applicationOpeningCancellationStub struct{}

func (*applicationOpeningCancellationStub) Cancel() {}

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
