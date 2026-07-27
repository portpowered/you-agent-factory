package wire_test

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	activationlifecyclewire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire"
)

type wireSourceStub struct {
	subscribeHook func()
}

func (s wireSourceStub) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if s.subscribeHook != nil {
		s.subscribeHook()
	}
	return &factorydefinitions.FactoryEventStream{
		Events: make(chan factorydefinitions.FactoryEvent),
	}, nil
}

func (wireSourceStub) GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error) {
	return &factoryruntime.StateSnapshot{}, nil
}

type wireProjectionStub struct{}

func (wireProjectionStub) ReconstructFactoryWorldState(
	[]factorydefinitions.FactoryEvent,
	int,
) (factorydefinitions.FactoryWorldState, error) {
	return factorydefinitions.FactoryWorldState{}, nil
}

func (wireProjectionStub) SimpleDashboardRenderData(
	factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (wireProjectionStub) ProjectActiveThrottlePauses(
	factorydefinitions.InitialStructurePayload,
	[]factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return nil
}

type wireClock struct{}

func (wireClock) Now() time.Time { return time.Unix(1, 0) }

func TestNewServiceConstructsActivationLifecycleOwner(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	presentCalls := 0
	source := wireSourceStub{subscribeHook: func() { subscribeCalls++ }}
	service, err := activationlifecyclewire.NewService(
		source,
		wireProjectionStub{},
		wireClock{},
		wireSinkFunc(func(activationlifecycle.View) { presentCalls++ }),
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil")
	}
	var _ activationlifecycle.Service = service
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatalf("NewService() side effects: subscribe=%d present=%d, want inert construction", subscribeCalls, presentCalls)
	}

	_, err = service.Join(context.Background(), activationlifecycle.JoinRequest{})
	if err == nil {
		t.Fatal("Join before Activate: error = nil, want not-activated failure")
	}
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatal("Join before Activate must not subscribe or present")
	}
}

func TestNewServiceExplicitRequestActivation(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	presentCalls := 0
	source := wireSourceStub{subscribeHook: func() { subscribeCalls++ }}
	service, err := activationlifecyclewire.NewService(
		source,
		wireProjectionStub{},
		wireClock{},
		wireSinkFunc(func(activationlifecycle.View) { presentCalls++ }),
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Activate(context.Background(), activationlifecycle.ActivateRequest{})
	if err == nil {
		t.Fatal("zero-value Activate: error = nil, want missing-parameters failure")
	}
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatal("zero-value Activate must not subscribe or present")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := service.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate RETAINED_THEN_LIVE: error = %v", err)
	}
	if result.State != activationlifecycle.LifecycleStateStarted {
		t.Fatalf("Activate state = %q, want %q", result.State, activationlifecycle.LifecycleStateStarted)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1 after explicit Activate", subscribeCalls)
	}

	_, err = service.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	})
	if err == nil {
		t.Fatal("repeat Activate: error = nil, want already-activated failure")
	}

	cancel()
	if _, err := service.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{}); err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}
}

func TestNewServiceStopWaitCleanup(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	source := wireSourceStub{subscribeHook: func() { subscribeCalls++ }}
	service, err := activationlifecyclewire.NewService(
		source,
		wireProjectionStub{},
		wireClock{},
		wireSinkFunc(func(activationlifecycle.View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.Wait(context.Background()); err == nil {
		t.Fatal("Wait before Activate: error = nil, want not-started failure")
	}

	ctx := context.Background()
	if _, err := service.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	}); err != nil {
		t.Fatalf("Activate: error = %v", err)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1", subscribeCalls)
	}

	stopResult, err := service.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{})
	if err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}
	if stopResult.State != activationlifecycle.LifecycleStateStopped {
		t.Fatalf("StopDrain state = %q, want %q", stopResult.State, activationlifecycle.LifecycleStateStopped)
	}
	if err := service.Wait(ctx); err != nil {
		t.Fatalf("Wait after StopDrain: error = %v", err)
	}

	stopResult, err = service.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{})
	if err != nil {
		t.Fatalf("repeat StopDrain: error = %v", err)
	}
	if stopResult.State != activationlifecycle.LifecycleStateStopped {
		t.Fatalf("repeat StopDrain state = %q, want %q", stopResult.State, activationlifecycle.LifecycleStateStopped)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls after repeated StopDrain = %d, want no reopened subscription", subscribeCalls)
	}
}

type wireSinkFunc func(activationlifecycle.View)

func (f wireSinkFunc) PresentFactoryView(view activationlifecycle.View) { f(view) }
