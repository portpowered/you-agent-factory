package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

// newAttachTestSession constructs a Store and one created Session ready for
// Attach/Detach calls.
func newAttachTestSession(t *testing.T) (*Store, chatsessions.Session) {
	t.Helper()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)
	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return store, created.Session
}

// TestStore_Attach_ReturnsDetachedValueWithUniqueID proves a successful
// Attach retains its own session ID, connection ID, zero initial
// AfterSequence, and interactive flag as a detached value.
func TestStore_Attach_ReturnsDetachedValueWithUniqueID(t *testing.T) {
	ctx := context.Background()
	store, session := newAttachTestSession(t)

	result, err := store.Attach(ctx, chatsessions.AttachRequest{
		SessionID:    session.ID,
		ConnectionID: "conn-a",
		Interactive:  true,
	})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	attachment := result.Attachment
	if attachment.ID == "" {
		t.Fatal("Attach: expected a non-blank attachment ID")
	}
	if attachment.SessionID != session.ID {
		t.Fatalf("SessionID = %q, want %q", attachment.SessionID, session.ID)
	}
	if attachment.ConnectionID != "conn-a" {
		t.Fatalf("ConnectionID = %q, want conn-a", attachment.ConnectionID)
	}
	if attachment.AfterSequence != 0 {
		t.Fatalf("AfterSequence = %d, want 0", attachment.AfterSequence)
	}
	if !attachment.Interactive {
		t.Fatal("Interactive = false, want true")
	}

	attachment.ConnectionID = "mutated"
	store.mu.RLock()
	stored := store.sessions[session.ID].attachments[result.Attachment.ID]
	store.mu.RUnlock()
	if stored.ConnectionID != "conn-a" {
		t.Fatalf("mutating returned value affected stored attachment: got %q", stored.ConnectionID)
	}
}

// TestStore_Attach_TwoAttachmentsRemainIndependent proves two attachments to
// the same session with matching other fields remain distinct, and attaching
// or detaching one does not change the other attachment or the session's
// stream head, episode history, turns, or version.
func TestStore_Attach_TwoAttachmentsRemainIndependent(t *testing.T) {
	ctx := context.Background()
	store, session := newAttachTestSession(t)

	first, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a", Interactive: true})
	if err != nil {
		t.Fatalf("Attach first: %v", err)
	}
	second, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a", Interactive: true})
	if err != nil {
		t.Fatalf("Attach second: %v", err)
	}
	if first.Attachment.ID == second.Attachment.ID {
		t.Fatalf("two attachments shared ID %q, want distinct", first.Attachment.ID)
	}

	if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: first.Attachment.ID}); err != nil {
		t.Fatalf("Detach first: %v", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if _, ok := record.attachments[first.Attachment.ID]; ok {
		t.Fatal("detached attachment still present")
	}
	remaining, ok := record.attachments[second.Attachment.ID]
	if !ok {
		t.Fatal("second attachment was removed by detaching the first")
	}
	if remaining != second.Attachment {
		t.Fatalf("second attachment changed: got %+v, want %+v", remaining, second.Attachment)
	}
	if record.session != session {
		t.Fatalf("attach/detach mutated Session: got %+v, want %+v", record.session, session)
	}
	if len(record.episodes) != 1 {
		t.Fatalf("attach/detach created %d episodes, want 1", len(record.episodes))
	}
	if len(record.turns) != 0 {
		t.Fatalf("attach/detach created %d turns, want 0", len(record.turns))
	}
}

// TestStore_Detach_UnknownOrRemovedIsTypedNotFound proves detaching an
// unknown or already-removed attachment reports *NotFoundError and performs
// no additional mutation.
func TestStore_Detach_UnknownOrRemovedIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store, session := newAttachTestSession(t)

	_, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: "does-not-exist"})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("Detach unknown attachment: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Attachment" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Attachment ID=does-not-exist", notFound)
	}

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: attached.Attachment.ID}); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	_, err = store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: attached.Attachment.ID})
	if !errors.As(err, &notFound) {
		t.Fatalf("Detach already-removed attachment: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Attachment" || notFound.ID != attached.Attachment.ID {
		t.Fatalf("NotFoundError = %+v, want Value=Attachment ID=%s", notFound, attached.Attachment.ID)
	}
}

// TestStore_Attach_UnknownSessionOrInvalidInputCreatesNoAttachment proves
// attaching to an unknown session or with a blank ConnectionID reports the
// applicable typed error and creates no attachment.
func TestStore_Attach_UnknownSessionOrInvalidInputCreatesNoAttachment(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown session", func(t *testing.T) {
		store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)
		_, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: "does-not-exist", ConnectionID: "conn-a"})
		var notFound *chatsessions.NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("Attach unknown session: got %v, want *NotFoundError", err)
		}
		if notFound.Value != "Session" || notFound.ID != "does-not-exist" {
			t.Fatalf("NotFoundError = %+v, want Value=Session ID=does-not-exist", notFound)
		}
	})

	t.Run("blank connection id", func(t *testing.T) {
		store, session := newAttachTestSession(t)
		_, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: ""})
		if !errors.Is(err, chatsessions.ErrRequiredValue) {
			t.Fatalf("Attach blank ConnectionID: got %v, want ErrRequiredValue", err)
		}
		store.mu.RLock()
		count := len(store.sessions[session.ID].attachments)
		store.mu.RUnlock()
		if count != 0 {
			t.Fatalf("Attach with invalid input created %d attachments, want 0", count)
		}
	})
}

// TestStore_Detach_UnknownSessionIsTypedNotFound proves Detach against an
// unknown SessionID reports *NotFoundError naming Session.
func TestStore_Detach_UnknownSessionIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

	_, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: "does-not-exist", AttachmentID: "attachment-1"})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("Detach unknown session: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Session" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Session ID=does-not-exist", notFound)
	}
}

// TestStore_Attach_ConcurrentAttachAndDetachRaceFree proves concurrent
// attach/detach operations against one session are race-free, produce
// unique accepted identities, and settle on exactly the attachments that
// were never detached.
func TestStore_Attach_ConcurrentAttachAndDetachRaceFree(t *testing.T) {
	ctx := context.Background()
	store, session := newAttachTestSession(t)

	const n = 25
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a"})
			if err != nil {
				t.Errorf("Attach[%d]: %v", i, err)
				return
			}
			ids[i] = result.Attachment.ID
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for _, id := range ids {
		if id == "" {
			t.Fatal("Attach produced a blank attachment ID")
		}
		if seen[id] {
			t.Fatalf("Attach produced duplicate ID %q", id)
		}
		seen[id] = true
	}

	toDetach := ids[:n/2]
	var detachWG sync.WaitGroup
	for _, id := range toDetach {
		detachWG.Add(1)
		go func(id string) {
			defer detachWG.Done()
			if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: id}); err != nil {
				t.Errorf("Detach(%s): %v", id, err)
			}
		}(id)
	}
	detachWG.Wait()

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.attachments) != n-len(toDetach) {
		t.Fatalf("remaining attachments = %d, want %d", len(record.attachments), n-len(toDetach))
	}
	for _, id := range toDetach {
		if _, ok := record.attachments[id]; ok {
			t.Fatalf("detached attachment %q still present", id)
		}
	}
	for _, id := range ids[n/2:] {
		if _, ok := record.attachments[id]; !ok {
			t.Fatalf("non-detached attachment %q missing", id)
		}
	}
}

// acknowledgeAttachmentRequest builds an AcknowledgeAttachmentRequest for the
// given attachment, expected session version, and requested position.
func acknowledgeAttachmentRequest(sessionID, attachmentID string, expectedVersion uint64, afterSequence events.AggregateSequence) chatsessions.AcknowledgeAttachmentRequest {
	return chatsessions.AcknowledgeAttachmentRequest{
		SessionID:       sessionID,
		AttachmentID:    attachmentID,
		ExpectedVersion: expectedVersion,
		AfterSequence:   afterSequence,
	}
}

// sequenceAndAdvance sequences n source records (source sequence numbers
// startSourceSeq..startSourceSeq+n-1, so a second call against the same
// session can pass a fresh starting point instead of colliding with an
// earlier call's already-committed source identity tuples) onto session and
// advances StreamHead to the last committed position, returning the final
// aggregate sequence and the session snapshot (with its now-current
// Version) after the advancement. It is test setup shared by every
// AcknowledgeAttachment test that needs a StreamHead ahead of position 0.
func sequenceAndAdvance(t *testing.T, store *Store, session chatsessions.Session, startSourceSeq, n int) (events.AggregateSequence, chatsessions.Session) {
	t.Helper()
	ctx := context.Background()
	var last events.AggregateSequence
	var lastSourceSeq events.SourceSequence
	for i := range n {
		sourceSeq := events.SourceSequence(startSourceSeq + i)
		result, err := store.Sequence(ctx, sequenceRequest(session.ID, sourceSeq, ""))
		if err != nil {
			t.Fatalf("Sequence(%d): %v", sourceSeq, err)
		}
		last = result.AggregateSequence
		lastSourceSeq = sourceSeq
	}
	advanced, err := store.AdvanceStreamHead(ctx, advanceStreamHeadRequest(session.ID, lastSourceSeq, session.Version, last))
	if err != nil {
		t.Fatalf("AdvanceStreamHead: %v", err)
	}
	return last, advanced.Session
}

// TestStore_AcknowledgeAttachment_AdvancesToRequestedPosition proves a valid
// acknowledgement against an already-committed position advances the named
// Attachment's AfterSequence to exactly that position.
func TestStore_AcknowledgeAttachment_AdvancesToRequestedPosition(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 3)

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	result, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, last))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment: %v", err)
	}
	if result.Outcome != chatsessions.AcknowledgeAttachmentOutcomeAdvanced {
		t.Fatalf("Outcome = %v, want AcknowledgeAttachmentOutcomeAdvanced", result.Outcome)
	}
	if result.Attachment.AfterSequence != uint64(last) {
		t.Fatalf("AfterSequence = %d, want %d", result.Attachment.AfterSequence, last)
	}
	if result.Attachment.ID != attached.Attachment.ID {
		t.Fatalf("Attachment.ID = %q, want %q", result.Attachment.ID, attached.Attachment.ID)
	}
}

// TestStore_AcknowledgeAttachment_MultipleAttachmentsAdvanceIndependently
// proves two attachments consuming the same committed records at different
// rates never observe or move one another's cursor, and neither
// acknowledgement changes Session.StreamHead.
func TestStore_AcknowledgeAttachment_MultipleAttachmentsAdvanceIndependently(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 5)

	first, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach (first): %v", err)
	}
	second, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-2"})
	if err != nil {
		t.Fatalf("Attach (second): %v", err)
	}

	if _, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, first.Attachment.ID, session.Version, 2)); err != nil {
		t.Fatalf("AcknowledgeAttachment (first, to 2): %v", err)
	}
	if _, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, second.Attachment.ID, session.Version, last)); err != nil {
		t.Fatalf("AcknowledgeAttachment (second, to %d): %v", last, err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()

	if got := record.attachments[first.Attachment.ID].AfterSequence; got != 2 {
		t.Fatalf("first attachment AfterSequence = %d, want 2 (unaffected by second's advance)", got)
	}
	if got := record.attachments[second.Attachment.ID].AfterSequence; got != uint64(last) {
		t.Fatalf("second attachment AfterSequence = %d, want %d", got, last)
	}
	if record.session.StreamHead != uint64(last) {
		t.Fatalf("StreamHead = %d, want unchanged %d", record.session.StreamHead, last)
	}
}

// TestStore_AcknowledgeAttachment_NeverMovesBackward proves a request naming
// a position at or before the attachment's current AfterSequence reconciles
// idempotently instead of regressing the cursor.
func TestStore_AcknowledgeAttachment_NeverMovesBackward(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 3)

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	advanced, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, last))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment (advance): %v", err)
	}
	if advanced.Attachment.AfterSequence != uint64(last) {
		t.Fatalf("AfterSequence = %d, want %d", advanced.Attachment.AfterSequence, last)
	}

	// A stale request for an earlier position, carrying the version observed
	// before the first acknowledgement -- must not regress AfterSequence.
	stale, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, 1))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment (stale earlier position): %v", err)
	}
	if stale.Outcome != chatsessions.AcknowledgeAttachmentOutcomeAlreadyCurrent {
		t.Fatalf("stale earlier position Outcome = %v, want AcknowledgeAttachmentOutcomeAlreadyCurrent", stale.Outcome)
	}
	if stale.Attachment.AfterSequence != uint64(last) {
		t.Fatalf("AfterSequence regressed: got %d, want unchanged %d", stale.Attachment.AfterSequence, last)
	}

	// An exact repeat of the same position is likewise idempotent.
	repeat, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, last))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment (repeat): %v", err)
	}
	if repeat.Outcome != chatsessions.AcknowledgeAttachmentOutcomeAlreadyCurrent {
		t.Fatalf("repeat Outcome = %v, want AcknowledgeAttachmentOutcomeAlreadyCurrent", repeat.Outcome)
	}
}

// TestStore_AcknowledgeAttachment_RejectsPositionBeyondStreamHead proves a
// requested position past the session's current StreamHead is rejected with
// *AttachmentPositionError and leaves the attachment unchanged.
func TestStore_AcknowledgeAttachment_RejectsPositionBeyondStreamHead(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 1)

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	_, err = store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, last+10))
	var positionErr *chatsessions.AttachmentPositionError
	if !errors.As(err, &positionErr) {
		t.Fatalf("AcknowledgeAttachment beyond stream head: got %v, want *AttachmentPositionError", err)
	}
	if !errors.Is(err, chatsessions.ErrAttachmentBeyondStreamHead) {
		t.Fatalf("AcknowledgeAttachment beyond stream head: got %v, want ErrAttachmentBeyondStreamHead", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if got := record.attachments[attached.Attachment.ID].AfterSequence; got != 0 {
		t.Fatalf("AfterSequence mutated by rejected request: got %d, want 0", got)
	}
}

// TestStore_AcknowledgeAttachment_StaleVersionIsTypedConflictWithNoPartialState
// proves a genuinely new, in-range position with a stale ExpectedVersion
// reports *ConflictError and leaves the attachment completely unchanged.
func TestStore_AcknowledgeAttachment_StaleVersionIsTypedConflictWithNoPartialState(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 2)

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	_, err = store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version+1, last))
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("AcknowledgeAttachment stale version: got %v, want *ConflictError", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if got := record.attachments[attached.Attachment.ID].AfterSequence; got != 0 {
		t.Fatalf("AfterSequence mutated by rejected request: got %d, want 0", got)
	}
	if record.session.Version != session.Version {
		t.Fatalf("Version mutated by rejected acknowledgement: got %d, want %d", record.session.Version, session.Version)
	}
}

// TestStore_AcknowledgeAttachment_RetentionGapIsRejected proves a requested
// position spanning a range Events retention has since evicted is rejected
// with *AttachmentRetentionGapError, using a fakeEventsAppender retention cap
// to force a deterministic eviction instead of committing thousands of
// records to reach a real retention limit. It also proves the companion
// positive case: acknowledging only up to a position the attachment can
// reach without spanning the evicted prefix still succeeds.
func TestStore_AcknowledgeAttachment_RetentionGapIsRejected(t *testing.T) {
	ctx := context.Background()
	store, session, appender := newSequencingTestSession(t)
	topic := chatsessions.EventsTopic(session.ID)

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Commit and acknowledge through position 3 while retention is still
	// unbounded, so the attachment legitimately reaches AfterSequence 3
	// before any eviction occurs.
	_, session = sequenceAndAdvance(t, store, session, 1, 3)
	acked, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, 3))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment (to 3, before eviction): %v", err)
	}
	if acked.Attachment.AfterSequence != 3 {
		t.Fatalf("AfterSequence = %d, want 3", acked.Attachment.AfterSequence)
	}

	// Now cap retention to 2 and commit 2 more records: positions 1-3 are
	// evicted, leaving only 4 and 5 retained.
	appender.setRetentionLimit(topic, 2)
	last, session := sequenceAndAdvance(t, store, session, 4, 2)

	// A stale, far-behind attachment that never advanced past 0 cannot
	// legitimately claim to have observed positions 1-3, which no longer
	// exist to prove delivery of: acknowledging straight to the head from
	// that starting position spans an unobserved retention gap.
	behindAttached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-2"})
	if err != nil {
		t.Fatalf("Attach (behind): %v", err)
	}
	_, err = store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, behindAttached.Attachment.ID, session.Version, last))
	var gapErr *chatsessions.AttachmentRetentionGapError
	if !errors.As(err, &gapErr) {
		t.Fatalf("AcknowledgeAttachment across a retention gap: got %v, want *AttachmentRetentionGapError", err)
	}
	if !errors.Is(err, chatsessions.ErrAttachmentRetentionGap) {
		t.Fatalf("AcknowledgeAttachment across a retention gap: got %v, want ErrAttachmentRetentionGap", err)
	}
	if gapErr.EarliestRetained != 4 {
		t.Fatalf("gapErr.EarliestRetained = %d, want 4", gapErr.EarliestRetained)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if got := record.attachments[behindAttached.Attachment.ID].AfterSequence; got != 0 {
		t.Fatalf("AfterSequence mutated by rejected request: got %d, want 0", got)
	}

	// The attachment that already reached position 3 (before eviction) can
	// still acknowledge forward through the retained tail without spanning
	// any evicted content.
	ok, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, last))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment within retained range: %v", err)
	}
	if ok.Attachment.AfterSequence != uint64(last) {
		t.Fatalf("AfterSequence = %d, want %d", ok.Attachment.AfterSequence, last)
	}
}

// TestStore_AcknowledgeAttachment_RejectsWhenReadStopsShortOfRequestedPosition
// proves AcknowledgeAttachment does not treat a bare ReadOutcomeProgress as
// sufficient proof that the full requested range was retained: it also
// requires the read's own Next cursor to land exactly on the requested
// position. This directly exercises the scenario a fabricated or
// unvalidated StreamHead would otherwise enable -- Events reports Progress,
// not Gap, when a caller asks for more records than currently exist, so a
// naive "Progress means safe" check would let AcknowledgeAttachment advance
// an attachment past positions Events never actually committed. The
// fabrication is injected directly on the stored session (bypassing
// AdvanceStreamHead, which independently rejects any uncommitted position)
// to isolate AcknowledgeAttachment's own defense.
func TestStore_AcknowledgeAttachment_RejectsWhenReadStopsShortOfRequestedPosition(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	_, session = sequenceAndAdvance(t, store, session, 1, 1)

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	store.mu.Lock()
	record := store.sessions[session.ID]
	record.session.StreamHead = 5
	store.sessions[session.ID] = record
	store.mu.Unlock()

	_, err = store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, 5))
	if err == nil {
		t.Fatal("AcknowledgeAttachment: got nil error, want rejection for a position Events never actually committed")
	}

	store.mu.RLock()
	record = store.sessions[session.ID]
	store.mu.RUnlock()
	if got := record.attachments[attached.Attachment.ID].AfterSequence; got != 0 {
		t.Fatalf("AfterSequence mutated by rejected request: got %d, want 0", got)
	}
}

// TestStore_AcknowledgeAttachment_CrossSessionAttachmentReportsNotFound
// proves an AttachmentID that identifies a real attachment on a different
// session is not resolvable against this session -- an acknowledgement can
// never move a different session's attachment.
func TestStore_AcknowledgeAttachment_CrossSessionAttachmentReportsNotFound(t *testing.T) {
	ctx := context.Background()
	store, sessionA, _ := newSequencingTestSession(t)
	createdB, err := store.CreateSession(ctx, chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-2", JSONRPCStringID: "req-2"},
		WorkingRoot:   "/workspace/project-b",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession (B): %v", err)
	}

	attachedA, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: sessionA.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach (A): %v", err)
	}

	_, err = store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(createdB.Session.ID, attachedA.Attachment.ID, 0, 1))
	if !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("AcknowledgeAttachment(session B, session A's attachment): got %v, want ErrNotFound", err)
	}
}

// TestStore_AcknowledgeAttachment_UnknownSessionReportsNotFound proves
// AcknowledgeAttachment against a SessionID that does not identify an
// existing session reports *NotFoundError.
func TestStore_AcknowledgeAttachment_UnknownSessionReportsNotFound(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newSequencingTestSession(t)

	_, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest("does-not-exist", "attachment-1", 0, 1))
	if !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("AcknowledgeAttachment(unknown session): got %v, want ErrNotFound", err)
	}
}

// TestStore_AcknowledgeAttachment_UnknownAttachmentReportsNotFound proves
// AcknowledgeAttachment against an AttachmentID that does not identify an
// existing attachment on a known session reports *NotFoundError.
func TestStore_AcknowledgeAttachment_UnknownAttachmentReportsNotFound(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	_, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, "does-not-exist", session.Version, 1))
	if !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("AcknowledgeAttachment(unknown attachment): got %v, want ErrNotFound", err)
	}
}

// TestStore_AcknowledgeAttachment_InvalidRequestIsRejected table-drives
// every AcknowledgeAttachmentRequest validation failure.
func TestStore_AcknowledgeAttachment_InvalidRequestIsRejected(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		mutate  func(chatsessions.AcknowledgeAttachmentRequest) chatsessions.AcknowledgeAttachmentRequest
		wantErr error
	}{
		{
			name: "blank session id",
			mutate: func(r chatsessions.AcknowledgeAttachmentRequest) chatsessions.AcknowledgeAttachmentRequest {
				r.SessionID = ""
				return r
			},
			wantErr: chatsessions.ErrRequiredValue,
		},
		{
			name: "blank attachment id",
			mutate: func(r chatsessions.AcknowledgeAttachmentRequest) chatsessions.AcknowledgeAttachmentRequest {
				r.AttachmentID = ""
				return r
			},
			wantErr: chatsessions.ErrRequiredValue,
		},
		{
			name: "zero after sequence",
			mutate: func(r chatsessions.AcknowledgeAttachmentRequest) chatsessions.AcknowledgeAttachmentRequest {
				r.AfterSequence = 0
				return r
			},
			wantErr: events.ErrInvalidAggregateSequence,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, session, _ := newSequencingTestSession(t)
			attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
			if err != nil {
				t.Fatalf("Attach: %v", err)
			}
			req := tt.mutate(acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, 1))
			if _, err := store.AcknowledgeAttachment(ctx, req); !errors.Is(err, tt.wantErr) {
				t.Fatalf("AcknowledgeAttachment(%s): got %v, want %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

// TestStore_AcknowledgeAttachment_IndependentOfControlState proves
// acknowledging an attachment never reads or writes any ControlIntent, and a
// control request/advancement never moves an attachment cursor -- the two
// positions this story keeps distinct.
func TestStore_AcknowledgeAttachment_IndependentOfControlState(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 1)

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID: startTurnRequestID("req-turn-1"), SessionID: session.ID, ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	intent, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: controlRequestID("conn-2", "req-1"), SessionID: session.ID,
		ExpectedVersion: started.Session.Version, Action: chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl: %v", err)
	}

	if _, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, started.Session.Version, last)); err != nil {
		t.Fatalf("AcknowledgeAttachment: %v", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.controls[intent.Intent.RequestID].State != chatsessions.ControlIntentStateRequested {
		t.Fatalf("control intent state changed by AcknowledgeAttachment: got %v, want unchanged REQUESTED", record.controls[intent.Intent.RequestID].State)
	}

	if _, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: intent.Intent.RequestID, Next: chatsessions.ControlIntentStateCommitted,
	}); err != nil {
		t.Fatalf("AdvanceControl: %v", err)
	}

	store.mu.RLock()
	record = store.sessions[session.ID]
	store.mu.RUnlock()
	if got := record.attachments[attached.Attachment.ID].AfterSequence; got != uint64(last) {
		t.Fatalf("attachment AfterSequence moved by AdvanceControl: got %d, want unchanged %d", got, last)
	}
}

// TestStore_AcknowledgeAttachment_ConcurrentSamePositionConverges races many
// goroutines acknowledging the same attachment to the exact same position,
// released through a shared start barrier (never a sleep). Exactly one call
// observes AcknowledgeAttachmentOutcomeAdvanced; every other call reconciles
// as AlreadyCurrent, and the attachment lands on exactly that position with
// no torn or partial state.
func TestStore_AcknowledgeAttachment_ConcurrentSamePositionConverges(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 1)

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	req := acknowledgeAttachmentRequest(session.ID, attached.Attachment.ID, session.Version, last)

	const n = 25
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]chatsessions.AcknowledgeAttachmentResult, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = store.AcknowledgeAttachment(ctx, req)
		}(i)
	}
	close(start)
	wg.Wait()

	advancedCount := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("AcknowledgeAttachment[%d]: got %v, want success (idempotent convergence)", i, err)
		}
		if results[i].Attachment.AfterSequence != uint64(last) {
			t.Fatalf("AcknowledgeAttachment[%d]: AfterSequence = %d, want %d", i, results[i].Attachment.AfterSequence, last)
		}
		if results[i].Outcome == chatsessions.AcknowledgeAttachmentOutcomeAdvanced {
			advancedCount++
		}
	}
	if advancedCount != 1 {
		t.Fatalf("advanced outcome count = %d, want exactly 1", advancedCount)
	}
}

// TestStore_AcknowledgeAttachment_ConcurrentDistinctAttachmentsDoNotInterfere
// races two independent attachments acknowledging concurrently to different
// positions, released through a shared start barrier, proving neither
// observes or corrupts the other's cursor under concurrent scheduling.
func TestStore_AcknowledgeAttachment_ConcurrentDistinctAttachmentsDoNotInterfere(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 4)

	first, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach (first): %v", err)
	}
	second, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-2"})
	if err != nil {
		t.Fatalf("Attach (second): %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var firstErr, secondErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, firstErr = store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, first.Attachment.ID, session.Version, 2))
	}()
	go func() {
		defer wg.Done()
		<-start
		_, secondErr = store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, second.Attachment.ID, session.Version, last))
	}()
	close(start)
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("AcknowledgeAttachment (first): %v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("AcknowledgeAttachment (second): %v", secondErr)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if got := record.attachments[first.Attachment.ID].AfterSequence; got != 2 {
		t.Fatalf("first attachment AfterSequence = %d, want 2", got)
	}
	if got := record.attachments[second.Attachment.ID].AfterSequence; got != uint64(last) {
		t.Fatalf("second attachment AfterSequence = %d, want %d", got, last)
	}
}

// TestStore_AcknowledgeAttachment_SafeLogFields proves AcknowledgeAttachment
// logs the session, attachment, requested position, version, and outcome --
// and never the session's WorkingRoot or any other unsafe field.
func TestStore_AcknowledgeAttachment_SafeLogFields(t *testing.T) {
	logger, calls := newCaptureLogger()
	appender := newFakeEventsAppender()
	store := NewStore(sequentialIDs("id"), fixedClock(time.Now()), appender, appender, logger)

	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	seqResult, err := store.Sequence(context.Background(), sequenceRequest(created.Session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}
	advanced, err := store.AdvanceStreamHead(context.Background(), advanceStreamHeadRequest(created.Session.ID, 1, created.Session.Version, seqResult.AggregateSequence))
	if err != nil {
		t.Fatalf("AdvanceStreamHead: %v", err)
	}
	attached, err := store.Attach(context.Background(), chatsessions.AttachRequest{SessionID: created.Session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	*calls = nil // discard CreateSession/Sequence/AdvanceStreamHead/Attach's own log calls

	req := acknowledgeAttachmentRequest(created.Session.ID, attached.Attachment.ID, advanced.Session.Version, seqResult.AggregateSequence)
	result, err := store.AcknowledgeAttachment(context.Background(), req)
	if err != nil {
		t.Fatalf("AcknowledgeAttachment: %v", err)
	}

	if len(*calls) != 2 {
		t.Fatalf("AcknowledgeAttachment logged %d calls, want 2 (start, outcome): %+v", len(*calls), *calls)
	}
	outcome := (*calls)[1]
	if !hasKV(outcome.kv, "session_id", created.Session.ID) {
		t.Fatalf("outcome log missing session_id=%q: %+v", created.Session.ID, outcome.kv)
	}
	if !hasKV(outcome.kv, "attachment_id", attached.Attachment.ID) {
		t.Fatalf("outcome log missing attachment_id=%q: %+v", attached.Attachment.ID, outcome.kv)
	}
	if !hasKV(outcome.kv, "after_sequence", uint64(seqResult.AggregateSequence)) {
		t.Fatalf("outcome log missing after_sequence=%d: %+v", seqResult.AggregateSequence, outcome.kv)
	}
	if !hasKV(outcome.kv, "outcome", "advanced") {
		t.Fatalf("outcome log missing outcome=advanced: %+v", outcome.kv)
	}
	if !hasKV(outcome.kv, "error_class", "") {
		t.Fatalf("successful outcome log must carry an empty error_class: %+v", outcome.kv)
	}
	if result.Attachment.AfterSequence != uint64(seqResult.AggregateSequence) {
		t.Fatalf("AfterSequence = %d, want %d", result.Attachment.AfterSequence, seqResult.AggregateSequence)
	}

	assertNoUnsafeFields(t, *calls, created.Session.WorkingRoot)
}
