package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
)

func TestActivationLifecycleOwnerBacksRootLifecycleSlice(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	source := &lifecycleSourceStub{
		stream:   newLifecycleEventStream(),
		snapshot: &factoryruntime.StateSnapshot{TickCount: 1},
	}
	source.subscribeHook = func() { subscribeCalls++ }
	presentCalls := 0
	owner, err := lifecycleservice.New(
		source,
		lifecycleProjectionStub{},
		fixedLifecycleClock{now: time.Unix(1, 0)},
		lifecycleSinkFunc(func(activationlifecycle.View) { presentCalls++ }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var lifecycle activationlifecycle.Service = owner
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatalf("construction side effects: subscribe=%d present=%d, want inert", subscribeCalls, presentCalls)
	}

	_, err = lifecycle.Join(context.Background(), activationlifecycle.JoinRequest{})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorNotActivated, "Join before Activate")

	_, err = lifecycle.Activate(context.Background(), activationlifecycle.ActivateRequest{})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorMissingParameters, "Activate missing parameters")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := lifecycle.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate: error = %v", err)
	}
	if result.State != activationlifecycle.LifecycleStateStarted {
		t.Fatalf("Activate state = %q, want %q", result.State, activationlifecycle.LifecycleStateStarted)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1", subscribeCalls)
	}

	_, err = lifecycle.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateModeRetainedThenLive,
	})
	requireActivationLifecycleError(t, err, activationlifecycle.LifecycleErrorAlreadyActivated, "Activate already activated")

	cancel()
	if _, err := lifecycle.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{}); err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}
}

func requireActivationLifecycleError(
	t *testing.T,
	err error,
	kind activationlifecycle.LifecycleErrorKind,
	label string,
) {
	t.Helper()
	var lifeErr *activationlifecycle.LifecycleError
	if !errors.As(err, &lifeErr) || lifeErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}

type lifecycleSourceStub struct {
	stream        *factorydefinitions.FactoryEventStream
	subscribeHook func()
	snapshot      *factoryruntime.StateSnapshot
}

func (s *lifecycleSourceStub) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if s.subscribeHook != nil {
		s.subscribeHook()
	}
	return s.stream, nil
}

func (s *lifecycleSourceStub) GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error) {
	return s.snapshot, nil
}

func newLifecycleEventStream() *factorydefinitions.FactoryEventStream {
	return &factorydefinitions.FactoryEventStream{
		Events: make(chan factorydefinitions.FactoryEvent),
	}
}

func newClosableLifecycleEventStream() (*factorydefinitions.FactoryEventStream, chan<- factorydefinitions.FactoryEvent) {
	events := make(chan factorydefinitions.FactoryEvent)
	return &factorydefinitions.FactoryEventStream{
		Events: events,
	}, events
}

type lifecycleProjectionStub struct{}

func (lifecycleProjectionStub) ReconstructFactoryWorldState(
	[]factorydefinitions.FactoryEvent,
	int,
) (factorydefinitions.FactoryWorldState, error) {
	return factorydefinitions.FactoryWorldState{}, nil
}

func (lifecycleProjectionStub) SimpleDashboardRenderData(
	factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (lifecycleProjectionStub) ProjectActiveThrottlePauses(
	factorydefinitions.InitialStructurePayload,
	[]factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return nil
}

type fixedLifecycleClock struct{ now time.Time }

func (c fixedLifecycleClock) Now() time.Time { return c.now }

type lifecycleSinkFunc func(activationlifecycle.View)

func (f lifecycleSinkFunc) PresentFactoryView(view activationlifecycle.View) { f(view) }

var _ activationlifecycle.Service = (*lifecycleservice.Service)(nil)
