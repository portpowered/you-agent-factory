package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

// sequentialIDs returns a deterministic IDGenerator that yields prefix-1,
// prefix-2, ... so tests can assert on exact generated identities.
func sequentialIDs(prefix string) IDGenerator {
	n := 0
	return func() string {
		n++
		return prefix + "-" + strconv.Itoa(n)
	}
}

func fixedClock(at time.Time) Clock {
	return func() time.Time { return at }
}

func validCreateRequest() chatsessions.CreateSessionRequest {
	return chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-1"},
		WorkingRoot:   "/workspace/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
	}
}

func TestStore_CreateSession_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store := NewStore(sequentialIDs("session"), fixedClock(now), nil, nil)

	result, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	session := result.Session
	if session.ID == "" {
		t.Fatal("CreateSession: expected a non-blank session ID")
	}
	if session.State != chatsessions.SessionStateCreated {
		t.Fatalf("State = %v, want CREATED", session.State)
	}
	if session.WorkingRoot != "/workspace/project" {
		t.Fatalf("WorkingRoot = %q, want /workspace/project", session.WorkingRoot)
	}
	if session.SelectedTarget.Ref != "factory:@you/review" {
		t.Fatalf("SelectedTarget = %+v, want the requested initial target", session.SelectedTarget)
	}
	if session.TargetEpisode != 1 {
		t.Fatalf("TargetEpisode = %d, want 1", session.TargetEpisode)
	}
	if session.ActiveTurnID != "" {
		t.Fatalf("ActiveTurnID = %q, want blank", session.ActiveTurnID)
	}
	if session.Version == 0 {
		t.Fatal("Version = 0, want a non-zero initial version")
	}
	if session.CreatedAt.IsZero() || session.UpdatedAt.IsZero() {
		t.Fatalf("CreatedAt/UpdatedAt must be non-zero, got %+v", session)
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("created Session fails its own Validate: %v", err)
	}
}

// TestStore_CreateSession_InvalidInputCreatesNoSession proves a blank WorkingRoot,
// invalid RequestID, or invalid InitialTarget reports the existing typed
// validation classification and leaves the Store empty.
func TestStore_CreateSession_InvalidInputCreatesNoSession(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mutate  func(chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest
		wantErr error
	}{
		{"blank working root", func(r chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest {
			r.WorkingRoot = ""
			return r
		}, chatsessions.ErrRequiredValue},
		{"invalid request identity", func(r chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest {
			r.RequestID = chatsessions.RequestIdentity{}
			return r
		}, chatsessions.ErrUnknownEnumValue},
		{"invalid initial target", func(r chatsessions.CreateSessionRequest) chatsessions.CreateSessionRequest {
			r.InitialTarget = chatsessions.ChatTargetRef{}
			return r
		}, chatsessions.ErrUnknownEnumValue},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

			_, err := store.CreateSession(ctx, tt.mutate(validCreateRequest()))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateSession(%s): got %v, want %v", tt.name, err, tt.wantErr)
			}
			store.mu.RLock()
			count := len(store.sessions)
			store.mu.RUnlock()
			if count != 0 {
				t.Fatalf("CreateSession(%s): got %d observable sessions, want 0", tt.name, count)
			}
		})
	}
}

// TestStore_CreateSession_ReturnsDetachedValue proves mutating a returned
// Session cannot change what a later GetSession observes.
func TestStore_CreateSession_ReturnsDetachedValue(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	mutated := created.Session
	mutated.SelectedTarget.Ref = "factory:@you/mutated"

	reread, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if reread.Session.SelectedTarget.Ref != "factory:@you/review" {
		t.Fatalf("GetSession observed a mutation made to a previously returned value: %+v", reread.Session)
	}
}

// TestStore_CreateSession_UniqueIDs proves two successful creates never
// collide on generated session identity.
func TestStore_CreateSession_UniqueIDs(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

	first, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession first: %v", err)
	}
	second, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession second: %v", err)
	}
	if first.Session.ID == second.Session.ID {
		t.Fatalf("two CreateSession calls returned the same ID %q", first.Session.ID)
	}
}

func TestStore_GetSession_UnknownIDIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

	_, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: "does-not-exist"})
	if !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("GetSession unknown id: got %v, want ErrNotFound", err)
	}
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("GetSession unknown id: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Session" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Session ID=does-not-exist", notFound)
	}

	store.mu.RLock()
	count := len(store.sessions)
	store.mu.RUnlock()
	if count != 0 {
		t.Fatalf("GetSession on an unknown ID created %d placeholder sessions, want 0", count)
	}
}

// TestStore_InstancesShareNoState proves two independently constructed Store
// instances are fully isolated: a session created in one is invisible to the
// other.
func TestStore_InstancesShareNoState(t *testing.T) {
	ctx := context.Background()
	first := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)
	second := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

	created, err := first.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := second.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID}); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("second Store observed the first Store's session: got %v, want ErrNotFound", err)
	}
}

// TestStore_CreateSession_ConcurrentDifferentSessionsAreIndependent proves
// CreateSession is safe under concurrent calls that each create a distinct
// session, even though the injected IDGenerator here (sequentialIDs)
// mutates a plain closure variable with no synchronization of its own:
// Store must serialize its own calls to newID/now under s.mu rather than
// require every caller-supplied dependency to be concurrency-safe by
// contract. It also proves the resulting sessions are independent -- a
// concurrent SetTarget against each of the n distinct sessions afterward
// succeeds for every one with no cross-session interference.
func TestStore_CreateSession_ConcurrentDifferentSessionsAreIndependent(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

	const n = 25
	var wg sync.WaitGroup
	results := make([]chatsessions.CreateSessionResult, n)
	createErrs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], createErrs[i] = store.CreateSession(ctx, chatsessions.CreateSessionRequest{
				RequestID: chatsessions.RequestIdentity{
					Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: fmt.Sprintf("conn-%d", i), JSONRPCStringID: "req-1",
				},
				WorkingRoot:   fmt.Sprintf("/workspace/project-%d", i),
				InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
			})
		}(i)
	}
	wg.Wait()

	seenIDs := make(map[string]int, n)
	for i, err := range createErrs {
		if err != nil {
			t.Fatalf("CreateSession[%d]: unexpected error %v", i, err)
		}
		seenIDs[results[i].Session.ID]++
		wantWorkingRoot := fmt.Sprintf("/workspace/project-%d", i)
		if results[i].Session.WorkingRoot != wantWorkingRoot {
			t.Fatalf("CreateSession[%d]: WorkingRoot = %q, want %q", i, results[i].Session.WorkingRoot, wantWorkingRoot)
		}
	}
	for id, count := range seenIDs {
		if count != 1 {
			t.Fatalf("session ID %q was assigned to %d concurrent CreateSession calls, want a unique ID per call", id, count)
		}
	}
	if len(seenIDs) != n {
		t.Fatalf("got %d unique session IDs from %d concurrent CreateSession calls, want %d", len(seenIDs), n, n)
	}

	var mutateWG sync.WaitGroup
	setErrs := make([]error, n)
	for i := range n {
		mutateWG.Add(1)
		go func(i int) {
			defer mutateWG.Done()
			_, setErrs[i] = store.SetTarget(ctx, chatsessions.SetTargetRequest{
				RequestID:       setTargetRequestID(fmt.Sprintf("retarget-%d", i)),
				SessionID:       results[i].Session.ID,
				ExpectedVersion: results[i].Session.Version,
				Target:          otherTarget(),
			})
		}(i)
	}
	mutateWG.Wait()

	for i, err := range setErrs {
		if err != nil {
			t.Fatalf("SetTarget[%d]: unexpected error %v", i, err)
		}
	}
	for i := range n {
		final, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: results[i].Session.ID})
		if err != nil {
			t.Fatalf("GetSession[%d]: %v", i, err)
		}
		if final.Session.SelectedTarget != otherTarget() {
			t.Fatalf("GetSession[%d]: SelectedTarget = %+v, want %+v", i, final.Session.SelectedTarget, otherTarget())
		}
		if final.Session.Version != results[i].Session.Version+1 {
			t.Fatalf("GetSession[%d]: Version = %d, want %d", i, final.Session.Version, results[i].Session.Version+1)
		}
	}
}

// advanceStreamHeadRequest builds the AdvanceStreamHeadRequest a caller would
// issue immediately after an accepted Sequence call: the same source identity
// tuple (matching sequenceRequest's own SourceEventID construction, since
// AdvanceStreamHead now requires the exact tuple that committed the position
// it is asked to advance to), the committed aggregate position, and the
// session version observed before that Sequence call.
func advanceStreamHeadRequest(sessionID string, sourceSeq events.SourceSequence, expectedVersion uint64, position events.AggregateSequence) chatsessions.AdvanceStreamHeadRequest {
	return chatsessions.AdvanceStreamHeadRequest{
		SessionID:         sessionID,
		ExpectedVersion:   expectedVersion,
		AggregateSequence: position,
		SourceType:        "worker",
		SourceID:          "worker-1",
		SourceSequence:    sourceSeq,
		SourceEventID:     events.SourceEventID("event-" + strconv.FormatUint(uint64(sourceSeq), 10)),
	}
}

// TestStore_AdvanceStreamHead_AdvancesToCommittedPosition proves a valid
// advancement against a freshly accepted Sequence commit moves StreamHead to
// exactly that aggregate position and strictly advances Session.Version.
func TestStore_AdvanceStreamHead_AdvancesToCommittedPosition(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	seqResult, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}

	result, err := store.AdvanceStreamHead(ctx, advanceStreamHeadRequest(session.ID, 1, session.Version, seqResult.AggregateSequence))
	if err != nil {
		t.Fatalf("AdvanceStreamHead: %v", err)
	}
	if result.Outcome != chatsessions.AdvanceStreamHeadOutcomeAdvanced {
		t.Fatalf("Outcome = %v, want AdvanceStreamHeadOutcomeAdvanced", result.Outcome)
	}
	if result.Session.StreamHead != uint64(seqResult.AggregateSequence) {
		t.Fatalf("StreamHead = %d, want %d", result.Session.StreamHead, seqResult.AggregateSequence)
	}
	if result.Session.Version != session.Version+1 {
		t.Fatalf("Version = %d, want %d (one strict advance)", result.Session.Version, session.Version+1)
	}
}

// TestStore_AdvanceStreamHead_PreservesUnrelatedSessionFields proves a
// successful advancement never changes SelectedTarget, TargetEpisode,
// ActiveTurnID, or any attachment/control state -- only StreamHead, Version,
// and UpdatedAt move.
func TestStore_AdvanceStreamHead_PreservesUnrelatedSessionFields(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-1"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	seqResult, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}

	result, err := store.AdvanceStreamHead(ctx, advanceStreamHeadRequest(session.ID, 1, started.Session.Version, seqResult.AggregateSequence))
	if err != nil {
		t.Fatalf("AdvanceStreamHead: %v", err)
	}

	if result.Session.SelectedTarget != started.Session.SelectedTarget {
		t.Fatalf("SelectedTarget changed: got %+v, want %+v", result.Session.SelectedTarget, started.Session.SelectedTarget)
	}
	if result.Session.TargetEpisode != started.Session.TargetEpisode {
		t.Fatalf("TargetEpisode changed: got %d, want %d", result.Session.TargetEpisode, started.Session.TargetEpisode)
	}
	if result.Session.ActiveTurnID != started.Turn.ID {
		t.Fatalf("ActiveTurnID changed: got %q, want %q", result.Session.ActiveTurnID, started.Turn.ID)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if _, ok := record.attachments[attached.Attachment.ID]; !ok {
		t.Fatalf("attachment %q lost after AdvanceStreamHead", attached.Attachment.ID)
	}
	if record.attachments[attached.Attachment.ID].AfterSequence != 0 {
		t.Fatalf("attachment AfterSequence moved by AdvanceStreamHead: got %d, want 0", record.attachments[attached.Attachment.ID].AfterSequence)
	}
}

// TestStore_AdvanceStreamHead_StaleVersionIsTypedConflictWithNoPartialState
// proves a genuinely new position with a stale ExpectedVersion reports
// *ConflictError and leaves StreamHead and Version completely unchanged --
// never a partially applied head update.
func TestStore_AdvanceStreamHead_StaleVersionIsTypedConflictWithNoPartialState(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	seqResult, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}

	req := advanceStreamHeadRequest(session.ID, 1, session.Version+1, seqResult.AggregateSequence)
	_, err = store.AdvanceStreamHead(ctx, req)
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("AdvanceStreamHead stale version: got %v, want *ConflictError", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session.StreamHead != 0 {
		t.Fatalf("StreamHead mutated by rejected advancement: got %d, want 0", record.session.StreamHead)
	}
	if record.session.Version != session.Version {
		t.Fatalf("Version mutated by rejected advancement: got %d, want %d", record.session.Version, session.Version)
	}
}

// TestStore_AdvanceStreamHead_AlreadyCurrentReconcilesIdempotently proves a
// repeat advancement to a position StreamHead has already reached converges
// without another mutation, even when ExpectedVersion is now stale --
// mirroring BindFactorySession's idempotent-reconcile precedent.
func TestStore_AdvanceStreamHead_AlreadyCurrentReconcilesIdempotently(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	seqResult, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}
	first, err := store.AdvanceStreamHead(ctx, advanceStreamHeadRequest(session.ID, 1, session.Version, seqResult.AggregateSequence))
	if err != nil {
		t.Fatalf("AdvanceStreamHead (first): %v", err)
	}

	// Repeat with the original (now-stale) ExpectedVersion: idempotent
	// convergence must not require a fresh read.
	second, err := store.AdvanceStreamHead(ctx, advanceStreamHeadRequest(session.ID, 1, session.Version, seqResult.AggregateSequence))
	if err != nil {
		t.Fatalf("AdvanceStreamHead (repeat): %v", err)
	}
	if second.Outcome != chatsessions.AdvanceStreamHeadOutcomeAlreadyCurrent {
		t.Fatalf("repeat Outcome = %v, want AdvanceStreamHeadOutcomeAlreadyCurrent", second.Outcome)
	}
	if second.Session.Version != first.Session.Version {
		t.Fatalf("repeat advancement mutated version: got %d, want %d (no-op)", second.Session.Version, first.Session.Version)
	}
}

// TestStore_AdvanceStreamHead_NeverMovesBackward proves a stale call for a
// position StreamHead has already passed reconciles as already-current
// instead of regressing StreamHead, even with a plausible-looking
// ExpectedVersion.
func TestStore_AdvanceStreamHead_NeverMovesBackward(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	first, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence (1): %v", err)
	}
	second, err := store.Sequence(ctx, sequenceRequest(session.ID, 2, ""))
	if err != nil {
		t.Fatalf("Sequence (2): %v", err)
	}

	advanced, err := store.AdvanceStreamHead(ctx, advanceStreamHeadRequest(session.ID, 2, session.Version, second.AggregateSequence))
	if err != nil {
		t.Fatalf("AdvanceStreamHead to position 2: %v", err)
	}
	if advanced.Session.StreamHead != uint64(second.AggregateSequence) {
		t.Fatalf("StreamHead = %d, want %d", advanced.Session.StreamHead, second.AggregateSequence)
	}

	// A stale request for the earlier position, carrying the version
	// observed before either advancement -- must not regress StreamHead.
	stale, err := store.AdvanceStreamHead(ctx, advanceStreamHeadRequest(session.ID, 1, session.Version, first.AggregateSequence))
	if err != nil {
		t.Fatalf("AdvanceStreamHead stale earlier position: %v", err)
	}
	if stale.Outcome != chatsessions.AdvanceStreamHeadOutcomeAlreadyCurrent {
		t.Fatalf("stale earlier position Outcome = %v, want AdvanceStreamHeadOutcomeAlreadyCurrent", stale.Outcome)
	}
	if stale.Session.StreamHead != uint64(second.AggregateSequence) {
		t.Fatalf("StreamHead regressed: got %d, want unchanged %d", stale.Session.StreamHead, second.AggregateSequence)
	}
	if stale.Session.Version != advanced.Session.Version {
		t.Fatalf("Version mutated by stale earlier-position request: got %d, want unchanged %d", stale.Session.Version, advanced.Session.Version)
	}
}

// TestStore_AdvanceStreamHead_UnknownSessionReportsNotFound proves
// AdvanceStreamHead against a SessionID that does not identify an existing
// session reports *NotFoundError.
func TestStore_AdvanceStreamHead_UnknownSessionReportsNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("id"), fixedClock(time.Now()), nil, nil)

	_, err := store.AdvanceStreamHead(ctx, advanceStreamHeadRequest("does-not-exist", 1, 0, 1))
	if !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("AdvanceStreamHead(unknown session): got %v, want ErrNotFound", err)
	}
}

// TestStore_AdvanceStreamHead_CancelledContextIsRejected proves
// AdvanceStreamHead checks ctx.Err() before taking the Store lock or
// attempting any mutation, so a caller racing a cancellation never observes
// a partial or unconditional head advancement -- mirroring Sequence's own
// TestStore_Sequence_CancelledContextIsRejected precedent.
func TestStore_AdvanceStreamHead_CancelledContextIsRejected(t *testing.T) {
	store, session, _ := newSequencingTestSession(t)
	seqResult, err := store.Sequence(context.Background(), sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := advanceStreamHeadRequest(session.ID, 1, session.Version, seqResult.AggregateSequence)
	if _, err := store.AdvanceStreamHead(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("AdvanceStreamHead(cancelled ctx): got %v, want context.Canceled", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session.StreamHead != 0 {
		t.Fatalf("StreamHead mutated by cancelled-context call: got %d, want 0", record.session.StreamHead)
	}
	if record.session.Version != session.Version {
		t.Fatalf("Version mutated by cancelled-context call: got %d, want %d", record.session.Version, session.Version)
	}
}

// TestStore_AdvanceStreamHead_RejectsUncommittedPosition proves
// AdvanceStreamHead rejects an AggregateSequence this session's sequencer
// never actually committed -- a position no Sequence call ever produced for
// this session at all -- with *UncommittedStreamPositionError and no
// mutation. Trusting an unvalidated position here is exactly what let a
// caller advance StreamHead to fabricated state before this fix.
func TestStore_AdvanceStreamHead_RejectsUncommittedPosition(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	req := advanceStreamHeadRequest(session.ID, 1, session.Version, 999)
	_, err := store.AdvanceStreamHead(ctx, req)
	var uncommitted *chatsessions.UncommittedStreamPositionError
	if !errors.As(err, &uncommitted) {
		t.Fatalf("AdvanceStreamHead(never-sequenced position): got %v, want *UncommittedStreamPositionError", err)
	}
	if !errors.Is(err, chatsessions.ErrUncommittedStreamPosition) {
		t.Fatalf("AdvanceStreamHead(never-sequenced position): got %v, want ErrUncommittedStreamPosition", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session.StreamHead != 0 {
		t.Fatalf("StreamHead mutated by rejected request: got %d, want 0", record.session.StreamHead)
	}
	if record.session.Version != session.Version {
		t.Fatalf("Version mutated by rejected request: got %d, want %d", record.session.Version, session.Version)
	}
}

// TestStore_AdvanceStreamHead_RejectsCrossSessionPosition proves a position
// genuinely committed by one session's sequencer is not a valid
// AdvanceStreamHead target for a different session, even though it is a
// well-formed, real, already-assigned aggregate position -- mirroring
// Sequence's own TestStore_Sequence_ParentFromAnotherSessionIsRejected
// precedent for cross-session identity reuse.
func TestStore_AdvanceStreamHead_RejectsCrossSessionPosition(t *testing.T) {
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

	inA, err := store.Sequence(ctx, sequenceRequest(sessionA.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence in session A: %v", err)
	}

	// Session B's own sequencer never committed inA's position; reusing that
	// exact (position, source identity) pair against session B must be
	// rejected, not silently accepted because the position and tuple are
	// individually well-formed and real.
	req := advanceStreamHeadRequest(createdB.Session.ID, 1, createdB.Session.Version, inA.AggregateSequence)
	_, err = store.AdvanceStreamHead(ctx, req)
	if !errors.Is(err, chatsessions.ErrUncommittedStreamPosition) {
		t.Fatalf("AdvanceStreamHead(session B, session A's position): got %v, want ErrUncommittedStreamPosition", err)
	}

	store.mu.RLock()
	record := store.sessions[createdB.Session.ID]
	store.mu.RUnlock()
	if record.session.StreamHead != 0 {
		t.Fatalf("session B StreamHead mutated by rejected cross-session request: got %d, want 0", record.session.StreamHead)
	}
}

// TestStore_AdvanceStreamHead_RejectsMismatchedSourceIdentityForCommittedPosition
// proves that even when the requested AggregateSequence is a real position
// this exact session's sequencer committed, AdvanceStreamHead still rejects
// it if the caller's stated source identity does not match the tuple that
// actually committed it -- the position alone is never sufficient proof.
func TestStore_AdvanceStreamHead_RejectsMismatchedSourceIdentityForCommittedPosition(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	seqResult, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}

	req := advanceStreamHeadRequest(session.ID, 1, session.Version, seqResult.AggregateSequence)
	req.SourceEventID = "event-does-not-match"
	_, err = store.AdvanceStreamHead(ctx, req)
	var uncommitted *chatsessions.UncommittedStreamPositionError
	if !errors.As(err, &uncommitted) {
		t.Fatalf("AdvanceStreamHead(mismatched source identity): got %v, want *UncommittedStreamPositionError", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session.StreamHead != 0 {
		t.Fatalf("StreamHead mutated by rejected request: got %d, want 0", record.session.StreamHead)
	}
}

// TestStore_AdvanceStreamHead_InvalidRequestIsRejected table-drives every
// AdvanceStreamHeadRequest validation failure and proves each is rejected
// before any mutation.
func TestStore_AdvanceStreamHead_InvalidRequestIsRejected(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		mutate  func(chatsessions.AdvanceStreamHeadRequest) chatsessions.AdvanceStreamHeadRequest
		wantErr error
	}{
		{
			name: "blank session id",
			mutate: func(r chatsessions.AdvanceStreamHeadRequest) chatsessions.AdvanceStreamHeadRequest {
				r.SessionID = ""
				return r
			},
			wantErr: chatsessions.ErrRequiredValue,
		},
		{
			name: "zero aggregate sequence",
			mutate: func(r chatsessions.AdvanceStreamHeadRequest) chatsessions.AdvanceStreamHeadRequest {
				r.AggregateSequence = 0
				return r
			},
			wantErr: events.ErrInvalidAggregateSequence,
		},
		{
			name: "blank source type",
			mutate: func(r chatsessions.AdvanceStreamHeadRequest) chatsessions.AdvanceStreamHeadRequest {
				r.SourceType = ""
				return r
			},
			wantErr: events.ErrEmptySourceType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, session, _ := newSequencingTestSession(t)
			req := tt.mutate(advanceStreamHeadRequest(session.ID, 1, session.Version, 1))
			if _, err := store.AdvanceStreamHead(ctx, req); !errors.Is(err, tt.wantErr) {
				t.Fatalf("AdvanceStreamHead(%s): got %v, want %v", tt.name, err, tt.wantErr)
			}
			store.mu.RLock()
			record := store.sessions[session.ID]
			store.mu.RUnlock()
			if record.session.StreamHead != 0 {
				t.Fatalf("StreamHead mutated by rejected request (%s) = %d, want 0", tt.name, record.session.StreamHead)
			}
		})
	}
}

// TestStore_AdvanceStreamHead_ConcurrentSamePositionConverges races many
// goroutines advancing to the exact same aggregate position with the exact
// same ExpectedVersion, released through a shared start barrier (never a
// sleep). Exactly one call observes AdvanceStreamHeadOutcomeAdvanced; every
// other call safely reconciles as AlreadyCurrent instead of a version
// conflict, and the final session shows exactly one version increment -- no
// torn or partial state visible under concurrent scheduling.
func TestStore_AdvanceStreamHead_ConcurrentSamePositionConverges(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	seqResult, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}
	req := advanceStreamHeadRequest(session.ID, 1, session.Version, seqResult.AggregateSequence)

	const n = 25
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]chatsessions.AdvanceStreamHeadResult, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = store.AdvanceStreamHead(ctx, req)
		}(i)
	}
	close(start)
	wg.Wait()

	advancedCount := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("AdvanceStreamHead[%d]: got %v, want success (idempotent convergence)", i, err)
		}
		if results[i].Session.StreamHead != uint64(seqResult.AggregateSequence) {
			t.Fatalf("AdvanceStreamHead[%d]: StreamHead = %d, want %d", i, results[i].Session.StreamHead, seqResult.AggregateSequence)
		}
		if results[i].Outcome == chatsessions.AdvanceStreamHeadOutcomeAdvanced {
			advancedCount++
		}
	}
	if advancedCount != 1 {
		t.Fatalf("advanced outcome count = %d, want exactly 1", advancedCount)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session.Version != session.Version+1 {
		t.Fatalf("final version = %d, want exactly one increment past %d", record.session.Version, session.Version)
	}
}

// TestStore_AdvanceStreamHead_SafeLogFields proves AdvanceStreamHead logs the
// session, source identity, aggregate position, version, and outcome -- and
// never the session's WorkingRoot or any other unsafe field.
func TestStore_AdvanceStreamHead_SafeLogFields(t *testing.T) {
	logger, calls := newCaptureLogger()
	store := NewStore(sequentialIDs("id"), fixedClock(time.Now()), newFakeEventsAppender(), nil, logger)

	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	seqResult, err := store.Sequence(context.Background(), sequenceRequest(created.Session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}
	*calls = nil // discard CreateSession/Sequence's own log calls

	req := advanceStreamHeadRequest(created.Session.ID, 1, created.Session.Version, seqResult.AggregateSequence)
	result, err := store.AdvanceStreamHead(context.Background(), req)
	if err != nil {
		t.Fatalf("AdvanceStreamHead: %v", err)
	}

	if len(*calls) != 2 {
		t.Fatalf("AdvanceStreamHead logged %d calls, want 2 (start, outcome): %+v", len(*calls), *calls)
	}
	outcome := (*calls)[1]
	if !hasKV(outcome.kv, "session_id", created.Session.ID) {
		t.Fatalf("outcome log missing session_id=%q: %+v", created.Session.ID, outcome.kv)
	}
	if !hasKV(outcome.kv, "source_type", string(req.SourceType)) {
		t.Fatalf("outcome log missing source_type=%q: %+v", req.SourceType, outcome.kv)
	}
	if !hasKV(outcome.kv, "source_id", string(req.SourceID)) {
		t.Fatalf("outcome log missing source_id=%q: %+v", req.SourceID, outcome.kv)
	}
	if !hasKV(outcome.kv, "aggregate_sequence", uint64(seqResult.AggregateSequence)) {
		t.Fatalf("outcome log missing aggregate_sequence=%d: %+v", seqResult.AggregateSequence, outcome.kv)
	}
	if !hasKV(outcome.kv, "version", result.Session.Version) {
		t.Fatalf("outcome log missing version=%d: %+v", result.Session.Version, outcome.kv)
	}
	if !hasKV(outcome.kv, "outcome", "advanced") {
		t.Fatalf("outcome log missing outcome=advanced: %+v", outcome.kv)
	}
	if !hasKV(outcome.kv, "error_class", "") {
		t.Fatalf("successful outcome log must carry an empty error_class: %+v", outcome.kv)
	}

	assertNoUnsafeFields(t, *calls, created.Session.WorkingRoot)
}
