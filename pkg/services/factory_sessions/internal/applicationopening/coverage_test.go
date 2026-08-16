package applicationopening

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

func TestRuntimeReadyReturnsNilWithoutReadinessPort(t *testing.T) {
	t.Parallel()

	if got := runtimeReady(nil); got != nil {
		t.Fatalf("runtimeReady(nil) = %v, want nil", got)
	}
}

func TestCloseOpenedRuntimePreservesCauseWithoutCloseOperation(t *testing.T) {
	t.Parallel()

	cause := errors.New("binding failed")
	if got := closeOpenedRuntime(roles.OpenedApplicationRuntime{}, cause); !errors.Is(got, cause) {
		t.Fatalf("closeOpenedRuntime() = %v, want original cause", got)
	}
}

func TestHistoricalReplayPlanRunsDetachedNoopTransport(t *testing.T) {
	inspection := &factorysessions.HistoricalReplayInspection{}
	resolve, open, adapt, _, owner := validApplicationOpeningDependencies()
	open = runtimeOpenerFunc(func(context.Context, *factorysessions.RuntimeOpeningRequest) (roles.OpenedApplicationRuntime, error) {
		return roles.OpenedApplicationRuntime{HistoricalReplay: inspection}, nil
	})
	var transport interface {
		Start(context.Context) error
		Wait(context.Context) error
		Stop(context.Context) error
	}
	plan := roles.LifecyclePlanOperation(func(request roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
		transport, _ = request.Components.Transport.(interface {
			Start(context.Context) error
			Wait(context.Context) error
			Stop(context.Context) error
		})
		return lifecycle.Plan{}, nil
	})
	service, err := New(resolve, open, adapt, plan, owner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := service.OpenApplication(context.Background(), &factorysessions.RuntimeOpeningRequest{}, ""); err != nil {
		t.Fatalf("OpenApplication: %v", err)
	}
	if transport == nil {
		t.Fatal("historical replay plan did not provide a detached transport runner")
	}
	ctx := context.Background()
	if err := transport.Start(ctx); err != nil {
		t.Fatalf("detached transport Start: %v", err)
	}
	if err := transport.Wait(ctx); err != nil {
		t.Fatalf("detached transport Wait: %v", err)
	}
	if err := transport.Stop(ctx); err != nil {
		t.Fatalf("detached transport Stop: %v", err)
	}
}
