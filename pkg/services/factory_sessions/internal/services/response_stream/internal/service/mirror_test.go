package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	events "github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
)

// rejectingEventsService is a minimal events.Service test double whose
// Append always fails, used to prove Publish cannot produce a locally-only
// record when the injected Events root rejects the write: the two surfaces
// must never be able to diverge, not even under an Events failure.
type rejectingEventsService struct {
	appendCalls atomic.Int64
}

var errRejectingEventsServiceAppend = errors.New("rejectingEventsService: Append always fails")

func (r *rejectingEventsService) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	r.appendCalls.Add(1)
	return events.AppendResult{}, errRejectingEventsServiceAppend
}

func (r *rejectingEventsService) AttachSource(context.Context, events.AttachSourceRequest) (events.AttachSourceResult, error) {
	return events.AttachSourceResult{}, errRejectingEventsServiceAppend
}

func (r *rejectingEventsService) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, errRejectingEventsServiceAppend
}

func (r *rejectingEventsService) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return nil, errRejectingEventsServiceAppend
}

var _ events.Service = (*rejectingEventsService)(nil)

func publishThroughService(
	t *testing.T,
	service responsestreamservice.Service,
	store *responseeventstore.SessionResponseEventStore,
	kind responseevents.Kind,
	dispatchID string,
) responseevents.FactoryResponseEvent {
	t.Helper()
	event, err := service.Publish(store, responseevents.FactoryResponseEvent{
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
		t.Fatalf("service.Publish: %v", err)
	}
	return event
}

// responseEventTopicForTest reconstructs the session-scoped Events topic the
// response-stream service mirrors into, matching the documented convention in
// pkg/services/events/identity.go ("factory-session/<id>/response-events").
func responseEventTopicForTest(sessionID string) events.Topic {
	return events.Topic(fmt.Sprintf("factory-session/%s/response-events", sessionID))
}

// TestService_PublishMirrorsIntoInjectedEventsRootWithMatchingAggregatePosition
// proves that every response event the service accepts into its session-owned
// store is also published into the exact injected events.Service instance, on
// a session-scoped topic, carrying the same identity and content -- so a
// direct Events read and the Factory Sessions compatibility surface observe
// the same underlying accepted records and aggregate ordering.
func TestService_PublishMirrorsIntoInjectedEventsRootWithMatchingAggregatePosition(t *testing.T) {
	t.Parallel()

	eventsService := newTestEventsService(t)
	service, err := serviceWithEvents(t, eventsService)
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store := newStore(t, service)

	const publishCount = 5
	var published []responseevents.FactoryResponseEvent
	for index := range publishCount {
		published = append(published, publishThroughService(t, service, store, responseevents.KindMessage, fmt.Sprintf("dispatch-%d", index)))
	}

	ctx := context.Background()
	read, err := eventsService.Read(ctx, events.ReadRequest{
		Topic: responseEventTopicForTest(store.FactorySessionID()),
		From:  events.Cursor{Topic: responseEventTopicForTest(store.FactorySessionID())},
		Limit: publishCount + 1,
	})
	if err != nil {
		t.Fatalf("Read() on injected Events root error = %v", err)
	}
	if read.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Read() outcome = %v, want ReadOutcomeProgress", read.Outcome)
	}
	if len(read.Records) != publishCount {
		t.Fatalf("Read() records = %d, want %d", len(read.Records), publishCount)
	}

	for index, record := range read.Records {
		want := published[index]
		if uint64(record.ID.Position) != uint64(want.Sequence) {
			t.Fatalf("record %d Events position = %d, want store sequence %d", index, record.ID.Position, want.Sequence)
		}
		if string(record.SourceEventID) != want.EventID {
			t.Fatalf("record %d SourceEventID = %q, want %q", index, record.SourceEventID, want.EventID)
		}
		var decoded responseevents.FactoryResponseEvent
		if err := json.Unmarshal(record.Payload, &decoded); err != nil {
			t.Fatalf("decode mirrored payload %d: %v", index, err)
		}
		if decoded.EventID != want.EventID || decoded.Sequence != want.Sequence || decoded.DispatchID != want.DispatchID {
			t.Fatalf("decoded mirrored event %d = %#v, want event ID %q sequence %d dispatch %q", index, decoded, want.EventID, want.Sequence, want.DispatchID)
		}
	}
}

// TestService_ConcurrentPublishesKeepEventsPositionsInLockstepWithStoreSequence
// proves the publish/mirror pairing stays correctly ordered under concurrent
// producers on the same session: the injected Events root never assigns
// positions out of step with the store's own commit-ordered sequence, even
// when publishers race.
func TestService_ConcurrentPublishesKeepEventsPositionsInLockstepWithStoreSequence(t *testing.T) {
	t.Parallel()

	eventsService := newTestEventsService(t)
	service, err := serviceWithEvents(t, eventsService)
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store := newStore(t, service)

	const producers = 8
	const perProducer = 10
	var wg sync.WaitGroup
	var counter atomic.Int64
	for range producers {
		wg.Go(func() {
			for range perProducer {
				n := counter.Add(1)
				publishThroughService(t, service, store, responseevents.KindMessage, fmt.Sprintf("dispatch-%d", n))
			}
		})
	}
	wg.Wait()

	want := producers * perProducer
	ctx := context.Background()
	topic := responseEventTopicForTest(store.FactorySessionID())
	read, err := eventsService.Read(ctx, events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: want + 1,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(read.Records) != want {
		t.Fatalf("Read() records = %d, want %d", len(read.Records), want)
	}
	for index, record := range read.Records {
		wantPosition := uint64(index + 1)
		if uint64(record.ID.Position) != wantPosition {
			t.Fatalf("record %d Events position = %d, want %d (contiguous commit order)", index, record.ID.Position, wantPosition)
		}
	}
	if store.LatestSequence() != int64(want) {
		t.Fatalf("store.LatestSequence() = %d, want %d", store.LatestSequence(), want)
	}
}

// TestService_PublishFailsAtomicallyWhenEventsRejectsTheAppend proves Events
// is the authority the local store's own record depends on, not a
// best-effort shadow copy: when the injected Events root's Append fails, the
// whole Publish call fails and the session-owned store is left completely
// untouched (no sequence assigned, no subscriber notified) -- the two
// surfaces can never observe different records because Events accepting the
// write is a precondition for the store ever seeing it, not an
// after-the-fact side effect that can silently fail alone.
func TestService_PublishFailsAtomicallyWhenEventsRejectsTheAppend(t *testing.T) {
	t.Parallel()

	rejecting := &rejectingEventsService{}
	service, err := serviceWithEvents(t, rejecting)
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store := newStore(t, service)

	subscribed, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscribed.Detach()

	if _, err := service.Publish(store, responseevents.FactoryResponseEvent{
		DispatchID: "dispatch-1",
		RunID:      "run-1",
		Kind:       responseevents.KindMessage,
		Phase:      responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider: "test", NativeEventType: "delta",
			Delivery:       responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationDelta,
			Fidelity:       responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`),
	}); !errors.Is(err, errRejectingEventsServiceAppend) {
		t.Fatalf("Publish() error = %v, want it to wrap the Events rejection", err)
	}

	if rejecting.appendCalls.Load() != 1 {
		t.Fatalf("Events Append call count = %d, want exactly 1", rejecting.appendCalls.Load())
	}
	if got := store.LatestSequence(); got != 0 {
		t.Fatalf("store.LatestSequence() = %d, want 0 (a rejected Publish must never assign a local sequence)", got)
	}
	if events := store.Events(); len(events) != 0 {
		t.Fatalf("store.Events() = %d, want 0 (a rejected Publish must never retain a local record)", len(events))
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got, err := subscribed.Next(drainCtx)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Next() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("subscriber observed %d events after a rejected Publish, want 0 (no partial/local-only notification)", len(got))
	}
}

// TestService_ConcurrentPublishAndCompletionNeverDivergesFromEvents races
// Publish against Complete/Close on the same store -- the exact interleaving
// the earlier two-phase-commit design could lose (an Events-accepted record
// whose subsequent local store write was rejected by intervening completion
// or closure). Publish now holds the store's write lock across the entire
// Events.Append call, so Complete/Close cannot interleave inside a single
// publish's critical section; this test proves that holds under real
// concurrency by asserting every record accepted by Events for this topic
// is also retained by the store, with no exceptions.
func TestService_ConcurrentPublishAndCompletionNeverDivergesFromEvents(t *testing.T) {
	t.Parallel()

	eventsService := newTestEventsService(t)
	service, err := serviceWithEvents(t, eventsService)
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store := newStore(t, service)

	const producers = 8
	const perProducer = 25
	var wg sync.WaitGroup
	var accepted atomic.Int64
	for range producers {
		wg.Go(func() {
			for i := range perProducer {
				if _, err := service.Publish(store, responseevents.FactoryResponseEvent{
					DispatchID: fmt.Sprintf("dispatch-%d", i),
					RunID:      "run-1",
					Kind:       responseevents.KindMessage,
					Phase:      responseevents.PhaseDelta,
					Provenance: responseevents.Provenance{
						Provider: "test", NativeEventType: "delta",
						Delivery:       responseevents.DeliveryNativeStream,
						Representation: responseevents.RepresentationDelta,
						Fidelity:       responseevents.FidelityLossless,
					},
					Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`),
				}); err == nil {
					accepted.Add(1)
				}
			}
		})
	}
	wg.Go(func() {
		time.Sleep(time.Millisecond)
		service.Complete(store)
	})
	wg.Wait()

	ctx := context.Background()
	topic := responseEventTopicForTest(store.FactorySessionID())
	read, err := eventsService.Read(ctx, events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: producers*perProducer + 1,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	storeEvents := store.Events()
	storeSequences := make(map[int64]bool, len(storeEvents))
	for _, event := range storeEvents {
		storeSequences[event.Sequence] = true
	}
	for _, record := range read.Records {
		if !storeSequences[int64(record.ID.Position)] {
			t.Fatalf("Events accepted position %d for this topic, but the store never retained a matching record: divergence between the two surfaces", record.ID.Position)
		}
	}
	if int64(len(read.Records)) != accepted.Load() {
		t.Fatalf("Events retained %d records, want exactly %d (every Publish call this test observed as accepted)", len(read.Records), accepted.Load())
	}
}

// TestService_PublishNeverDivergesFromEventsWhenTheTopicIsNotAligned
// reproduces the exact non-aligned-topic shape a real Events topic can carry
// across two different store instances for the same session identity (for
// example, a second SessionResponseEventStore created for a session whose
// Events topic already has history from a prior store instance): the topic
// already has a foreign record at position 1 before this test's store ever
// publishes anything, so the store's own predicted sequenceHint (1) cannot
// match what Events actually assigns (2). Publish must still succeed and the
// store must retain the record under Events' real assigned position -- not
// reject it and leave Events with a record the store never observed.
func TestService_PublishNeverDivergesFromEventsWhenTheTopicIsNotAligned(t *testing.T) {
	t.Parallel()

	eventsService := newTestEventsService(t)
	service, err := serviceWithEvents(t, eventsService)
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store := newStore(t, service)
	topic := responseEventTopicForTest(store.FactorySessionID())

	ctx := context.Background()
	if _, err := eventsService.Append(ctx, events.AppendRequest{
		Topic:          topic,
		SourceType:     "factory-session-response-event",
		SourceID:       events.SourceID(store.FactorySessionID()),
		SourceSequence: 1,
		SourceEventID:  "pre-existing-from-another-store-instance",
		SchemaID:       "factory-response-event.v1",
		Payload:        json.RawMessage(`{"foreign":"record"}`),
	}); err != nil {
		t.Fatalf("pre-populate topic with a foreign record: %v", err)
	}

	published := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	if published.Sequence != 2 {
		t.Fatalf("published.Sequence = %d, want 2 (adopted from Events, not the store's own predicted hint of 1)", published.Sequence)
	}
	if store.LatestSequence() != 2 {
		t.Fatalf("store.LatestSequence() = %d, want 2", store.LatestSequence())
	}

	read, err := eventsService.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(read.Records) != 2 {
		t.Fatalf("Read() records = %d, want 2 (the pre-existing foreign record plus this test's publish)", len(read.Records))
	}
	if uint64(read.Records[1].ID.Position) != 2 || string(read.Records[1].SourceEventID) != published.EventID {
		t.Fatalf("Events record at position 2 = %#v, want it to carry this store's published identity %q", read.Records[1], published.EventID)
	}

	stored, ok := store.EventAtSequence(2)
	if !ok {
		t.Fatalf("store.EventAtSequence(2) not found: the store never retained the record Events accepted at position 2")
	}
	if stored.EventID != published.EventID {
		t.Fatalf("stored.EventID = %q, want %q: store and Events must observe the same record at the same position", stored.EventID, published.EventID)
	}
}
