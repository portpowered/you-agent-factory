package service_test

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	projectionservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/internal/service"
)

func assertCursor(
	t *testing.T,
	cursor *factorydefinitions.FactoryEventReconnectCursor,
	eventID string,
	sequence int,
) {
	t.Helper()
	if cursor == nil {
		t.Fatalf("cursor = nil, want after %q sequence %d", eventID, sequence)
	}
	if cursor.AfterEventID != eventID {
		t.Fatalf("cursor.AfterEventID = %q, want %q", cursor.AfterEventID, eventID)
	}
	if cursor.AfterSequence == nil || *cursor.AfterSequence != sequence {
		t.Fatalf("cursor.AfterSequence = %v, want %d", cursor.AfterSequence, sequence)
	}
}

func assertMonotonicReconstructCalls(
	t *testing.T,
	calls [][]factorydefinitions.FactoryEvent,
) {
	t.Helper()
	for i, events := range calls {
		seen := make(map[string]int)
		for j, event := range events {
			if prior, ok := seen[event.Id]; ok {
				t.Fatalf("reconstruct call %d: duplicate event id %q at indices %d and %d", i, event.Id, prior, j)
			}
			seen[event.Id] = j
		}
		if i == 0 {
			continue
		}
		prev := calls[i-1]
		if len(events) < len(prev) {
			t.Fatalf("reconstruct call %d: event count %d < prior %d", i, len(events), len(prev))
		}
		for j := range prev {
			if events[j].Id != prev[j].Id {
				t.Fatalf("reconstruct call %d: prefix mismatch at %d, got %q want %q", i, j, events[j].Id, prev[j].Id)
			}
		}
	}
}

func TestRetainedOnlyStartCursorPointsAtLastRetainedEvent(t *testing.T) {
	t.Parallel()

	retained := []factorydefinitions.FactoryEvent{
		event("retained-a", 10),
		event("retained-b", 11),
	}
	live := make(chan factorydefinitions.FactoryEvent)
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: retained,
			Events:  live,
		},
		snapshot: snapshotFacts(11),
	}
	svc, err := projectionservice.New(
		source,
		newProjectionStub(),
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

	assertCursor(t, svc.ReconnectCursor(), "retained-b", 11)

	observed, err := svc.Observe(context.Background(), liveviewprojection.ObserveRequest{
		Mode: liveviewprojection.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if observed.View.RetainedEventCount != 2 {
		t.Fatalf("Observe RetainedEventCount = %d, want 2", observed.View.RetainedEventCount)
	}

	cancel()
	if err := svc.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestRetainedThenOneLiveAdvanceContinuesSameCursorAuthority(t *testing.T) {
	t.Parallel()

	retained := []factorydefinitions.FactoryEvent{event("retained", 1)}
	live := make(chan factorydefinitions.FactoryEvent, 1)
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: retained,
			Events:  live,
		},
		snapshot: snapshotFacts(1),
	}
	projected := make(chan []factorydefinitions.FactoryEvent, 2)
	projections := newTrackingProjectionStub(projected)
	rendered := make(chan liveviewprojection.View, 2)
	svc, err := projectionservice.New(
		source,
		projections,
		fixedClock{now: time.Unix(2, 0)},
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

	assertCursor(t, svc.ReconnectCursor(), "retained", 1)
	<-projected
	<-rendered

	liveEvent := event("live-one", 2)
	live <- liveEvent
	if got := <-projected; len(got) != 2 || got[1].Id != liveEvent.Id {
		t.Fatalf("live projection events = %#v", got)
	}
	<-rendered

	assertCursor(t, svc.ReconnectCursor(), "live-one", 2)

	observed, err := svc.Observe(context.Background(), liveviewprojection.ObserveRequest{
		Mode: liveviewprojection.ObserveModeRetainedThenLive,
		Reconnect: &liveviewprojection.ObserveReconnectCursor{
			AfterEventID:  "live-one",
			AfterSequence: ptrInt(2),
		},
	})
	if err != nil {
		t.Fatalf("Observe() with live cursor error = %v", err)
	}
	if observed.View.RetainedEventCount != 2 {
		t.Fatalf("Observe RetainedEventCount = %d, want 2", observed.View.RetainedEventCount)
	}

	cancel()
	if err := svc.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestMultiLiveAdvanceAfterRetainedHistoryContinuesCursor(t *testing.T) {
	t.Parallel()

	retained := []factorydefinitions.FactoryEvent{
		event("retained-a", 1),
		event("retained-b", 2),
	}
	live := make(chan factorydefinitions.FactoryEvent, 3)
	projections := newCursorContinuityProjection()
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: retained,
			Events:  live,
		},
		snapshot: snapshotFacts(2),
	}
	rendered := make(chan liveviewprojection.View, 4)
	svc, err := projectionservice.New(
		source,
		projections,
		fixedClock{now: time.Unix(3, 0)},
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

	assertCursor(t, svc.ReconnectCursor(), "retained-b", 2)
	if view := <-rendered; view.RetainedEventCount != 2 {
		t.Fatalf("retained view RetainedEventCount = %d, want 2", view.RetainedEventCount)
	}

	liveEvents := []factorydefinitions.FactoryEvent{
		event("live-a", 3),
		event("live-b", 4),
		event("live-c", 5),
	}
	for i, liveEvent := range liveEvents {
		live <- liveEvent
		if view := <-rendered; view.RetainedEventCount != 2+i+1 {
			t.Fatalf("live view %d RetainedEventCount = %d, want %d", i, view.RetainedEventCount, 2+i+1)
		}
		assertCursor(t, svc.ReconnectCursor(), liveEvent.Id, liveEvent.Context.Sequence)
	}

	assertMonotonicReconstructCalls(t, projections.reconstructCalls)

	cancel()
	if err := svc.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestLiveDeltasDoNotDuplicateRetainedReplay(t *testing.T) {
	t.Parallel()

	retained := []factorydefinitions.FactoryEvent{
		event("retained-a", 1),
		event("retained-b", 2),
	}
	live := make(chan factorydefinitions.FactoryEvent, 2)
	projections := newCursorContinuityProjection()
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: retained,
			Events:  live,
		},
		snapshot: snapshotFacts(2),
	}
	subscribeCalls := 0
	source.subscribeHook = func() { subscribeCalls++ }

	svc, err := projectionservice.New(
		source,
		projections,
		fixedClock{now: time.Unix(4, 0)},
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
		t.Fatalf("subscribe calls after Start = %d, want 1", subscribeCalls)
	}

	waitReconstructCalls := func(min int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for len(projections.reconstructCalls) < min {
			if time.Now().After(deadline) {
				t.Fatalf("reconstruct calls = %d, want at least %d", len(projections.reconstructCalls), min)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	waitReconstructCalls(1)
	live <- event("live-a", 3)
	waitReconstructCalls(2)
	live <- event("live-b", 4)
	waitReconstructCalls(3)

	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls after live deltas = %d, want still 1", subscribeCalls)
	}
	assertMonotonicReconstructCalls(t, projections.reconstructCalls)
	if len(projections.reconstructCalls) < 3 {
		t.Fatalf("reconstruct calls = %d, want at least retained + 2 live projections", len(projections.reconstructCalls))
	}
	last := projections.reconstructCalls[len(projections.reconstructCalls)-1]
	if len(last) != 4 {
		t.Fatalf("final reconstruct event count = %d, want 4 cumulative events", len(last))
	}
	for i, wantID := range []string{"retained-a", "retained-b", "live-a", "live-b"} {
		if last[i].Id != wantID {
			t.Fatalf("final reconstruct[%d] id = %q, want %q", i, last[i].Id, wantID)
		}
	}

	cancel()
	if err := svc.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func ptrInt(v int) *int { return &v }
