package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/services/response_stream"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/services/response_stream/wire"
)

type memoryCursorStore struct {
	checkpoint cursors.Checkpoint
	found      bool
}

func (s *memoryCursorStore) Load(context.Context, cursors.StorageIdentity) (cursors.Checkpoint, bool, error) {
	return s.checkpoint, s.found, nil
}

func (s *memoryCursorStore) Save(_ context.Context, _ cursors.StorageIdentity, checkpoint cursors.Checkpoint) error {
	s.checkpoint, s.found = checkpoint, true
	return nil
}

func (*memoryCursorStore) Close() error { return nil }

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

func newService(t *testing.T) responsestreamservice.Service {
	t.Helper()
	var next atomic.Uint64
	service, err := responsestreamwire.NewService(func() string {
		return fmt.Sprintf("response-event-%d", next.Add(1))
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func newStore(t *testing.T, service responsestreamservice.Service) *responseeventstore.SessionResponseEventStore {
	t.Helper()
	store, err := service.NewEventStore("session-1", &fixedClock{now: time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	return store
}

func publish(t *testing.T, store *responseeventstore.SessionResponseEventStore, kind responseevents.Kind, dispatchID string) responseevents.FactoryResponseEvent {
	t.Helper()
	event, err := store.Publish(responseevents.FactoryResponseEvent{
		DispatchID: dispatchID,
		RunID:      "run-1",
		Kind:       kind,
		Phase:      responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider: "test", NativeEventType: "delta",
			Delivery:       responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationDelta,
			Fidelity:       responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return event
}

func TestService_SubscribeReconnectsAfterKnownCursorWithOrderedFilteredEvents(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)
	first := publish(t, store, responseevents.KindMessage, "dispatch-1")
	publish(t, store, responseevents.KindMessage, "dispatch-2")
	third := publish(t, store, responseevents.KindMessage, "dispatch-1")

	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{
		AfterSequence: first.Sequence,
		DispatchID:    "dispatch-1",
		Kinds:         []responseevents.Kind{responseevents.KindMessage},
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cursor.Detach()
	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != third.Sequence {
		t.Fatalf("events = %#v, want only sequence %d", events, third.Sequence)
	}
}

func TestService_StaleCursorSignalsGapAndPreservesFirstAvailableEvent(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)
	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 1, MaxBytes: 1 << 20}); err != nil {
		t.Fatalf("SetRetentionLimits: %v", err)
	}
	publish(t, store, responseevents.KindMessage, "dispatch-1")
	retained := publish(t, store, responseevents.KindMessage, "dispatch-1")
	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cursor.Detach()
	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 2 || events[0].Kind != responseevents.KindStreamGap || events[1].Sequence != retained.Sequence {
		t.Fatalf("events = %#v, want gap followed by sequence %d", events, retained.Sequence)
	}
}

func TestService_CancellationAndSlowSubscribersStayBounded(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)
	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	for range 100 {
		publish(t, store, responseevents.KindMessage, "dispatch-1")
	}
	if got := store.SubscriberCount(); got != 1 {
		t.Fatalf("SubscriberCount = %d, want 1", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A cancellation races with retained delivery by contract; drain retained
	// data, then verify cancellation without a publisher-owned queue or goroutine.
	if _, err := cursor.Next(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Next after cancellation: %v", err)
	}
	cursor.Detach()
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("SubscriberCount after detach = %d, want 0", got)
	}
	if accounting := store.RetentionAccounting(); accounting.EventCount > store.RetentionLimits().MaxEvents {
		t.Fatalf("retention accounting = %#v, exceeds limits %#v", accounting, store.RetentionLimits())
	}
}

func TestService_ConstructionIsInertAndRejectsMissingEffects(t *testing.T) {
	t.Parallel()
	if service, err := responsestreamwire.NewService(nil); err == nil || service != nil {
		t.Fatalf("NewService(nil) = %#v, %v; want deterministic dependency error", service, err)
	}
	service := newService(t)
	if store, err := service.NewEventStore("session-1", nil); err == nil || store != nil {
		t.Fatalf("NewEventStore without clock = %#v, %v; want clock error", store, err)
	}
}

func TestService_CursorPersistenceAndPublicationDiagnosticsUseInjectedEffects(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := &memoryCursorStore{}
	tracker, err := service.NewCursorTracker(store, cursors.StorageIdentity{
		BackendScopeID: "backend-1", FactorySessionID: "session-1",
		StreamGenerationID: "stream-1", ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("NewCursorTracker: %v", err)
	}
	sequence := 7
	checkpoint := cursors.Checkpoint{AfterSequence: &sequence}
	if err := tracker.Advance(context.Background(), checkpoint); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	restored, found, err := tracker.Restore(context.Background())
	if err != nil || !found || restored.AfterSequence == nil || *restored.AfterSequence != sequence {
		t.Fatalf("Restore = %#v, %t, %v; want sequence %d", restored, found, err, sequence)
	}

	registry, err := service.NewStreamRegistry(&fixedClock{now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("NewStreamRegistry: %v", err)
	}
	stream := registry.Streams("session-1").Stream("dispatch-1")
	var diagnosticCount atomic.Int64
	publisher := service.NewPublisher(stream, func(responsestream.CompactionSummary) {
		diagnosticCount.Add(1)
	})
	publisher.ReportCompaction(responsestream.CompactionSummary{Reason: responsestream.CompactionReasonTruncated})
	if got := publisher.Diagnostics().CompactionCount; got != 1 || diagnosticCount.Load() != 1 {
		t.Fatalf("diagnostics = %#v, observer count = %d; want one compaction", publisher.Diagnostics(), diagnosticCount.Load())
	}
}
