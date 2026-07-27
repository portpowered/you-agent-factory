package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	projectionservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/internal/service"
)

func TestStartAppliesRetainedHistoryBeforeFirstLiveDelta(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 27, 22, 0, 0, 0, time.UTC)
	live := make(chan factorydefinitions.FactoryEvent, 1)
	retained := []factorydefinitions.FactoryEvent{
		event("retained-a", 1),
		event("retained-b", 2),
	}
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: retained,
			Events:  live,
		},
		snapshot: &factoryruntime.StateSnapshot{TickCount: 2},
	}
	projected := make(chan []factorydefinitions.FactoryEvent, 1)
	projections := projectionStub{
		reconstruct: func(events []factorydefinitions.FactoryEvent, tick int) (factorydefinitions.FactoryWorldState, error) {
			projected <- append([]factorydefinitions.FactoryEvent(nil), events...)
			return factorydefinitions.FactoryWorldState{}, nil
		},
	}
	rendered := make(chan liveviewprojection.View, 1)
	svc, err := projectionservice.New(
		source,
		projections,
		fixedClock{now: now},
		liveviewprojection.SinkFunc(func(view liveviewprojection.View) { rendered <- view }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	got := <-projected
	if len(got) != len(retained) {
		t.Fatalf("retained projection events = %#v, want %d retained events", got, len(retained))
	}
	for i := range retained {
		if got[i].Id != retained[i].Id {
			t.Fatalf("retained projection[%d] id = %q, want %q", i, got[i].Id, retained[i].Id)
		}
	}
	if view := <-rendered; view.EngineState.TickCount != 2 {
		t.Fatalf("retained view tick = %d, want 2", view.EngineState.TickCount)
	}

	select {
	case <-projected:
		t.Fatal("live delta projected before any live event was sent")
	default:
	}

	live <- event("live", 3)
	if got := <-projected; len(got) != 3 || got[2].Id != "live" {
		t.Fatalf("live projection events = %#v", got)
	}
	<-rendered

	cancel()
	if err := svc.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestStartRetainsVisualizationOwnedCursorAndEventsAfterSubscribe(t *testing.T) {
	t.Parallel()

	now := time.Unix(2, 0)
	live := make(chan factorydefinitions.FactoryEvent)
	retained := []factorydefinitions.FactoryEvent{
		event("retained", 7),
	}
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: retained,
			Events:  live,
		},
		snapshot: &factoryruntime.StateSnapshot{TickCount: 7},
	}
	svc, err := projectionservice.New(
		source,
		projectionStub{},
		fixedClock{now: now},
		liveviewprojection.SinkFunc(func(liveviewprojection.View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	cursor := svc.ReconnectCursor()
	if cursor == nil || cursor.AfterEventID != retained[0].Id ||
		cursor.AfterSequence == nil || *cursor.AfterSequence != 7 {
		t.Fatalf("cursor after subscribe = %#v, want retained event cursor", cursor)
	}

	observed, err := svc.Observe(context.Background(), liveviewprojection.ObserveRequest{
		Mode: liveviewprojection.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if observed.View.RetainedEventCount != 1 {
		t.Fatalf("Observe RetainedEventCount = %d, want 1", observed.View.RetainedEventCount)
	}

	cancel()
	if err := svc.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestStartInvalidSubscriptionDoesNotLeaveHalfStartedSubscriber(t *testing.T) {
	t.Parallel()

	subscribeErr := errors.New("subscribe rejected")
	tests := []struct {
		name   string
		source *sourceStub
	}{
		{
			name: "subscribe error",
			source: &sourceStub{
				subscribeErr: subscribeErr,
				snapshot:     &factoryruntime.StateSnapshot{TickCount: 1},
			},
		},
		{
			name: "nil stream",
			source: &sourceStub{
				stream:   nil,
				snapshot: &factoryruntime.StateSnapshot{TickCount: 1},
			},
		},
		{
			name: "nil events channel",
			source: &sourceStub{
				stream: &factorydefinitions.FactoryEventStream{
					History: []factorydefinitions.FactoryEvent{event("history", 1)},
				},
				snapshot: &factoryruntime.StateSnapshot{TickCount: 1},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			presented := make(chan liveviewprojection.View, 1)
			svc, err := projectionservice.New(
				test.source,
				projectionStub{},
				fixedClock{now: time.Unix(1, 0)},
				liveviewprojection.SinkFunc(func(view liveviewprojection.View) { presented <- view }),
				nil,
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			err = svc.Start(context.Background())
			var projErr *liveviewprojection.ProjectionError
			if !errors.As(err, &projErr) || projErr.Kind != liveviewprojection.ProjectionErrorInvalidInput {
				t.Fatalf("Start() error = %v, want InvalidInput ProjectionError", err)
			}
			if errors.Is(err, liveviewprojection.ErrLiveViewProjectionAlreadyStarted) {
				t.Fatalf("Start() error = %v, must not mark service started", err)
			}
			if err := svc.Wait(context.Background()); !errors.Is(err, liveviewprojection.ErrLiveViewProjectionNotStarted) {
				t.Fatalf("Wait() after failed Start = %v, want not started", err)
			}
			select {
			case view := <-presented:
				t.Fatalf("view emitted after failed Start = %#v", view)
			default:
			}
			if cursor := svc.ReconnectCursor(); cursor != nil {
				t.Fatalf("cursor after failed Start = %#v, want nil", cursor)
			}
		})
	}
}

func TestStartSubscribesOnceForRetainedThenLive(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	live := make(chan factorydefinitions.FactoryEvent)
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{Events: live},
		snapshot: &factoryruntime.StateSnapshot{TickCount: 1},
		subscribeHook: func() { subscribeCalls++ },
	}
	svc, err := projectionservice.New(
		source,
		projectionStub{},
		fixedClock{now: time.Unix(1, 0)},
		liveviewprojection.SinkFunc(func(liveviewprojection.View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1", subscribeCalls)
	}
	if err := svc.Start(ctx); !errors.Is(err, liveviewprojection.ErrLiveViewProjectionAlreadyStarted) {
		t.Fatalf("second Start() error = %v, want already started", err)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls after duplicate Start = %d, want still 1", subscribeCalls)
	}

	cancel()
	if err := svc.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}
