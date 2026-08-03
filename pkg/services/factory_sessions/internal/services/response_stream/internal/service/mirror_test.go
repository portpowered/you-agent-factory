package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	events "github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
)

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
