package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
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

func publishDraft(kind responseevents.Kind, dispatchID string) responseevents.FactoryResponseEvent {
	payload := json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`)
	switch kind {
	case responseevents.KindReasoning:
		payload = json.RawMessage(`{"summaryDelta":"thinking"}`)
	case responseevents.KindTool:
		payload = json.RawMessage(`{"toolCallId":"call-1","outputDelta":"partial"}`)
	}
	return responseevents.FactoryResponseEvent{
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
		Payload: payload,
	}
}

func publish(t *testing.T, store *responseeventstore.SessionResponseEventStore, kind responseevents.Kind, dispatchID string) responseevents.FactoryResponseEvent {
	t.Helper()
	event, err := store.Publish(publishDraft(kind, dispatchID))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return event
}

func publishThroughService(
	t *testing.T,
	service responsestreamservice.Service,
	store *responseeventstore.SessionResponseEventStore,
	kind responseevents.Kind,
	dispatchID string,
) responseevents.FactoryResponseEvent {
	t.Helper()
	event, err := service.Publish(store, publishDraft(kind, dispatchID))
	if err != nil {
		t.Fatalf("service.Publish: %v", err)
	}
	return event
}

func TestService_PublishRetainsMonotonicSequences(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)

	first := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	second := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	third := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-2")

	if first.Sequence <= 0 || second.Sequence <= first.Sequence || third.Sequence <= second.Sequence {
		t.Fatalf("sequences = %d, %d, %d; want strictly increasing positive values", first.Sequence, second.Sequence, third.Sequence)
	}
	if first.EventID == "" || second.EventID == "" || third.EventID == "" {
		t.Fatalf("retained events missing IDs: %#v %#v %#v", first, second, third)
	}
	if first.EventID == second.EventID || second.EventID == third.EventID {
		t.Fatalf("event IDs must be unique across retained publishes")
	}
}

func TestService_SubscribeFromZeroDeliversRetainedThenLiveInOrder(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)

	first := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	second := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")

	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{
		AfterSequence: 0,
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cursor.Detach()

	retained, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next retained: %v", err)
	}
	if len(retained) != 2 || retained[0].Sequence != first.Sequence || retained[1].Sequence != second.Sequence {
		t.Fatalf("retained events = %#v, want sequences %d then %d", retained, first.Sequence, second.Sequence)
	}

	live := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	gotLive, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next live: %v", err)
	}
	if len(gotLive) != 1 || gotLive[0].Sequence != live.Sequence {
		t.Fatalf("live events = %#v, want sequence %d", gotLive, live.Sequence)
	}
}

func TestService_SubscribeReconnectsAfterKnownCursorWithOrderedFilteredEvents(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)
	first := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-2")
	third := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")

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

func TestService_SubscribeKindFilterReturnsOnlyMatchingRetainedAndLiveEvents(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)

	message := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	publishThroughService(t, service, store, responseevents.KindReasoning, "dispatch-1")
	publishThroughService(t, service, store, responseevents.KindTool, "dispatch-1")

	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{
		Kinds: []responseevents.Kind{responseevents.KindMessage},
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cursor.Detach()

	retained, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next retained: %v", err)
	}
	if len(retained) != 1 || retained[0].Sequence != message.Sequence || retained[0].Kind != responseevents.KindMessage {
		t.Fatalf("retained events = %#v, want only message sequence %d", retained, message.Sequence)
	}

	publishThroughService(t, service, store, responseevents.KindReasoning, "dispatch-1")
	liveMessage := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	gotLive, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next live: %v", err)
	}
	if len(gotLive) != 1 || gotLive[0].Sequence != liveMessage.Sequence || gotLive[0].Kind != responseevents.KindMessage {
		t.Fatalf("live events = %#v, want only message sequence %d", gotLive, liveMessage.Sequence)
	}
}

func TestService_SubscribeKindFilterStillDeliversStreamGapEvents(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)
	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 1, MaxBytes: 1 << 20}); err != nil {
		t.Fatalf("SetRetentionLimits: %v", err)
	}
	publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	retained := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")

	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{
		Kinds: []responseevents.Kind{responseevents.KindMessage},
	})
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

func TestService_SubscribeRejectsInvalidCursorAndUnsupportedKindFilter(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)

	_, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{
		AfterSequence: -1,
	})
	if !errors.Is(err, responsestreamservice.ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v, want ErrInvalidCursor", err)
	}

	_, err = service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{
		Kinds: []responseevents.Kind{responseevents.Kind("not-a-supported-kind")},
	})
	if !errors.Is(err, responsestreamservice.ErrInvalidFilter) {
		t.Fatalf("invalid filter error = %v, want ErrInvalidFilter", err)
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
