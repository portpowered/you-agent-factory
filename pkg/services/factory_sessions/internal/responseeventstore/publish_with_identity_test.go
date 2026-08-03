package responseeventstore_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

func TestSessionResponseEventStore_PrepareInputNormalizesWithoutMutatingState(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	store := responseeventstore.NewSessionResponseEventStoreWithClock("session-abc", &fixedClock{now: start}, testResponseEventID)

	input := samplePublishInput()
	input.SchemaVersion = ""
	input.FactorySessionID = ""
	input.Sequence = 42
	input.EventID = "caller-supplied"

	prepared := store.PrepareInput(input)

	if prepared.SchemaVersion != responseevents.SchemaVersionV1 {
		t.Fatalf("SchemaVersion = %q, want %q", prepared.SchemaVersion, responseevents.SchemaVersionV1)
	}
	if prepared.FactorySessionID != "session-abc" {
		t.Fatalf("FactorySessionID = %q, want session-abc", prepared.FactorySessionID)
	}
	if !prepared.RecordedAt.Equal(start) {
		t.Fatalf("RecordedAt = %s, want %s", prepared.RecordedAt, start)
	}
	if prepared.Sequence != 0 || prepared.EventID != "" {
		t.Fatalf("prepared identity = (%d, %q), want zero values before assignment", prepared.Sequence, prepared.EventID)
	}
	if store.LatestSequence() != 0 || len(store.Events()) != 0 {
		t.Fatalf("PrepareInput must not mutate store state: latest=%d events=%d", store.LatestSequence(), len(store.Events()))
	}
}

func TestSessionResponseEventStore_PrepareInputPreservesExplicitRecordedAtAsUTC(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	loc := time.FixedZone("UTC-5", -5*60*60)
	explicit := time.Date(2026, 1, 2, 3, 4, 5, 0, loc)

	input := samplePublishInput()
	input.RecordedAt = explicit

	prepared := store.PrepareInput(input)
	if !prepared.RecordedAt.Equal(explicit) || prepared.RecordedAt.Location() != time.UTC {
		t.Fatalf("RecordedAt = %s (%s), want %s normalized to UTC", prepared.RecordedAt, prepared.RecordedAt.Location(), explicit)
	}
}

func TestSessionResponseEventStore_PrepareInputOnNilStoreReturnsInputUnchanged(t *testing.T) {
	t.Parallel()

	var store *responseeventstore.SessionResponseEventStore
	input := samplePublishInput()

	if got := store.PrepareInput(input); got.RunID != input.RunID || got.Payload == nil {
		t.Fatalf("PrepareInput on nil store = %#v, want input echoed back unchanged", got)
	}
}

func TestSessionResponseEventStore_NextSequenceHintTracksLatestSequence(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")

	if got := store.NextSequenceHint(); got != 1 {
		t.Fatalf("NextSequenceHint() on empty store = %d, want 1", got)
	}

	published, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := store.NextSequenceHint(); got != published.Sequence+1 {
		t.Fatalf("NextSequenceHint() after one publish = %d, want %d", got, published.Sequence+1)
	}
}

func TestSessionResponseEventStore_PublishWithIdentityUsesSuppliedIdentity(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	prepared := store.PrepareInput(samplePublishInput())
	sequence := store.NextSequenceHint()

	published, err := store.PublishWithIdentity(prepared, sequence, "external-event-id")
	if err != nil {
		t.Fatalf("PublishWithIdentity: %v", err)
	}
	if published.Sequence != sequence || published.EventID != "external-event-id" {
		t.Fatalf("published identity = (%d, %q), want (%d, %q)", published.Sequence, published.EventID, sequence, "external-event-id")
	}
	if store.LatestSequence() != sequence {
		t.Fatalf("LatestSequence() = %d, want %d", store.LatestSequence(), sequence)
	}
	stored, ok := store.EventAtSequence(sequence)
	if !ok || stored.EventID != "external-event-id" {
		t.Fatalf("EventAtSequence(%d) = %#v, %v, want the externally-identified record", sequence, stored, ok)
	}
}

func TestSessionResponseEventStore_PublishWithIdentityRejectsSequenceMismatchWithoutMutatingState(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	prepared := store.PrepareInput(samplePublishInput())

	if _, err := store.PublishWithIdentity(prepared, 5, "external-event-id"); !errors.Is(err, responseeventstore.ErrSequenceMismatch) {
		t.Fatalf("PublishWithIdentity with wrong sequence error = %v, want ErrSequenceMismatch", err)
	}
	if store.LatestSequence() != 0 || len(store.Events()) != 0 {
		t.Fatalf("rejected PublishWithIdentity mutated state: latest=%d events=%d", store.LatestSequence(), len(store.Events()))
	}
}

func TestSessionResponseEventStore_PublishWithIdentityRejectsEmptyEventID(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	prepared := store.PrepareInput(samplePublishInput())
	sequence := store.NextSequenceHint()

	if _, err := store.PublishWithIdentity(prepared, sequence, "   "); err == nil {
		t.Fatal("PublishWithIdentity with blank event ID = nil error")
	}
	if store.LatestSequence() != 0 {
		t.Fatalf("LatestSequence() = %d, want 0 after rejected identity", store.LatestSequence())
	}
}

func TestSessionResponseEventStore_PublishWithIdentityRejectsForeignFactorySessionID(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	prepared := store.PrepareInput(samplePublishInput())
	prepared.FactorySessionID = "session-other"
	sequence := store.NextSequenceHint()

	if _, err := store.PublishWithIdentity(prepared, sequence, "external-event-id"); !errors.Is(err, responseeventstore.ErrFactorySessionMismatch) {
		t.Fatalf("PublishWithIdentity foreign session error = %v, want ErrFactorySessionMismatch", err)
	}
}

func TestSessionResponseEventStore_PublishWithIdentityRejectsInvalidEvent(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	prepared := store.PrepareInput(samplePublishInput())
	prepared.Phase = responseevents.PhaseUpdated
	sequence := store.NextSequenceHint()

	if _, err := store.PublishWithIdentity(prepared, sequence, "external-event-id"); err == nil {
		t.Fatal("PublishWithIdentity with invalid kind/phase pair = nil error")
	}
	if store.LatestSequence() != 0 {
		t.Fatalf("LatestSequence() = %d, want 0 after rejected identity", store.LatestSequence())
	}
}

func TestSessionResponseEventStore_PublishWithIdentityRejectsAfterClose(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	prepared := store.PrepareInput(samplePublishInput())
	sequence := store.NextSequenceHint()
	store.Close()

	if _, err := store.PublishWithIdentity(prepared, sequence, "external-event-id"); !errors.Is(err, responseeventstore.ErrStoreClosed) {
		t.Fatalf("PublishWithIdentity after close error = %v, want ErrStoreClosed", err)
	}
}

func TestSessionResponseEventStore_PublishWithIdentityRejectsAfterComplete(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	prepared := store.PrepareInput(samplePublishInput())
	sequence := store.NextSequenceHint()
	store.Complete()

	if _, err := store.PublishWithIdentity(prepared, sequence, "external-event-id"); !errors.Is(err, responseeventstore.ErrStoreCompleted) {
		t.Fatalf("PublishWithIdentity after complete error = %v, want ErrStoreCompleted", err)
	}
}

func TestSessionResponseEventStore_PublishWithIdentityOnNilStoreReturnsError(t *testing.T) {
	t.Parallel()

	var store *responseeventstore.SessionResponseEventStore
	if _, err := store.PublishWithIdentity(samplePublishInput(), 1, "external-event-id"); err == nil {
		t.Fatal("PublishWithIdentity on nil store = nil error")
	}
}

func TestSessionResponseEventStore_PublishWithIdentityStoredEventIsImmutableAfterCallerMutation(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	prepared := store.PrepareInput(samplePublishInput())
	sequence := store.NextSequenceHint()

	published, err := store.PublishWithIdentity(prepared, sequence, "external-event-id")
	if err != nil {
		t.Fatalf("PublishWithIdentity: %v", err)
	}

	prepared.EventID = "mutated-after-return"
	prepared.Sequence = 999

	stored, ok := store.EventAtSequence(published.Sequence)
	if !ok || stored.EventID != published.EventID || stored.Sequence != published.Sequence {
		t.Fatalf("identity changed after caller mutation of the prepared input: got (%q, %d), want (%q, %d)",
			stored.EventID, stored.Sequence, published.EventID, published.Sequence)
	}
}
