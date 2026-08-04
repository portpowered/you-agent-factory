package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

func startTurnRequestID(id string) chatsessions.RequestIdentity {
	return chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: id}
}

// newStartTurnTestSession constructs a Store and one created Session ready
// for StartTurn calls.
func newStartTurnTestSession(t *testing.T, now time.Time) (*Store, chatsessions.Session) {
	t.Helper()
	store := NewStore(sequentialIDs("session"), fixedClock(now), nil, nil)
	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return store, created.Session
}

// TestStore_StartTurn_FirstTurnActivatesSession proves admitting the first
// turn creates one ADMITTED turn bound to the current target episode and
// full request identity, moves a CREATED session to ACTIVE, sets
// ActiveTurnID, and returns a strictly newer session version.
func TestStore_StartTurn_FirstTurnActivatesSession(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	reqID := startTurnRequestID("req-turn-1")
	result, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       reqID,
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if result.Turn.ID == "" {
		t.Fatal("StartTurn: expected a non-blank turn ID")
	}
	if result.Turn.State != chatsessions.TurnStateAdmitted {
		t.Fatalf("Turn.State = %v, want ADMITTED", result.Turn.State)
	}
	if result.Turn.Episode != session.TargetEpisode {
		t.Fatalf("Turn.Episode = %d, want %d", result.Turn.Episode, session.TargetEpisode)
	}
	if result.Turn.RequestID != reqID {
		t.Fatalf("Turn.RequestID = %+v, want %+v", result.Turn.RequestID, reqID)
	}
	if result.Session.State != chatsessions.SessionStateActive {
		t.Fatalf("Session.State = %v, want ACTIVE", result.Session.State)
	}
	if result.Session.ActiveTurnID != result.Turn.ID {
		t.Fatalf("Session.ActiveTurnID = %q, want %q", result.Session.ActiveTurnID, result.Turn.ID)
	}
	if result.Session.Version <= session.Version {
		t.Fatalf("Session.Version = %d, want strictly greater than %d", result.Session.Version, session.Version)
	}
	if err := result.Turn.Validate(); err != nil {
		t.Fatalf("admitted Turn fails its own Validate: %v", err)
	}
}

// TestStore_StartTurn_ReturnsAdmittedEpisodeSnapshot proves StartTurnResult
// carries the exact current TargetEpisode the turn was admitted into
// (Number and Target), with a blank FactorySessionID for a brand-new
// episode, so a caller can decide whether to start or reuse a Factory
// Session without a second read.
func TestStore_StartTurn_ReturnsAdmittedEpisodeSnapshot(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	result, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if result.Episode.Number != session.TargetEpisode {
		t.Fatalf("Episode.Number = %d, want %d", result.Episode.Number, session.TargetEpisode)
	}
	if result.Episode.Target != session.SelectedTarget {
		t.Fatalf("Episode.Target = %+v, want %+v", result.Episode.Target, session.SelectedTarget)
	}
	if result.Episode.FactorySessionID != "" {
		t.Fatalf("Episode.FactorySessionID = %q, want blank for a brand-new episode", result.Episode.FactorySessionID)
	}
	if result.Episode.State != chatsessions.TargetEpisodeStateOpen {
		t.Fatalf("Episode.State = %v, want OPEN", result.Episode.State)
	}
}

// TestStore_StartTurn_SecondAdmissionWhileActiveIsBusy proves a second
// sequential admission while the first turn is non-terminal reports typed
// *BusyError, creates no additional active turn, and leaves the accepted
// turn unchanged.
func TestStore_StartTurn_SecondAdmissionWhileActiveIsBusy(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}

	_, err = store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-2"),
		SessionID:       session.ID,
		ExpectedVersion: first.Session.Version,
	})
	var busy *chatsessions.BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("StartTurn second: got %v, want *BusyError", err)
	}
	if busy.ActiveTurnID != first.Turn.ID || busy.ActiveTurnState != chatsessions.TurnStateAdmitted {
		t.Fatalf("BusyError = %+v, want ActiveTurnID=%q ActiveTurnState=ADMITTED", busy, first.Turn.ID)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.turns) != 1 {
		t.Fatalf("turns after busy admission = %d, want 1", len(record.turns))
	}
	if got := record.turns[first.Turn.ID]; got != first.Turn {
		t.Fatalf("accepted turn changed after busy admission: got %+v, want %+v", got, first.Turn)
	}
	if record.session != first.Session {
		t.Fatalf("session changed after busy admission: got %+v, want %+v", record.session, first.Session)
	}
}

// TestStore_StartTurn_BusyReportsLiveRunningState proves a second admission
// against a turn already advanced to RUNNING reports *BusyError carrying the
// turn's live RUNNING state, not the ADMITTED state it was created with. An
// earlier revision cached a second, independently-mutable copy of the active
// turn that AdvanceTurn's non-terminal path never refreshed, so this busy
// check reported a stale ADMITTED state after a public ADMITTED->RUNNING
// advancement.
func TestStore_StartTurn_BusyReportsLiveRunningState(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}
	advanced, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    first.Turn.ID,
		Next:      chatsessions.TurnStateRunning,
	})
	if err != nil {
		t.Fatalf("AdvanceTurn to RUNNING: %v", err)
	}

	_, err = store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-2"),
		SessionID:       session.ID,
		ExpectedVersion: first.Session.Version,
	})
	var busy *chatsessions.BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("StartTurn while running: got %v, want *BusyError", err)
	}
	if busy.ActiveTurnID != first.Turn.ID || busy.ActiveTurnState != chatsessions.TurnStateRunning {
		t.Fatalf("BusyError = %+v, want ActiveTurnID=%q ActiveTurnState=RUNNING", busy, first.Turn.ID)
	}
	if advanced.Turn.State != chatsessions.TurnStateRunning {
		t.Fatalf("AdvanceTurn result state = %v, want RUNNING", advanced.Turn.State)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session.Version != first.Session.Version {
		t.Fatalf("session mutated by busy StartTurn: version = %d, want %d", record.session.Version, first.Session.Version)
	}
}

// TestStore_StartTurn_StaleVersionConflictLeavesStateUnchanged proves a
// stale-version admission reports typed *ConflictError without creating a
// turn or changing session state, timestamps, active-turn identity, or
// version.
func TestStore_StartTurn_StaleVersionConflictLeavesStateUnchanged(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	_, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version + 1,
	})
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("StartTurn stale version: got %v, want *ConflictError", err)
	}
	if conflict.Expected != session.Version+1 || conflict.Actual != session.Version {
		t.Fatalf("ConflictError = %+v, want Expected=%d Actual=%d", conflict, session.Version+1, session.Version)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session != session {
		t.Fatalf("stale StartTurn mutated Session: got %+v, want %+v", record.session, session)
	}
	if len(record.turns) != 0 {
		t.Fatalf("stale StartTurn created %d turns, want 0", len(record.turns))
	}
	if active, ok := record.activeTurnValue(); ok {
		t.Fatalf("stale StartTurn set an active turn: %+v", active)
	}
}

// TestStore_StartTurn_UnknownSessionIsTypedNotFound proves StartTurn against
// an unknown SessionID reports *NotFoundError and creates no turn.
func TestStore_StartTurn_UnknownSessionIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

	_, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       "does-not-exist",
		ExpectedVersion: 1,
	})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("StartTurn unknown session: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Session" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Session ID=does-not-exist", notFound)
	}
}

// TestStore_StartTurn_InvalidRequestIdentityCreatesNoMutation proves an
// invalid RequestID reports the existing typed validation classification and
// leaves the session and turn state untouched.
func TestStore_StartTurn_InvalidRequestIdentityCreatesNoMutation(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	_, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       chatsessions.RequestIdentity{},
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if !errors.Is(err, chatsessions.ErrUnknownEnumValue) {
		t.Fatalf("StartTurn invalid request identity: got %v, want ErrUnknownEnumValue", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session != session {
		t.Fatalf("invalid StartTurn mutated Session: got %+v, want %+v", record.session, session)
	}
	if len(record.turns) != 0 {
		t.Fatalf("invalid StartTurn created %d turns, want 0", len(record.turns))
	}
}

// TestStore_StartTurn_GeneratedInvalidTurnLeavesSessionUnchanged proves an
// invalid injected ID is rejected before turn admission mutates the session.
func TestStore_StartTurn_GeneratedInvalidTurnLeavesSessionUnchanged(t *testing.T) {
	ctx := context.Background()
	ids := []string{"session-valid", ""}
	store := NewStore(func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}, fixedClock(time.Now()), nil, nil)
	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	_, err = store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID: startTurnRequestID("invalid-generated-turn"), SessionID: created.Session.ID, ExpectedVersion: created.Session.Version,
	})
	if !errors.Is(err, chatsessions.ErrRequiredValue) {
		t.Fatalf("StartTurn with invalid generated ID: got %v, want ErrRequiredValue", err)
	}
	current, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: created.Session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if current.Session != created.Session || current.MostRecentTurnID != "" {
		t.Fatalf("invalid generated turn changed session: got %#v, want %#v", current, created)
	}
}

// TestStore_StartTurn_SecondTurnAfterTerminalStaysActive proves a session
// already ACTIVE (its first turn already terminated) admits a second turn
// without an illegal ACTIVE->ACTIVE self-transition.
func TestStore_StartTurn_SecondTurnAfterTerminalStaysActive(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}
	advanced, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    first.Turn.ID,
		Next:      chatsessions.TurnStateCanceled,
	})
	if err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}
	if advanced.Turn.State != chatsessions.TurnStateCanceled {
		t.Fatalf("Turn.State = %v, want CANCELED", advanced.Turn.State)
	}

	released, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	second, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-2"),
		SessionID:       session.ID,
		ExpectedVersion: released.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn second: %v", err)
	}
	if second.Session.State != chatsessions.SessionStateActive {
		t.Fatalf("Session.State = %v, want ACTIVE", second.Session.State)
	}
	if second.Turn.ID == first.Turn.ID {
		t.Fatal("second StartTurn reused the first turn's ID")
	}

	store.mu.RLock()
	firstAfter := store.sessions[session.ID].turns[first.Turn.ID]
	store.mu.RUnlock()
	if firstAfter != advanced.Turn {
		t.Fatalf("first turn rewritten by second admission: got %+v, want %+v", firstAfter, advanced.Turn)
	}
}

// TestStore_StartTurn_CommittedControlFencesReplacementUntilResolved proves
// that the captured-control fence survives the captured turn's terminal
// transition. A would-be successor remains rejected until the committed
// intent resolves, so it cannot become a target for an older control.
func TestStore_StartTurn_CommittedControlFencesReplacementUntilResolved(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}
	requested, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID:       controlRequestID("conn-1", "req-control-1"),
		SessionID:       session.ID,
		ExpectedVersion: first.Session.Version,
		Action:          chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl: %v", err)
	}
	if _, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: requested.Intent.RequestID, Next: chatsessions.ControlIntentStateCommitted,
	}); err != nil {
		t.Fatalf("AdvanceControl REQUESTED->COMMITTED: %v", err)
	}
	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: first.Turn.ID, Next: chatsessions.TurnStateCanceled,
	}); err != nil {
		t.Fatalf("AdvanceTurn to CANCELED: %v", err)
	}

	released, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after terminal turn: %v", err)
	}
	_, err = store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-2"),
		SessionID:       session.ID,
		ExpectedVersion: released.Session.Version,
	})
	var busy *chatsessions.BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("StartTurn while control is committed: got %v, want *BusyError", err)
	}
	if busy.ActiveTurnID != first.Turn.ID || busy.ActiveTurnState != chatsessions.TurnStateCanceled {
		t.Fatalf("BusyError = %+v, want captured terminal turn %q CANCELED", busy, first.Turn.ID)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.turns) != 1 || record.session != released.Session || record.session.ActiveTurnID != "" {
		t.Fatalf("fenced StartTurn mutated state: session=%+v turns=%d, want unchanged released session and one turn", record.session, len(record.turns))
	}

	resolved, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: requested.Intent.RequestID, Next: chatsessions.ControlIntentStateCompleted,
	})
	if err != nil {
		t.Fatalf("AdvanceControl COMMITTED->outcome: %v", err)
	}
	if resolved.Intent.State != chatsessions.ControlIntentStateNoop {
		t.Fatalf("resolved intent state = %s, want NOOP", resolved.Intent.State)
	}
	second, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-2"),
		SessionID:       session.ID,
		ExpectedVersion: released.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn after control resolution: %v", err)
	}
	if second.Turn.ID == first.Turn.ID {
		t.Fatal("StartTurn after control resolution reused the captured turn")
	}
}

// TestStore_StartTurn_RedeliveredRequestIDWhileBusyReturnsSameTurn proves a
// StartTurn call reusing a RequestID that already admitted a still-busy turn
// returns that exact turn unchanged instead of reporting *BusyError or
// admitting a second turn -- the caller (not the Store) decides how to react
// to a non-ADMITTED returned Turn.State.
func TestStore_StartTurn_RedeliveredRequestIDWhileBusyReturnsSameTurn(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())
	reqID := startTurnRequestID("req-turn-1")

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       reqID,
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}

	replay, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       reqID,
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn replay: %v", err)
	}
	if replay.Turn.ID != first.Turn.ID {
		t.Fatalf("replay Turn.ID = %q, want the original turn %q", replay.Turn.ID, first.Turn.ID)
	}
	if replay.Turn.State != chatsessions.TurnStateAdmitted {
		t.Fatalf("replay Turn.State = %v, want ADMITTED (unchanged)", replay.Turn.State)
	}

	store.mu.RLock()
	turnCount := len(store.sessions[session.ID].turns)
	store.mu.RUnlock()
	if turnCount != 1 {
		t.Fatalf("stored turn count = %d, want exactly 1 (no second turn admitted)", turnCount)
	}
}

// TestStore_StartTurn_RedeliveredRequestIDAfterTerminalReturnsSameTurn proves
// that once the originally admitted turn for a RequestID has terminalized
// (releasing ActiveTurnID and the session's busy state), a StartTurn call
// reusing that exact RequestID still returns the original, now-terminal turn
// rather than admitting a fresh ADMITTED turn and letting a caller dispatch a
// second Factory effect for content already executed once.
func TestStore_StartTurn_RedeliveredRequestIDAfterTerminalReturnsSameTurn(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())
	reqID := startTurnRequestID("req-turn-1")

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       reqID,
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}
	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    first.Turn.ID,
		Next:      chatsessions.TurnStateRunning,
	}); err != nil {
		t.Fatalf("AdvanceTurn to RUNNING: %v", err)
	}
	advanced, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    first.Turn.ID,
		Next:      chatsessions.TurnStateCompleted,
	})
	if err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}

	replay, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID: reqID,
		SessionID: session.ID,
		// A version stale relative to the released session would fail
		// *ConflictError for a genuinely new admission; the replay must
		// short-circuit before that check is ever reached.
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn replay after terminal: %v", err)
	}
	if replay.Turn.ID != first.Turn.ID {
		t.Fatalf("replay Turn.ID = %q, want the original terminal turn %q", replay.Turn.ID, first.Turn.ID)
	}
	if replay.Turn.State != chatsessions.TurnStateCompleted {
		t.Fatalf("replay Turn.State = %v, want COMPLETED (the original terminal state)", replay.Turn.State)
	}
	if replay.Turn != advanced.Turn {
		t.Fatalf("replay Turn = %+v, want the exact original terminal turn %+v", replay.Turn, advanced.Turn)
	}

	store.mu.RLock()
	turnCount := len(store.sessions[session.ID].turns)
	store.mu.RUnlock()
	if turnCount != 1 {
		t.Fatalf("stored turn count = %d, want exactly 1 (no second turn admitted by the replay)", turnCount)
	}
}

// TestStore_StartTurn_DistinctRequestIDAfterTerminalAdmitsNewTurn proves the
// redelivery guard is scoped to an exact RequestID: a genuinely distinct
// later request against the same session, submitted after the original turn
// terminalized, still admits its own new turn.
func TestStore_StartTurn_DistinctRequestIDAfterTerminalAdmitsNewTurn(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}
	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    first.Turn.ID,
		Next:      chatsessions.TurnStateRunning,
	}); err != nil {
		t.Fatalf("AdvanceTurn to RUNNING: %v", err)
	}
	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    first.Turn.ID,
		Next:      chatsessions.TurnStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}
	released, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	second, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-2"),
		SessionID:       session.ID,
		ExpectedVersion: released.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn second: %v", err)
	}
	if second.Turn.ID == first.Turn.ID {
		t.Fatal("distinct RequestID reused the first turn's ID")
	}
	if second.Turn.State != chatsessions.TurnStateAdmitted {
		t.Fatalf("second Turn.State = %v, want ADMITTED", second.Turn.State)
	}
}

// TestStore_StartTurn_ConcurrentAdmissionSingleWinner proves concurrent
// admissions using the same ExpectedVersion serialize to exactly one
// successful commit. Since StartTurn checks ExpectedVersion before the busy
// precondition (matching SetTarget's ordering) and the winner's commit
// itself advances Session.Version, every loser observes the committed state
// as a typed *ConflictError, not *BusyError.
func TestStore_StartTurn_ConcurrentAdmissionSingleWinner(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	const n = 25
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
				RequestID:       startTurnRequestID(fmt.Sprintf("req-turn-%d", i)),
				SessionID:       session.ID,
				ExpectedVersion: session.Version,
			})
			errs[i] = err
		}(i)
	}
	wg.Wait()

	successes, conflicts := 0, 0
	for _, err := range errs {
		var conflict *chatsessions.ConflictError
		switch {
		case err == nil:
			successes++
		case errors.As(err, &conflict):
			conflicts++
		default:
			t.Fatalf("unexpected StartTurn error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1", successes)
	}
	if conflicts != n-1 {
		t.Fatalf("conflicts = %d, want %d", conflicts, n-1)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.turns) != 1 {
		t.Fatalf("final turn count = %d, want 1", len(record.turns))
	}
}

// TestStore_StartTurn_ConcurrentAdmissionWhileBusyAllBusy proves concurrent
// admission attempts against an already-active turn, each supplying the
// session's current (matching) version, all observe typed *BusyError and
// create no additional active turn -- the direct concurrent counterpart of
// AC4's "second sequential or concurrent admission ... returns typed
// BusyError".
func TestStore_StartTurn_ConcurrentAdmissionWhileBusyAllBusy(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}

	const n = 25
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
				RequestID:       startTurnRequestID(fmt.Sprintf("req-turn-busy-%d", i)),
				SessionID:       session.ID,
				ExpectedVersion: first.Session.Version,
			})
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		var busy *chatsessions.BusyError
		if !errors.As(err, &busy) {
			t.Fatalf("StartTurn[%d] while busy: got %v, want *BusyError", i, err)
		}
		if busy.ActiveTurnID != first.Turn.ID {
			t.Fatalf("BusyError[%d].ActiveTurnID = %q, want %q", i, busy.ActiveTurnID, first.Turn.ID)
		}
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.turns) != 1 {
		t.Fatalf("final turn count = %d, want 1", len(record.turns))
	}
	if record.session != first.Session {
		t.Fatalf("session changed after concurrent busy admissions: got %+v, want %+v", record.session, first.Session)
	}
}

// TestStore_AdvanceTurn_LegalTransitions proves every legal TurnState
// transition succeeds and, when terminal, releases the session for a later
// turn without rewriting the earlier turn.
func TestStore_AdvanceTurn_LegalTransitions(t *testing.T) {
	for _, tt := range []struct {
		name string
		path []chatsessions.TurnState
	}{
		{"admitted to running to completed", []chatsessions.TurnState{chatsessions.TurnStateRunning, chatsessions.TurnStateCompleted}},
		{"admitted to running to failed", []chatsessions.TurnState{chatsessions.TurnStateRunning, chatsessions.TurnStateFailed}},
		{"admitted to running to canceled", []chatsessions.TurnState{chatsessions.TurnStateRunning, chatsessions.TurnStateCanceled}},
		{"admitted directly to canceled", []chatsessions.TurnState{chatsessions.TurnStateCanceled}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store, session := newStartTurnTestSession(t, time.Now())
			started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
				RequestID:       startTurnRequestID("req-turn-1"),
				SessionID:       session.ID,
				ExpectedVersion: session.Version,
			})
			if err != nil {
				t.Fatalf("StartTurn: %v", err)
			}

			var last chatsessions.Turn
			for _, next := range tt.path {
				result, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
					SessionID: session.ID,
					TurnID:    started.Turn.ID,
					Next:      next,
				})
				if err != nil {
					t.Fatalf("AdvanceTurn to %s: %v", next, err)
				}
				last = result.Turn
			}
			if last.State != tt.path[len(tt.path)-1] {
				t.Fatalf("final Turn.State = %v, want %v", last.State, tt.path[len(tt.path)-1])
			}
			if err := last.Validate(); err != nil {
				t.Fatalf("terminal Turn fails its own Validate: %v", err)
			}

			released, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if released.Session.ActiveTurnID != "" {
				t.Fatalf("Session.ActiveTurnID = %q after terminal advancement, want blank", released.Session.ActiveTurnID)
			}
			if released.Session.Version <= started.Session.Version {
				t.Fatalf("Session.Version = %d, want strictly greater than %d", released.Session.Version, started.Session.Version)
			}
		})
	}
}

// TestStore_AdvanceTurn_IllegalTransitionLeavesStateUnchanged proves a
// representative set of illegal transitions report typed *TransitionError
// and leave the stored turn unchanged.
func TestStore_AdvanceTurn_IllegalTransitionLeavesStateUnchanged(t *testing.T) {
	for _, tt := range []struct {
		name string
		next chatsessions.TurnState
	}{
		{"admitted to completed", chatsessions.TurnStateCompleted},
		{"admitted to failed", chatsessions.TurnStateFailed},
		{"admitted to self", chatsessions.TurnStateAdmitted},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store, session := newStartTurnTestSession(t, time.Now())
			started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
				RequestID:       startTurnRequestID("req-turn-1"),
				SessionID:       session.ID,
				ExpectedVersion: session.Version,
			})
			if err != nil {
				t.Fatalf("StartTurn: %v", err)
			}

			_, err = store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
				SessionID: session.ID,
				TurnID:    started.Turn.ID,
				Next:      tt.next,
			})
			var transition *chatsessions.TransitionError
			if !errors.As(err, &transition) {
				t.Fatalf("AdvanceTurn(%s): got %v, want *TransitionError", tt.name, err)
			}

			store.mu.RLock()
			after := store.sessions[session.ID]
			store.mu.RUnlock()
			if got := after.turns[started.Turn.ID]; got != started.Turn {
				t.Fatalf("AdvanceTurn(%s) mutated turn: got %+v, want %+v", tt.name, got, started.Turn)
			}
			if after.session != started.Session {
				t.Fatalf("AdvanceTurn(%s) mutated session: got %+v, want %+v", tt.name, after.session, started.Session)
			}
		})
	}
}

// TestStore_AdvanceTurn_TerminalTurnRejectsFurtherAdvancement proves
// advancing an already-terminal turn again reports typed *TransitionError
// and leaves the turn unchanged, so it can never rewrite its own history.
func TestStore_AdvanceTurn_TerminalTurnRejectsFurtherAdvancement(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())
	started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	terminal, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    started.Turn.ID,
		Next:      chatsessions.TurnStateCanceled,
	})
	if err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}

	_, err = store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    started.Turn.ID,
		Next:      chatsessions.TurnStateRunning,
	})
	var transition *chatsessions.TransitionError
	if !errors.As(err, &transition) {
		t.Fatalf("AdvanceTurn after terminal: got %v, want *TransitionError", err)
	}

	store.mu.RLock()
	after := store.sessions[session.ID].turns[started.Turn.ID]
	store.mu.RUnlock()
	if after != terminal.Turn {
		t.Fatalf("terminal turn rewritten: got %+v, want %+v", after, terminal.Turn)
	}
}

// TestStore_AdvanceTurn_UnknownTurnIsTypedNotFound proves AdvanceTurn against
// an unknown TurnID (on a known or unknown session) reports typed
// *NotFoundError naming the Turn, not the Session.
func TestStore_AdvanceTurn_UnknownTurnIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	_, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    "does-not-exist",
		Next:      chatsessions.TurnStateRunning,
	})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("AdvanceTurn unknown turn: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Turn" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Turn ID=does-not-exist", notFound)
	}

	_, err = store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: "does-not-exist-session",
		TurnID:    "does-not-exist-turn",
		Next:      chatsessions.TurnStateRunning,
	})
	if !errors.As(err, &notFound) {
		t.Fatalf("AdvanceTurn unknown session: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Turn" || notFound.ID != "does-not-exist-turn" {
		t.Fatalf("NotFoundError = %+v, want Value=Turn ID=does-not-exist-turn", notFound)
	}
}

// TestStore_AdvanceTurn_DistinctTerminalSequences proves two different
// terminal turns in the same session never collide on TerminalSequence.
func TestStore_AdvanceTurn_DistinctTerminalSequences(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	first, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}
	firstTerminal, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    first.Turn.ID,
		Next:      chatsessions.TurnStateCanceled,
	})
	if err != nil {
		t.Fatalf("AdvanceTurn first to terminal: %v", err)
	}

	released, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	second, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-2"),
		SessionID:       session.ID,
		ExpectedVersion: released.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn second: %v", err)
	}
	secondTerminal, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    second.Turn.ID,
		Next:      chatsessions.TurnStateCanceled,
	})
	if err != nil {
		t.Fatalf("AdvanceTurn second to terminal: %v", err)
	}

	if firstTerminal.Turn.TerminalSequence == 0 || secondTerminal.Turn.TerminalSequence == 0 {
		t.Fatalf("terminal sequences must be non-zero: first=%d second=%d",
			firstTerminal.Turn.TerminalSequence, secondTerminal.Turn.TerminalSequence)
	}
	if firstTerminal.Turn.TerminalSequence == secondTerminal.Turn.TerminalSequence {
		t.Fatalf("two terminal turns collided on TerminalSequence %d", firstTerminal.Turn.TerminalSequence)
	}
}

// TestStore_AdvanceTurn_ConcurrentAdvancementIsRaceFree proves concurrent
// AdvanceTurn calls against the same turn serialize so exactly one legal
// ADMITTED->RUNNING transition wins and every other caller observes a typed
// *TransitionError, never a corrupted or partially applied turn.
func TestStore_AdvanceTurn_ConcurrentAdvancementIsRaceFree(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())
	started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	const n = 25
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
				SessionID: session.ID,
				TurnID:    started.Turn.ID,
				Next:      chatsessions.TurnStateRunning,
			})
			errs[i] = err
		}(i)
	}
	wg.Wait()

	successes, transitions := 0, 0
	for _, err := range errs {
		var transition *chatsessions.TransitionError
		switch {
		case err == nil:
			successes++
		case errors.As(err, &transition):
			transitions++
		default:
			t.Fatalf("unexpected AdvanceTurn error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1", successes)
	}
	if transitions != n-1 {
		t.Fatalf("transitions = %d, want %d", transitions, n-1)
	}

	store.mu.RLock()
	final := store.sessions[session.ID].turns[started.Turn.ID]
	store.mu.RUnlock()
	if final.State != chatsessions.TurnStateRunning {
		t.Fatalf("final Turn.State = %v, want RUNNING", final.State)
	}
}
