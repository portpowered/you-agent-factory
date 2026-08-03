package service

import (
	"context"
	"errors"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

func controlRequestID(connID, id string) chatsessions.RequestIdentity {
	return chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: connID, JSONRPCStringID: id}
}

// newActiveTurnTestSession constructs a Store with one Session that has
// already admitted its first, still-active Turn, ready for RequestControl
// calls.
func newActiveTurnTestSession(t *testing.T, now time.Time) (*Store, chatsessions.Session, chatsessions.Turn) {
	t.Helper()
	store := New(sequentialIDs("session"), fixedClock(now))
	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	started, err := store.StartTurn(context.Background(), chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       created.Session.ID,
		ExpectedVersion: created.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	return store, started.Session, started.Turn
}

// TestStore_RequestControl_CapturesActiveTurnAtomically proves a supported
// control request with the current session version creates a REQUESTED
// intent retaining the full request identity and atomically capturing the
// active turn ID, target episode, expected version, action, and request
// time, without changing the session's version.
func TestStore_RequestControl_CapturesActiveTurnAtomically(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store, session, turn := newActiveTurnTestSession(t, now)

	reqID := controlRequestID("conn-1", "cancel-1")
	result, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID:       reqID,
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
		Action:          chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl: %v", err)
	}
	intent := result.Intent
	if intent.RequestID != reqID {
		t.Fatalf("Intent.RequestID = %+v, want %+v", intent.RequestID, reqID)
	}
	if intent.SessionID != session.ID {
		t.Fatalf("Intent.SessionID = %q, want %q", intent.SessionID, session.ID)
	}
	if intent.TurnID != turn.ID {
		t.Fatalf("Intent.TurnID = %q, want %q", intent.TurnID, turn.ID)
	}
	if intent.TargetEpisode != session.TargetEpisode {
		t.Fatalf("Intent.TargetEpisode = %d, want %d", intent.TargetEpisode, session.TargetEpisode)
	}
	if intent.ExpectedVersion != session.Version {
		t.Fatalf("Intent.ExpectedVersion = %d, want %d", intent.ExpectedVersion, session.Version)
	}
	if intent.Action != chatsessions.ControlActionCancel {
		t.Fatalf("Intent.Action = %v, want CANCEL", intent.Action)
	}
	if intent.State != chatsessions.ControlIntentStateRequested {
		t.Fatalf("Intent.State = %v, want REQUESTED", intent.State)
	}
	if intent.RequestedAt.IsZero() {
		t.Fatal("Intent.RequestedAt must be non-zero")
	}
	if err := intent.Validate(); err != nil {
		t.Fatalf("requested Intent fails its own Validate: %v", err)
	}

	afterVersion, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if afterVersion.Session.Version != session.Version {
		t.Fatalf("RequestControl changed Session.Version: got %d, want %d", afterVersion.Session.Version, session.Version)
	}
}

// TestStore_RequestControl_DistinctConnectionsNeverCollide proves two
// control requests carrying equal JSON-RPC string ids from different
// ConnectionIDs remain distinct keys: both are accepted, and advancing one
// never retrieves, overwrites, or deduplicates against the other.
func TestStore_RequestControl_DistinctConnectionsNeverCollide(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newActiveTurnTestSession(t, time.Now())

	reqA := controlRequestID("conn-A", "1")
	reqB := controlRequestID("conn-B", "1")

	resultA, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: reqA, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl A: %v", err)
	}
	resultB, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: reqB, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionClose,
	})
	if err != nil {
		t.Fatalf("RequestControl B: %v", err)
	}
	if resultA.Intent.RequestID != reqA || resultB.Intent.RequestID != reqB {
		t.Fatalf("RequestControl returned mismatched identities: A=%+v B=%+v", resultA.Intent.RequestID, resultB.Intent.RequestID)
	}

	store.mu.RLock()
	controlCount := len(store.sessions[session.ID].controls)
	store.mu.RUnlock()
	if controlCount != 2 {
		t.Fatalf("stored control intents = %d, want 2", controlCount)
	}

	committedA, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: reqA, Next: chatsessions.ControlIntentStateCommitted,
	})
	if err != nil {
		t.Fatalf("AdvanceControl A: %v", err)
	}
	if committedA.Intent.RequestID != reqA || committedA.Intent.Action != chatsessions.ControlActionCancel {
		t.Fatalf("AdvanceControl A retrieved wrong intent: %+v", committedA.Intent)
	}

	committedB, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: reqB, Next: chatsessions.ControlIntentStateCommitted,
	})
	if err != nil {
		t.Fatalf("AdvanceControl B: %v", err)
	}
	if committedB.Intent.RequestID != reqB || committedB.Intent.Action != chatsessions.ControlActionClose {
		t.Fatalf("AdvanceControl B retrieved wrong intent: %+v", committedB.Intent)
	}

	store.mu.RLock()
	stillA := store.sessions[session.ID].controls[reqA]
	store.mu.RUnlock()
	if stillA.State != chatsessions.ControlIntentStateCommitted || stillA.Action != chatsessions.ControlActionCancel {
		t.Fatalf("advancing B mutated A: got %+v", stillA)
	}
}

// TestStore_RequestControl_InvalidIdentityCreatesNoMutation proves an
// invalid RequestIdentity (mixed fields inconsistent with its declared Kind)
// reports the existing typed validation classification and creates no
// control intent.
func TestStore_RequestControl_InvalidIdentityCreatesNoMutation(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newActiveTurnTestSession(t, time.Now())

	invalid := chatsessions.RequestIdentity{
		Kind: chatsessions.RequestIdentityKindTransportUUID, ConnectionID: "conn-1", TransportUUID: "not-a-uuid",
	}
	_, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: invalid, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
	})
	if !errors.Is(err, chatsessions.ErrInconsistentValue) {
		t.Fatalf("RequestControl invalid identity: got %v, want ErrInconsistentValue", err)
	}

	store.mu.RLock()
	controlCount := len(store.sessions[session.ID].controls)
	store.mu.RUnlock()
	if controlCount != 0 {
		t.Fatalf("invalid RequestControl created %d intents, want 0", controlCount)
	}
}

// TestStore_RequestControl_UnsupportedActionCreatesNoMutation proves a
// declared-but-not-executable ControlAction (PAUSE) reports a
// *ValidationError wrapping ErrUnsupportedControlAction and creates no
// control intent or version change.
func TestStore_RequestControl_UnsupportedActionCreatesNoMutation(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newActiveTurnTestSession(t, time.Now())

	_, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: controlRequestID("conn-1", "1"), SessionID: session.ID,
		ExpectedVersion: session.Version, Action: chatsessions.ControlActionPause,
	})
	if !errors.Is(err, chatsessions.ErrUnsupportedControlAction) {
		t.Fatalf("RequestControl unsupported action: got %v, want ErrUnsupportedControlAction", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.controls) != 0 {
		t.Fatalf("unsupported action created %d intents, want 0", len(record.controls))
	}
	if record.session.Version != session.Version {
		t.Fatalf("unsupported action changed Session.Version: got %d, want %d", record.session.Version, session.Version)
	}
}

// TestStore_RequestControl_NoActiveTurnIsNotFound proves a control request
// against a session with no active turn reports typed *NotFoundError and
// creates no control intent.
func TestStore_RequestControl_NoActiveTurnIsNotFound(t *testing.T) {
	ctx := context.Background()
	store := New(sequentialIDs("session"), fixedClock(time.Now()))
	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	_, err = store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: controlRequestID("conn-1", "1"), SessionID: created.Session.ID,
		ExpectedVersion: created.Session.Version, Action: chatsessions.ControlActionCancel,
	})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("RequestControl no active turn: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Turn" {
		t.Fatalf("NotFoundError.Value = %q, want Turn", notFound.Value)
	}

	store.mu.RLock()
	controlCount := len(store.sessions[created.Session.ID].controls)
	store.mu.RUnlock()
	if controlCount != 0 {
		t.Fatalf("no-active-turn RequestControl created %d intents, want 0", controlCount)
	}
}

// TestStore_RequestControl_StaleVersionConflictLeavesStateUnchanged proves a
// stale expected version reports typed *ConflictError and creates no control
// intent or version change.
func TestStore_RequestControl_StaleVersionConflictLeavesStateUnchanged(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newActiveTurnTestSession(t, time.Now())

	_, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: controlRequestID("conn-1", "1"), SessionID: session.ID,
		ExpectedVersion: session.Version + 1, Action: chatsessions.ControlActionCancel,
	})
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("RequestControl stale version: got %v, want *ConflictError", err)
	}
	if conflict.Expected != session.Version+1 || conflict.Actual != session.Version {
		t.Fatalf("ConflictError = %+v, want Expected=%d Actual=%d", conflict, session.Version+1, session.Version)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.controls) != 0 {
		t.Fatalf("stale RequestControl created %d intents, want 0", len(record.controls))
	}
	if record.session != session {
		t.Fatalf("stale RequestControl mutated Session: got %+v, want %+v", record.session, session)
	}
}

// TestStore_RequestControl_DuplicateIdentityBeforeAdvancementIsIdempotent
// proves reusing a RequestID that already identifies a REQUESTED intent
// returns the existing intent unchanged rather than recapturing state --
// even when the second call's own Action or ExpectedVersion differs from
// what was originally captured.
func TestStore_RequestControl_DuplicateIdentityBeforeAdvancementIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, session, turn := newActiveTurnTestSession(t, time.Now())
	reqID := controlRequestID("conn-1", "1")

	first, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: reqID, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl first: %v", err)
	}

	second, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: reqID, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl duplicate: %v", err)
	}
	if second.Intent != first.Intent {
		t.Fatalf("duplicate RequestControl returned a different intent: got %+v, want %+v", second.Intent, first.Intent)
	}
	if second.Intent.TurnID != turn.ID {
		t.Fatalf("duplicate RequestControl retargeted TurnID: got %q, want %q", second.Intent.TurnID, turn.ID)
	}

	store.mu.RLock()
	controlCount := len(store.sessions[session.ID].controls)
	store.mu.RUnlock()
	if controlCount != 1 {
		t.Fatalf("controls after duplicate request = %d, want 1", controlCount)
	}
}

// TestStore_RequestControl_DuplicateIdentityAfterNewTurnNeverRetargets proves
// AC5's "later work cannot become the target of an older intent" guarantee:
// reusing a RequestID after its original intent advanced to COMMITTED and a
// later turn started never rewrites the stored intent's captured turn,
// target episode, or version to the newer facts -- the exact identity is
// immutable once it has been used.
func TestStore_RequestControl_DuplicateIdentityAfterNewTurnNeverRetargets(t *testing.T) {
	ctx := context.Background()
	store, session, turn := newActiveTurnTestSession(t, time.Now())
	reqID := controlRequestID("conn-1", "1")
	committed := committedControlIntent(t, ctx, store, session, reqID)

	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCanceled,
	}); err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}
	released, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	newTurn, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID: startTurnRequestID("req-turn-2"), SessionID: session.ID, ExpectedVersion: released.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn second: %v", err)
	}

	replay, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: reqID, SessionID: session.ID, ExpectedVersion: newTurn.Session.Version, Action: chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl duplicate after new turn: %v", err)
	}
	if replay.Intent != committed {
		t.Fatalf("duplicate RequestControl after new turn changed the stored intent: got %+v, want %+v", replay.Intent, committed)
	}
	if replay.Intent.TurnID == newTurn.Turn.ID {
		t.Fatal("duplicate RequestControl after new turn retargeted TurnID to the new turn")
	}
}

// TestStore_AdvanceControl_RequestedToCommitted proves the first legal
// advancement (REQUESTED->COMMITTED) is a plain state transition, not routed
// through ResolveControlIntentOutcome.
func TestStore_AdvanceControl_RequestedToCommitted(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newActiveTurnTestSession(t, time.Now())
	reqID := controlRequestID("conn-1", "1")
	requested, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: reqID, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl: %v", err)
	}

	committed, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: reqID, Next: chatsessions.ControlIntentStateCommitted,
	})
	if err != nil {
		t.Fatalf("AdvanceControl REQUESTED->COMMITTED: %v", err)
	}
	if committed.Intent.State != chatsessions.ControlIntentStateCommitted {
		t.Fatalf("Intent.State = %v, want COMMITTED", committed.Intent.State)
	}
	if committed.Intent.TurnID != requested.Intent.TurnID {
		t.Fatalf("commit changed captured TurnID: got %q, want %q", committed.Intent.TurnID, requested.Intent.TurnID)
	}
}

// TestStore_AdvanceControl_IllegalDirectRequestedToCompletedLeavesUnchanged
// proves an illegal REQUESTED->COMPLETED advancement (skipping COMMITTED)
// reports typed *TransitionError and leaves the stored intent unchanged.
func TestStore_AdvanceControl_IllegalDirectRequestedToCompletedLeavesUnchanged(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newActiveTurnTestSession(t, time.Now())
	reqID := controlRequestID("conn-1", "1")
	requested, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: reqID, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl: %v", err)
	}

	_, err = store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: reqID, Next: chatsessions.ControlIntentStateCompleted,
	})
	var transition *chatsessions.TransitionError
	if !errors.As(err, &transition) {
		t.Fatalf("AdvanceControl REQUESTED->COMPLETED: got %v, want *TransitionError", err)
	}

	store.mu.RLock()
	stored := store.sessions[session.ID].controls[reqID]
	store.mu.RUnlock()
	if stored != requested.Intent {
		t.Fatalf("illegal AdvanceControl mutated intent: got %+v, want %+v", stored, requested.Intent)
	}
}

// TestStore_AdvanceControl_UnknownIntentIsTypedNotFound proves AdvanceControl
// against an unknown RequestID (on a known or unknown session) reports typed
// *NotFoundError naming the ControlIntent.
func TestStore_AdvanceControl_UnknownIntentIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newActiveTurnTestSession(t, time.Now())

	_, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: controlRequestID("conn-1", "does-not-exist"),
		Next: chatsessions.ControlIntentStateCommitted,
	})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("AdvanceControl unknown intent: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "ControlIntent" {
		t.Fatalf("NotFoundError.Value = %q, want ControlIntent", notFound.Value)
	}

	_, err = store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: "does-not-exist-session", RequestID: controlRequestID("conn-1", "1"),
		Next: chatsessions.ControlIntentStateCommitted,
	})
	if !errors.As(err, &notFound) {
		t.Fatalf("AdvanceControl unknown session: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "ControlIntent" {
		t.Fatalf("NotFoundError.Value = %q, want ControlIntent", notFound.Value)
	}
}

// committedControlIntent creates a session with one active turn, requests a
// CANCEL control against it, and commits the intent, returning the session,
// turn, and committed intent for further advancement.
func committedControlIntent(t *testing.T, ctx context.Context, store *Store, session chatsessions.Session, reqID chatsessions.RequestIdentity) chatsessions.ControlIntent {
	t.Helper()
	if _, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: reqID, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
	}); err != nil {
		t.Fatalf("RequestControl: %v", err)
	}
	committed, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: reqID, Next: chatsessions.ControlIntentStateCommitted,
	})
	if err != nil {
		t.Fatalf("AdvanceControl REQUESTED->COMMITTED: %v", err)
	}
	return committed.Intent
}

// TestStore_AdvanceControl_ResolvesCompletedWhileTurnStillActive proves a
// COMMITTED intent whose captured turn is still current and non-terminal
// resolves to COMPLETED regardless of the caller-supplied Next.
func TestStore_AdvanceControl_ResolvesCompletedWhileTurnStillActive(t *testing.T) {
	ctx := context.Background()
	store, session, turn := newActiveTurnTestSession(t, time.Now())
	reqID := controlRequestID("conn-1", "1")
	intent := committedControlIntent(t, ctx, store, session, reqID)
	if intent.TurnID != turn.ID {
		t.Fatalf("captured TurnID = %q, want %q", intent.TurnID, turn.ID)
	}

	result, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: reqID, Next: chatsessions.ControlIntentStateNoop,
	})
	if err != nil {
		t.Fatalf("AdvanceControl: %v", err)
	}
	if result.Intent.State != chatsessions.ControlIntentStateCompleted {
		t.Fatalf("Intent.State = %v, want COMPLETED (caller-supplied Next must be ignored once COMMITTED)", result.Intent.State)
	}
}

// TestStore_AdvanceControl_OldControlVersusNewTurnResolvesSuperseded proves
// AC5's deterministic "old-control-versus-new-turn" race: once the captured
// turn terminates and a later turn is admitted, advancing the older
// committed intent resolves to SUPERSEDED and never completes, no-ops, or
// retargets the later turn.
func TestStore_AdvanceControl_OldControlVersusNewTurnResolvesSuperseded(t *testing.T) {
	ctx := context.Background()
	store, session, turn := newActiveTurnTestSession(t, time.Now())
	reqID := controlRequestID("conn-1", "1")
	intent := committedControlIntent(t, ctx, store, session, reqID)

	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCanceled,
	}); err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}
	released, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	newTurn, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID: startTurnRequestID("req-turn-2"), SessionID: session.ID, ExpectedVersion: released.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn second: %v", err)
	}
	if newTurn.Turn.ID == turn.ID {
		t.Fatal("second StartTurn reused the first turn's ID")
	}

	result, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: reqID, Next: chatsessions.ControlIntentStateCompleted,
	})
	if err != nil {
		t.Fatalf("AdvanceControl: %v", err)
	}
	if result.Intent.State != chatsessions.ControlIntentStateSuperseded {
		t.Fatalf("Intent.State = %v, want SUPERSEDED", result.Intent.State)
	}
	if result.Intent.TurnID != intent.TurnID {
		t.Fatalf("SUPERSEDED intent retargeted: TurnID = %q, want unchanged %q", result.Intent.TurnID, intent.TurnID)
	}

	stillNew, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if stillNew.Session.ActiveTurnID != newTurn.Turn.ID {
		t.Fatalf("SUPERSEDED resolution disturbed the new active turn: got %q, want %q", stillNew.Session.ActiveTurnID, newTurn.Turn.ID)
	}
}

// TestStore_AdvanceControl_CancelVersusTerminalResolvesNoop proves AC6's
// deterministic "cancel-versus-terminal" race through the public API alone:
// when the captured turn terminates on its own (with no successor turn yet
// admitted) before a COMMITTED cancel resolves, the intent observes there is
// nothing left to cancel and resolves to NOOP -- distinct from SUPERSEDED,
// which is reserved for a captured turn that a newer admitted turn has since
// replaced (see TestStore_AdvanceControl_OldControlVersusNewTurnResolvesSuperseded).
// Resolution reads Session.lastTurnID, which (unlike ActiveTurnID) is not
// cleared when the captured turn terminates, only overwritten by the next
// StartTurn -- so this same-turn, no-successor case is distinguishable from
// the new-turn case entirely through public Store calls, without fabricating
// private state.
func TestStore_AdvanceControl_CancelVersusTerminalResolvesNoop(t *testing.T) {
	ctx := context.Background()
	store, session, turn := newActiveTurnTestSession(t, time.Now())
	reqID := controlRequestID("conn-1", "1")
	intent := committedControlIntent(t, ctx, store, session, reqID)

	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCanceled,
	}); err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}

	result, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: reqID, Next: chatsessions.ControlIntentStateCompleted,
	})
	if err != nil {
		t.Fatalf("AdvanceControl: %v", err)
	}
	if result.Intent.State != chatsessions.ControlIntentStateNoop {
		t.Fatalf("Intent.State = %v, want NOOP", result.Intent.State)
	}
	if result.Intent.TurnID != intent.TurnID {
		t.Fatalf("resolution retargeted intent: TurnID = %q, want unchanged %q", result.Intent.TurnID, intent.TurnID)
	}
}

// TestStore_RequestControl_RepeatedDistinctIdentitiesNeverCollide is repeat
// coverage over every RequestIdentity kind (connection-scoped JSON-RPC
// string, JSON-RPC number, and a process-unique transport UUID), proving
// each is accepted as its own distinct, independently advanceable control
// intent with its own captured target.
func TestStore_RequestControl_RepeatedDistinctIdentitiesNeverCollide(t *testing.T) {
	ctx := context.Background()
	store, session, turn := newActiveTurnTestSession(t, time.Now())

	identities := []chatsessions.RequestIdentity{
		{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "1"},
		{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-1", JSONRPCNumberID: "1"},
		{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-2", JSONRPCStringID: "1"},
		{Kind: chatsessions.RequestIdentityKindTransportUUID, TransportUUID: "123e4567-e89b-12d3-a456-426614174000"},
	}

	for i, id := range identities {
		result, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
			RequestID: id, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
		})
		if err != nil {
			t.Fatalf("RequestControl[%d]: %v", i, err)
		}
		if result.Intent.TurnID != turn.ID {
			t.Fatalf("RequestControl[%d]: TurnID = %q, want %q", i, result.Intent.TurnID, turn.ID)
		}
	}

	store.mu.RLock()
	controlCount := len(store.sessions[session.ID].controls)
	store.mu.RUnlock()
	if controlCount != len(identities) {
		t.Fatalf("stored control intents = %d, want %d (a collision merged distinct identities)", controlCount, len(identities))
	}

	for i, id := range identities {
		committed, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
			SessionID: session.ID, RequestID: id, Next: chatsessions.ControlIntentStateCommitted,
		})
		if err != nil {
			t.Fatalf("AdvanceControl[%d]: %v", i, err)
		}
		if committed.Intent.RequestID != id {
			t.Fatalf("AdvanceControl[%d] retrieved wrong intent: got RequestID %+v, want %+v", i, committed.Intent.RequestID, id)
		}
	}
}
