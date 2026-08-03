package responseeventstore_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

func acceptingCommit(eventID string) func(responseevents.FactoryResponseEvent, int64) (int64, string, error) {
	return func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		return sequenceHint, eventID, nil
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityUsesCommitAssignedIdentity(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")

	published, err := store.PublishThroughAuthority(samplePublishInput(), acceptingCommit("external-event-id"))
	if err != nil {
		t.Fatalf("PublishThroughAuthority: %v", err)
	}
	if published.Sequence != 1 || published.EventID != "external-event-id" {
		t.Fatalf("published identity = (%d, %q), want (1, %q)", published.Sequence, published.EventID, "external-event-id")
	}
	if store.LatestSequence() != 1 {
		t.Fatalf("LatestSequence() = %d, want 1", store.LatestSequence())
	}
	stored, ok := store.EventAtSequence(1)
	if !ok || stored.EventID != "external-event-id" {
		t.Fatalf("EventAtSequence(1) = %#v, %v, want the commit-identified record", stored, ok)
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityPassesNormalizedEventAndSequenceHintToCommit(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	var gotSessionID string
	var gotSequenceHint int64
	if _, err := store.PublishThroughAuthority(samplePublishInput(), func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		gotSessionID = prepared.FactorySessionID
		gotSequenceHint = sequenceHint
		return sequenceHint, "event-1", nil
	}); err != nil {
		t.Fatalf("PublishThroughAuthority: %v", err)
	}
	if gotSessionID != "session-abc" {
		t.Fatalf("commit saw FactorySessionID = %q, want session-abc", gotSessionID)
	}
	if gotSequenceHint != 1 {
		t.Fatalf("commit saw sequenceHint = %d, want 1", gotSequenceHint)
	}

	if _, err := store.PublishThroughAuthority(samplePublishInput(), func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		gotSequenceHint = sequenceHint
		return sequenceHint, "event-2", nil
	}); err != nil {
		t.Fatalf("PublishThroughAuthority: %v", err)
	}
	if gotSequenceHint != 2 {
		t.Fatalf("second commit saw sequenceHint = %d, want 2", gotSequenceHint)
	}
}

// TestSessionResponseEventStore_PublishThroughAuthorityNeverRetainsWhenCommitFails proves
// the store cannot end up with a record an external authority (the injected
// Events root) rejected: any commit error leaves the store's retained state
// and next-sequence counter completely untouched.
func TestSessionResponseEventStore_PublishThroughAuthorityNeverRetainsWhenCommitFails(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	errCommitRejected := errors.New("authority rejected the append")

	if _, err := store.PublishThroughAuthority(samplePublishInput(), func(responseevents.FactoryResponseEvent, int64) (int64, string, error) {
		return 0, "", errCommitRejected
	}); !errors.Is(err, errCommitRejected) {
		t.Fatalf("PublishThroughAuthority error = %v, want wrapped errCommitRejected", err)
	}
	if store.LatestSequence() != 0 || len(store.Events()) != 0 {
		t.Fatalf("rejected commit mutated state: latest=%d events=%d", store.LatestSequence(), len(store.Events()))
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityRejectsSequenceMismatchWithoutMutatingState(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")

	_, err := store.PublishThroughAuthority(samplePublishInput(), func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		return sequenceHint + 1, "external-event-id", nil
	})
	if !errors.Is(err, responseeventstore.ErrSequenceMismatch) {
		t.Fatalf("PublishThroughAuthority with wrong sequence error = %v, want ErrSequenceMismatch", err)
	}
	if store.LatestSequence() != 0 || len(store.Events()) != 0 {
		t.Fatalf("rejected PublishThroughAuthority mutated state: latest=%d events=%d", store.LatestSequence(), len(store.Events()))
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityRejectsEmptyEventIDWithoutMutatingState(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")

	if _, err := store.PublishThroughAuthority(samplePublishInput(), acceptingCommit("   ")); err == nil {
		t.Fatal("PublishThroughAuthority with blank event ID = nil error")
	}
	if store.LatestSequence() != 0 || len(store.Events()) != 0 {
		t.Fatalf("rejected PublishThroughAuthority mutated state: latest=%d events=%d", store.LatestSequence(), len(store.Events()))
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityRejectsForeignFactorySessionIDBeforeCommit(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	input := samplePublishInput()
	input.FactorySessionID = "session-other"

	commitCalls := 0
	_, err := store.PublishThroughAuthority(input, func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		commitCalls++
		return sequenceHint, "external-event-id", nil
	})
	if !errors.Is(err, responseeventstore.ErrFactorySessionMismatch) {
		t.Fatalf("PublishThroughAuthority foreign session error = %v, want ErrFactorySessionMismatch", err)
	}
	if commitCalls != 0 {
		t.Fatalf("commit was called %d times for a rejected foreign session, want 0 (the external authority must never be asked to accept a record this store will not retain)", commitCalls)
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityRejectsInvalidEventBeforeCommit(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	input := samplePublishInput()
	input.Phase = responseevents.PhaseUpdated

	commitCalls := 0
	_, err := store.PublishThroughAuthority(input, func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		commitCalls++
		return sequenceHint, "external-event-id", nil
	})
	if err == nil {
		t.Fatal("PublishThroughAuthority with invalid kind/phase pair = nil error")
	}
	if commitCalls != 0 {
		t.Fatalf("commit was called %d times for a locally-invalid event, want 0", commitCalls)
	}
}

// TestSessionResponseEventStore_PublishThroughAuthorityRejectsAfterCloseBeforeCommit proves a
// closed store never invokes the external authority for a new publish: Close
// is checked before commit is called, so a closed store cannot cause a
// record to exist in the authority (Events) that this store will never
// retain.
func TestSessionResponseEventStore_PublishThroughAuthorityRejectsAfterCloseBeforeCommit(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	store.Close()

	commitCalls := 0
	_, err := store.PublishThroughAuthority(samplePublishInput(), func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		commitCalls++
		return sequenceHint, "external-event-id", nil
	})
	if !errors.Is(err, responseeventstore.ErrStoreClosed) {
		t.Fatalf("PublishThroughAuthority after close error = %v, want ErrStoreClosed", err)
	}
	if commitCalls != 0 {
		t.Fatalf("commit was called %d times after Close, want 0", commitCalls)
	}
}

// TestSessionResponseEventStore_PublishThroughAuthorityRejectsAfterCompleteBeforeCommit mirrors
// the Close case for Complete.
func TestSessionResponseEventStore_PublishThroughAuthorityRejectsAfterCompleteBeforeCommit(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	store.Complete()

	commitCalls := 0
	_, err := store.PublishThroughAuthority(samplePublishInput(), func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		commitCalls++
		return sequenceHint, "external-event-id", nil
	})
	if !errors.Is(err, responseeventstore.ErrStoreCompleted) {
		t.Fatalf("PublishThroughAuthority after complete error = %v, want ErrStoreCompleted", err)
	}
	if commitCalls != 0 {
		t.Fatalf("commit was called %d times after Complete, want 0", commitCalls)
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityOnNilStoreReturnsError(t *testing.T) {
	t.Parallel()

	var store *responseeventstore.SessionResponseEventStore
	if _, err := store.PublishThroughAuthority(samplePublishInput(), acceptingCommit("external-event-id")); err == nil {
		t.Fatal("PublishThroughAuthority on nil store = nil error")
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityRejectsNilCommit(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	if _, err := store.PublishThroughAuthority(samplePublishInput(), nil); err == nil {
		t.Fatal("PublishThroughAuthority with nil commit = nil error")
	}
	if store.LatestSequence() != 0 {
		t.Fatalf("LatestSequence() = %d, want 0", store.LatestSequence())
	}
}

func TestSessionResponseEventStore_PublishThroughAuthorityStoredEventIsImmutableAfterCallerMutation(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")

	var capturedPrepared responseevents.FactoryResponseEvent
	published, err := store.PublishThroughAuthority(samplePublishInput(), func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		capturedPrepared = prepared
		return sequenceHint, "external-event-id", nil
	})
	if err != nil {
		t.Fatalf("PublishThroughAuthority: %v", err)
	}

	capturedPrepared.EventID = "mutated-after-return"
	capturedPrepared.Sequence = 999

	stored, ok := store.EventAtSequence(published.Sequence)
	if !ok || stored.EventID != published.EventID || stored.Sequence != published.Sequence {
		t.Fatalf("identity changed after caller mutation of the prepared input: got (%q, %d), want (%q, %d)",
			stored.EventID, stored.Sequence, published.EventID, published.Sequence)
	}
}
