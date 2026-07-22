package responseeventstore_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseeventstore"
)

var responseEventIdentity atomic.Uint64

func testResponseEventID() string {
	return fmt.Sprintf("response-event-%d", responseEventIdentity.Add(1))
}

func newResponseEventStore(factorySessionID string) *responseeventstore.SessionResponseEventStore {
	return responseeventstore.NewSessionResponseEventStore(factorySessionID, platformclock.Real{}, testResponseEventID)
}

func newResponseEventStoreWithLimits(
	factorySessionID string,
	limits responseeventstore.RetentionLimits,
) (*responseeventstore.SessionResponseEventStore, error) {
	return responseeventstore.NewSessionResponseEventStoreWithLimits(
		factorySessionID,
		platformclock.Real{},
		limits,
		testResponseEventID,
	)
}

type fixedClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fixedClock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func samplePublishInput() responseevents.FactoryResponseEvent {
	return responseevents.FactoryResponseEvent{
		RunID: "run-test",
		Kind:  responseevents.KindMessage,
		Phase: responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:        "example-provider",
			NativeEventType: "message.delta",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationDelta,
			Fidelity:        responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`),
	}
}

func TestSessionResponseEventStoreWithClockAndLimits_RequiresSelectedClock(t *testing.T) {
	t.Parallel()

	store, err := responseeventstore.NewSessionResponseEventStoreWithClockAndLimits(
		"session-abc",
		nil,
		responseeventstore.DefaultRetentionLimits(),
		testResponseEventID,
	)
	if err == nil || store != nil {
		t.Fatalf("store, err = %#v, %v; want nil store and clock validation error", store, err)
	}
}

func TestSessionResponseEventStore_RequiresExplicitClock(t *testing.T) {
	t.Parallel()

	if store := responseeventstore.NewSessionResponseEventStore("session-abc", nil, testResponseEventID); store != nil {
		t.Fatalf("NewSessionResponseEventStore without clock = %#v, want nil", store)
	}
	store, err := responseeventstore.NewSessionResponseEventStoreWithLimits(
		"session-abc",
		nil,
		responseeventstore.DefaultRetentionLimits(),
		testResponseEventID,
	)
	if err == nil || store != nil {
		t.Fatalf("NewSessionResponseEventStoreWithLimits without clock = %#v, %v; want validation error", store, err)
	}
}

func TestSessionResponseEventStore_RequiresExplicitResponseEventIDGenerator(t *testing.T) {
	t.Parallel()

	store, err := responseeventstore.NewSessionResponseEventStoreWithClockAndLimits(
		"session-abc",
		platformclock.Real{},
		responseeventstore.DefaultRetentionLimits(),
		nil,
	)
	if err == nil || store != nil {
		t.Fatalf("store, err = %#v, %v; want nil store and generator validation error", store, err)
	}
}

func TestSessionResponseEventStore_RejectsEmptyGeneratedResponseEventIDWithoutConsumingSequence(t *testing.T) {
	t.Parallel()

	store := responseeventstore.NewSessionResponseEventStore(
		"session-abc",
		platformclock.Real{},
		func() string { return "  " },
	)
	if _, err := store.Publish(samplePublishInput()); err == nil {
		t.Fatal("Publish with empty generated response event ID = nil error")
	}
	if got := store.LatestSequence(); got != 0 {
		t.Fatalf("LatestSequence() = %d, want 0 after rejected identity", got)
	}
}

func TestSessionResponseEventStore_PublishAssignsMonotonicSequenceAndEventID(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	store := responseeventstore.NewSessionResponseEventStoreWithClock("session-abc", &fixedClock{now: start}, testResponseEventID)

	first, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	second, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}

	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences = [%d %d], want [1 2]", first.Sequence, second.Sequence)
	}
	if first.EventID == "" || second.EventID == "" {
		t.Fatal("published events must have non-empty event IDs")
	}
	if first.EventID == second.EventID {
		t.Fatalf("event IDs must be unique: %q", first.EventID)
	}
	if first.SchemaVersion != responseevents.SchemaVersionV1 || second.SchemaVersion != responseevents.SchemaVersionV1 {
		t.Fatalf("schema versions = [%q %q], want %q", first.SchemaVersion, second.SchemaVersion, responseevents.SchemaVersionV1)
	}
	if first.FactorySessionID != "session-abc" || second.FactorySessionID != "session-abc" {
		t.Fatalf("factory session IDs = [%q %q], want session-abc", first.FactorySessionID, second.FactorySessionID)
	}
	if !first.RecordedAt.Equal(start) || !second.RecordedAt.Equal(start) {
		t.Fatalf("recordedAt = [%s %s], want %s", first.RecordedAt, second.RecordedAt, start)
	}
	if store.LatestSequence() != 2 {
		t.Fatalf("latest sequence = %d, want 2", store.LatestSequence())
	}
}

func TestSessionResponseEventStore_FactorySessionID(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	if got := store.FactorySessionID(); got != "session-abc" {
		t.Fatalf("FactorySessionID() = %q, want session-abc", got)
	}
}

func TestSessionResponseEventStore_PublishStoredEventIsImmutableAfterCallerMutation(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	input := samplePublishInput()

	published, err := store.Publish(input)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	input.Payload = json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"mutated"}`)
	input.EventID = "caller-mutated"
	input.Sequence = 99

	stored, ok := store.EventAtSequence(published.Sequence)
	if !ok {
		t.Fatalf("event at sequence %d not found", published.Sequence)
	}
	if stored.EventID != published.EventID || stored.Sequence != published.Sequence {
		t.Fatalf("identity changed after caller mutation: got (%q, %d), want (%q, %d)",
			stored.EventID, stored.Sequence, published.EventID, published.Sequence)
	}
	if string(stored.Payload) != string(published.Payload) {
		t.Fatalf("payload changed after caller mutation:\nstored=%s\npublished=%s", stored.Payload, published.Payload)
	}
}

func TestSessionResponseEventStore_PublishConcurrentProducesUniqueSequencesAndEventIDs(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	const workers = 32

	var wg sync.WaitGroup
	wg.Add(workers)
	results := make(chan responseevents.FactoryResponseEvent, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			published, err := store.Publish(samplePublishInput())
			if err != nil {
				t.Errorf("publish: %v", err)
				return
			}
			results <- published
		}()
	}
	wg.Wait()
	close(results)

	sequences := make(map[int64]struct{}, workers)
	eventIDs := make(map[string]struct{}, workers)
	for published := range results {
		if published.Sequence <= 0 {
			t.Fatalf("invalid sequence %d", published.Sequence)
		}
		if published.EventID == "" {
			t.Fatal("published event missing event ID")
		}
		if _, exists := sequences[published.Sequence]; exists {
			t.Fatalf("duplicate sequence %d", published.Sequence)
		}
		sequences[published.Sequence] = struct{}{}
		if _, exists := eventIDs[published.EventID]; exists {
			t.Fatalf("duplicate event ID %q", published.EventID)
		}
		eventIDs[published.EventID] = struct{}{}
	}
	if len(sequences) != workers {
		t.Fatalf("sequence count = %d, want %d", len(sequences), workers)
	}
	if store.LatestSequence() != workers {
		t.Fatalf("latest sequence = %d, want %d", store.LatestSequence(), workers)
	}
}

func TestSessionResponseEventStore_PublishRejectsInvalidEvent(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	input := samplePublishInput()
	input.Phase = responseevents.PhaseUpdated

	if _, err := store.Publish(input); err == nil {
		t.Fatal("expected validation error for invalid kind/phase pair")
	}
	if events := store.Events(); len(events) != 0 {
		t.Fatalf("invalid publish retained events = %#v, want empty buffer", events)
	}
}

func TestSessionResponseEventStore_PublishRejectsForeignFactorySessionID(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	input := samplePublishInput()
	input.FactorySessionID = "session-other"

	if _, err := store.Publish(input); !errors.Is(err, responseeventstore.ErrFactorySessionMismatch) {
		t.Fatalf("publish foreign session error = %v, want ErrFactorySessionMismatch", err)
	}
	if events := store.Events(); len(events) != 0 {
		t.Fatalf("foreign-session publish retained events = %#v, want empty buffer", events)
	}
	if latest := store.LatestSequence(); latest != 0 {
		t.Fatalf("latest sequence = %d, want 0 after rejected publish", latest)
	}
}

func TestSessionResponseEventStore_EventsSnapshotPreservesAscendingOrder(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	for range 3 {
		if _, err := store.Publish(samplePublishInput()); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	events := store.Events()
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	for index, event := range events {
		wantSequence := int64(index + 1)
		if event.Sequence != wantSequence {
			t.Fatalf("events[%d].Sequence = %d, want %d", index, event.Sequence, wantSequence)
		}
	}
}

func TestSessionResponseEventStore_EventAtSequenceReturnsOnlyRetainedEvent(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	published, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	retained, ok := store.EventAtSequence(published.Sequence)
	if !ok {
		t.Fatalf("EventAtSequence(%d) did not find published event", published.Sequence)
	}
	if retained.EventID != published.EventID {
		t.Fatalf("event id = %q, want %q", retained.EventID, published.EventID)
	}
	if _, ok := store.EventAtSequence(published.Sequence + 1); ok {
		t.Fatalf("EventAtSequence(%d) found an unpublished event", published.Sequence+1)
	}
}

func TestSessionResponseEventStore_DoesNotDropPublishedEvents(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	for range 128 {
		if _, err := store.Publish(samplePublishInput()); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	if events := store.Events(); len(events) != 128 {
		t.Fatalf("retained event count = %d, want 128 without retention eviction", len(events))
	}
}
