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

func otherTarget() chatsessions.ChatTargetRef {
	return chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/other"}
}

func setTargetRequestID(id string) chatsessions.RequestIdentity {
	return chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: id}
}

// newSetTargetTestSession constructs a Store and one created Session ready
// for SetTarget calls.
func newSetTargetTestSession(t *testing.T, now time.Time) (*Store, chatsessions.Session) {
	t.Helper()
	store := NewStore(sequentialIDs("session"), fixedClock(now), nil, nil)
	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return store, created.Session
}

// TestStore_SetTarget_RolloverClosesAndOpensEpisode proves a target change
// with the current version atomically closes the prior episode, opens the
// next consecutively numbered episode at the new target, updates session
// selection/episode number, and returns a strictly newer version.
func TestStore_SetTarget_RolloverClosesAndOpensEpisode(t *testing.T) {
	ctx := context.Background()
	created := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	rollover := created.Add(time.Minute)
	store, session := newSetTargetTestSession(t, created)
	store.now = fixedClock(rollover)

	result, err := store.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       setTargetRequestID("req-target"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
		Target:          otherTarget(),
	})
	if err != nil {
		t.Fatalf("SetTarget: %v", err)
	}
	if result.Session.SelectedTarget != otherTarget() {
		t.Fatalf("SelectedTarget = %+v, want %+v", result.Session.SelectedTarget, otherTarget())
	}
	if result.Session.TargetEpisode != 2 {
		t.Fatalf("TargetEpisode = %d, want 2", result.Session.TargetEpisode)
	}
	if result.Session.Version <= session.Version {
		t.Fatalf("Version = %d, want strictly greater than %d", result.Session.Version, session.Version)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.episodes) != 2 {
		t.Fatalf("got %d episodes, want 2", len(record.episodes))
	}
	prior := record.episodes[0]
	if prior.Number != 1 || prior.State != chatsessions.TargetEpisodeStateClosed {
		t.Fatalf("prior episode = %+v, want Number=1 State=CLOSED", prior)
	}
	if prior.Target.Ref != "factory:@you/review" {
		t.Fatalf("prior episode Target = %+v, want original initial target", prior.Target)
	}
	if prior.ClosedAt == nil || !prior.ClosedAt.Equal(rollover) {
		t.Fatalf("prior episode ClosedAt = %v, want %v", prior.ClosedAt, rollover)
	}
	next := record.episodes[1]
	if next.Number != 2 || next.State != chatsessions.TargetEpisodeStateOpen {
		t.Fatalf("next episode = %+v, want Number=2 State=OPEN", next)
	}
	if next.Target != otherTarget() {
		t.Fatalf("next episode Target = %+v, want %+v", next.Target, otherTarget())
	}
	if next.ClosedAt != nil {
		t.Fatalf("next episode ClosedAt = %v, want nil", next.ClosedAt)
	}
}

// TestStore_SetTarget_PreviouslyReturnedValuesUnaffected proves a Session
// value read before a rollover is not mutated by the rollover, and a
// previously observed closed episode's fields are never rewritten by a
// subsequent rollover.
func TestStore_SetTarget_PreviouslyReturnedValuesUnaffected(t *testing.T) {
	ctx := context.Background()
	store, session := newSetTargetTestSession(t, time.Now())
	before := session

	if _, err := store.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       setTargetRequestID("req-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
		Target:          otherTarget(),
	}); err != nil {
		t.Fatalf("SetTarget first: %v", err)
	}
	if before.TargetEpisode != 1 || before.SelectedTarget.Ref != "factory:@you/review" {
		t.Fatalf("previously returned Session was mutated: %+v", before)
	}

	store.mu.RLock()
	firstClosed := store.sessions[session.ID].episodes[0]
	store.mu.RUnlock()

	second, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if _, err := store.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       setTargetRequestID("req-2"),
		SessionID:       session.ID,
		ExpectedVersion: second.Session.Version,
		Target:          chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/third"},
	}); err != nil {
		t.Fatalf("SetTarget second: %v", err)
	}

	store.mu.RLock()
	firstClosedAfter := store.sessions[session.ID].episodes[0]
	store.mu.RUnlock()
	if firstClosedAfter != firstClosed {
		t.Fatalf("historical episode 1 was rewritten: got %+v, want %+v", firstClosedAfter, firstClosed)
	}
}

// TestStore_SetTarget_StaleVersionConflictLeavesStateUnchanged proves a
// stale ExpectedVersion reports *ConflictError with the expected/actual
// facts and leaves selection, episode history, and version unchanged.
func TestStore_SetTarget_StaleVersionConflictLeavesStateUnchanged(t *testing.T) {
	ctx := context.Background()
	store, session := newSetTargetTestSession(t, time.Now())

	_, err := store.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       setTargetRequestID("req-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version + 1,
		Target:          otherTarget(),
	})
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("SetTarget stale version: got %v, want *ConflictError", err)
	}
	if conflict.Expected != session.Version+1 || conflict.Actual != session.Version {
		t.Fatalf("ConflictError = %+v, want Expected=%d Actual=%d", conflict, session.Version+1, session.Version)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if record.session != session {
		t.Fatalf("stale SetTarget mutated Session: got %+v, want %+v", record.session, session)
	}
	if len(record.episodes) != 1 {
		t.Fatalf("stale SetTarget created %d episodes, want 1", len(record.episodes))
	}
}

// TestStore_SetTarget_BusyWhileTurnActive proves a target change while a
// turn is non-terminal reports *BusyError, creates no episode, and leaves
// the session unchanged. The turn is admitted and advanced through the
// public StartTurn/AdvanceTurn API (not fabricated private state), and
// advanced to RUNNING before the busy check: an earlier revision kept a
// second, independently-mutable copy of the active turn that AdvanceTurn's
// non-terminal path never refreshed, so a BusyError raised after a public
// ADMITTED->RUNNING advancement reported the stale ADMITTED state instead of
// the turn's live RUNNING state.
func TestStore_SetTarget_BusyWhileTurnActive(t *testing.T) {
	ctx := context.Background()
	store, session := newSetTargetTestSession(t, time.Now())

	started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       setTargetRequestID("req-turn"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	advanced, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID,
		TurnID:    started.Turn.ID,
		Next:      chatsessions.TurnStateRunning,
	})
	if err != nil {
		t.Fatalf("AdvanceTurn to RUNNING: %v", err)
	}

	_, err = store.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       setTargetRequestID("req-1"),
		SessionID:       session.ID,
		ExpectedVersion: started.Session.Version,
		Target:          otherTarget(),
	})
	var busy *chatsessions.BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("SetTarget while active turn: got %v, want *BusyError", err)
	}
	if busy.ActiveTurnID != started.Turn.ID || busy.ActiveTurnState != chatsessions.TurnStateRunning {
		t.Fatalf("BusyError = %+v, want ActiveTurnID=%q ActiveTurnState=RUNNING", busy, started.Turn.ID)
	}
	if advanced.Turn.State != chatsessions.TurnStateRunning {
		t.Fatalf("AdvanceTurn result state = %v, want RUNNING", advanced.Turn.State)
	}

	store.mu.RLock()
	after := store.sessions[session.ID]
	store.mu.RUnlock()
	if after.session != started.Session {
		t.Fatalf("busy SetTarget mutated Session: got %+v, want %+v", after.session, started.Session)
	}
	if len(after.episodes) != 1 {
		t.Fatalf("busy SetTarget created %d episodes, want 1", len(after.episodes))
	}
}

// TestStore_SetTarget_BusyWhileCommittedCloseAwaitsFanout proves a terminal
// captured turn does not reopen target admission while its committed CLOSE
// still owns the episode. The close can therefore complete atomically after
// the downstream Factory effect rather than discovering a replaced episode
// and leaving a committed intent behind.
func TestStore_SetTarget_BusyWhileCommittedCloseAwaitsFanout(t *testing.T) {
	ctx := context.Background()
	store, session, turn := newActiveTurnTestSession(t, time.Now())
	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateRunning,
	}); err != nil {
		t.Fatalf("AdvanceTurn(RUNNING): %v", err)
	}
	requestID := controlRequestID("conn-target", "close-fence")
	if _, err := store.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: requestID, SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionClose,
	}); err != nil {
		t.Fatalf("RequestControl(CLOSE): %v", err)
	}
	if _, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: requestID, Next: chatsessions.ControlIntentStateCommitted,
	}); err != nil {
		t.Fatalf("AdvanceControl(COMMITTED): %v", err)
	}
	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceTurn(COMPLETED): %v", err)
	}
	released, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after terminal turn: %v", err)
	}
	if _, err := store.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID: setTargetRequestID("close-fence"), SessionID: session.ID,
		ExpectedVersion: released.Session.Version, Target: otherTarget(),
	}); !errors.Is(err, chatsessions.ErrBusy) {
		t.Fatalf("SetTarget while CLOSE is committed: got %v, want ErrBusy", err)
	}
	if _, err := store.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: requestID, Next: chatsessions.ControlIntentStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceControl(COMPLETED): %v", err)
	}
	closed, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after close: %v", err)
	}
	if closed.Session.State != chatsessions.SessionStateClosed || closed.Episode.Number != session.TargetEpisode || closed.Episode.State != chatsessions.TargetEpisodeStateClosed {
		t.Fatalf("committed close did not atomically close its original lifecycle: Session=%#v Episode=%#v", closed.Session, closed.Episode)
	}
}

// TestStore_SetTarget_UnknownSessionIsTypedNotFound proves SetTarget against
// an unknown SessionID reports *NotFoundError and mutates nothing.
func TestStore_SetTarget_UnknownSessionIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()), nil, nil)

	_, err := store.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       setTargetRequestID("req-1"),
		SessionID:       "does-not-exist",
		ExpectedVersion: 1,
		Target:          otherTarget(),
	})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("SetTarget unknown session: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Session" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Session ID=does-not-exist", notFound)
	}
}

// TestStore_SetTarget_InvalidInputCreatesNoMutation proves an invalid
// RequestID or Target reports the existing typed validation classification
// and leaves the session and episode history untouched.
func TestStore_SetTarget_InvalidInputCreatesNoMutation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mutate  func(chatsessions.SetTargetRequest) chatsessions.SetTargetRequest
		wantErr error
	}{
		{"invalid request identity", func(r chatsessions.SetTargetRequest) chatsessions.SetTargetRequest {
			r.RequestID = chatsessions.RequestIdentity{}
			return r
		}, chatsessions.ErrUnknownEnumValue},
		{"invalid target", func(r chatsessions.SetTargetRequest) chatsessions.SetTargetRequest {
			r.Target = chatsessions.ChatTargetRef{}
			return r
		}, chatsessions.ErrUnknownEnumValue},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store, session := newSetTargetTestSession(t, time.Now())

			req := chatsessions.SetTargetRequest{
				RequestID:       setTargetRequestID("req-1"),
				SessionID:       session.ID,
				ExpectedVersion: session.Version,
				Target:          otherTarget(),
			}
			_, err := store.SetTarget(ctx, tt.mutate(req))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SetTarget(%s): got %v, want %v", tt.name, err, tt.wantErr)
			}

			store.mu.RLock()
			record := store.sessions[session.ID]
			store.mu.RUnlock()
			if record.session != session {
				t.Fatalf("SetTarget(%s) mutated Session: got %+v, want %+v", tt.name, record.session, session)
			}
			if len(record.episodes) != 1 {
				t.Fatalf("SetTarget(%s) created %d episodes, want 1", tt.name, len(record.episodes))
			}
		})
	}
}

// TestStore_SetTarget_ConcurrentSameExpectedVersionSingleWinner proves
// concurrent target changes issued with the same ExpectedVersion yield
// exactly one successful commit; every other caller observes the committed
// state as a typed conflict, and the final episode history has no skipped
// or duplicate episode number.
func TestStore_SetTarget_ConcurrentSameExpectedVersionSingleWinner(t *testing.T) {
	ctx := context.Background()
	store, session := newSetTargetTestSession(t, time.Now())

	const n = 25
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.SetTarget(ctx, chatsessions.SetTargetRequest{
				RequestID:       setTargetRequestID(fmt.Sprintf("req-%d", i)),
				SessionID:       session.ID,
				ExpectedVersion: session.Version,
				Target:          otherTarget(),
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
			t.Fatalf("unexpected SetTarget error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1", successes)
	}
	if conflicts != n-1 {
		t.Fatalf("conflicts = %d, want %d", conflicts, n-1)
	}

	final, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if final.Session.Version != session.Version+1 {
		t.Fatalf("final Version = %d, want %d", final.Session.Version, session.Version+1)
	}
	if final.Session.TargetEpisode != 2 {
		t.Fatalf("final TargetEpisode = %d, want 2", final.Session.TargetEpisode)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.episodes) != 2 {
		t.Fatalf("final episode count = %d, want 2", len(record.episodes))
	}
	for idx, ep := range record.episodes {
		if ep.Number != uint64(idx+1) {
			t.Fatalf("episodes[%d].Number = %d, want %d", idx, ep.Number, idx+1)
		}
	}
}
