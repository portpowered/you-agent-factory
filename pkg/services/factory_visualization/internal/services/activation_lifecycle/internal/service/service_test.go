package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
)

func TestActivationLifecycleOwnerBacksRootLifecycleSlice(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	source := &lifecycleSourceStub{
		stream:   newLifecycleEventStream(),
		snapshot: &activationlifecycle.EngineObservation{TickCount: 1},
	}
	source.subscribeHook = func() { subscribeCalls++ }
	presentCalls := 0
	ownerBehavior := lifecycleservice.NewOwner(&recordingsstub.Service{})
	owner := ownerBehavior.Open(source, fixedLifecycleClock{now: time.Unix(1, 0)}, lifecycleSinkFunc(func(activationlifecycle.View) { presentCalls++ }), nil)
	var lifecycle activationlifecycle.Service = owner
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatalf("construction side effects: subscribe=%d present=%d, want inert", subscribeCalls, presentCalls)
	}

	_, err := lifecycle.Join(context.Background(), activationlifecycle.JoinRequest{})
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
	snapshot      *activationlifecycle.EngineObservation
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

func (s *lifecycleSourceStub) GetEngineObservation(context.Context) (*activationlifecycle.EngineObservation, error) {
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

type fixedLifecycleClock struct{ now time.Time }

func (c fixedLifecycleClock) Now() time.Time { return c.now }

type lifecycleSinkFunc func(activationlifecycle.View)

func (f lifecycleSinkFunc) PresentFactoryView(view activationlifecycle.View) { f(view) }

var _ activationlifecycle.Service = (*lifecycleservice.Service)(nil)

func TestSharedActivationOwnerKeepsScopedSubscriptionsIndependent(t *testing.T) {
	t.Parallel()
	owner := lifecycleservice.NewOwner(&recordingsstub.Service{})
	type scope struct {
		handle activationlifecycle.Service
		live   chan factorydefinitions.FactoryEvent
		views  chan activationlifecycle.View
	}
	scopes := make([]scope, 2)
	for index := range scopes {
		live := make(chan factorydefinitions.FactoryEvent)
		views := make(chan activationlifecycle.View, 8)
		clock := fixedLifecycleClock{now: time.Unix(int64(index+1), 0)}
		handle := owner.Open(&lifecycleSourceStub{
			stream:   &factorydefinitions.FactoryEventStream{History: []factorydefinitions.FactoryEvent{{Id: "history", Context: factorydefinitions.FactoryEventContext{Sequence: index + 1}}}, Events: live},
			snapshot: &activationlifecycle.EngineObservation{TickCount: index + 1},
		}, clock, lifecycleSinkFunc(func(view activationlifecycle.View) { views <- view }), nil)
		scopes[index] = scope{handle: handle, live: live, views: views}
		t.Cleanup(func() { _, _ = handle.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{}) })
		select {
		case view := <-views:
			t.Fatalf("inert scope emitted %#v", view)
		default:
		}
		if _, err := handle.Activate(context.Background(), activationlifecycle.ActivateRequest{Mode: activationlifecycle.ActivateModeRetainedThenLive}); err != nil {
			t.Fatal(err)
		}
		view := <-views
		if view.EngineObservation.TickCount != index+1 || !view.ObservedAt.Equal(clock.now) {
			t.Fatalf("selected scope view = %#v", view)
		}
	}
	for range 2 {
		if _, err := scopes[0].handle.StopDrain(context.Background(), activationlifecycle.StopDrainRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := scopes[0].handle.Join(context.Background(), activationlifecycle.JoinRequest{}); err != nil {
		t.Fatal(err)
	}
	scopes[1].live <- factorydefinitions.FactoryEvent{Id: "peer-live", Context: factorydefinitions.FactoryEventContext{Sequence: 3}}
	<-scopes[1].views
	for index, scope := range scopes {
		if retained := scope.handle.RetainedEvents(); len(retained) != index+1 {
			t.Fatalf("scope %d retained = %#v", index, retained)
		}
	}
	if cursor := scopes[1].handle.ReconnectCursor(); cursor == nil || cursor.AfterEventID != "peer-live" {
		t.Fatalf("peer cursor = %#v", cursor)
	}
}
