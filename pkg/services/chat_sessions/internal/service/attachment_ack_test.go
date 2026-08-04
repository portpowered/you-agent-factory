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
	store := NewStore(sequentialIDs("id"), fixedClock(time.Now()), logger)

	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	appender := newFakeEventsAppender()
	store.WithEventsAppender(appender).WithEventsReader(appender)
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
