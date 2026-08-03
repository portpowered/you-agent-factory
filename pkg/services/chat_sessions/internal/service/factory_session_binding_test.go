package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// startedTurnForBinding constructs a Store, one created Session, and one
// admitted turn ready for BindFactorySession calls, returning the exact
// request a caller would build from that admission's StartTurnResult.
func startedTurnForBinding(t *testing.T, now time.Time) (*Store, chatsessions.StartTurnResult) {
	t.Helper()
	store, session := newStartTurnTestSession(t, now)
	started, err := store.StartTurn(context.Background(), chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	return store, started
}

func bindRequest(started chatsessions.StartTurnResult, factorySessionID string) chatsessions.BindFactorySessionRequest {
	return chatsessions.BindFactorySessionRequest{
		SessionID:        started.Session.ID,
		ExpectedVersion:  started.Session.Version,
		Episode:          started.Episode.Number,
		TurnID:           started.Turn.ID,
		FactorySessionID: factorySessionID,
	}
}

// TestStore_BindFactorySession_CommitsOntoCurrentEpisode proves a valid bind
// commits FactorySessionID onto exactly the current episode, advances
// Session.Version, and leaves no other episode present.
func TestStore_BindFactorySession_CommitsOntoCurrentEpisode(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	result, err := store.BindFactorySession(ctx, bindRequest(started, "factory-session-1"))
	if err != nil {
		t.Fatalf("BindFactorySession: %v", err)
	}
	if result.Session.Version <= started.Session.Version {
		t.Fatalf("Session.Version = %d, want strictly greater than %d", result.Session.Version, started.Session.Version)
	}

	store.mu.RLock()
	record := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if len(record.episodes) != 1 {
		t.Fatalf("episode count = %d, want 1", len(record.episodes))
	}
	if record.episodes[0].FactorySessionID != "factory-session-1" {
		t.Fatalf("episode FactorySessionID = %q, want factory-session-1", record.episodes[0].FactorySessionID)
	}
}

// TestStore_BindFactorySession_IdempotentSameIdentityConverges proves a
// repeat bind carrying the exact identity the episode already carries
// succeeds without a second mutation, regardless of a now-stale
// ExpectedVersion -- the mechanism that lets concurrent or retried binding
// attempts for the identity that already won converge on one committed
// value.
func TestStore_BindFactorySession_IdempotentSameIdentityConverges(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	first, err := store.BindFactorySession(ctx, bindRequest(started, "factory-session-1"))
	if err != nil {
		t.Fatalf("BindFactorySession first: %v", err)
	}

	// Repeat with the original (now-stale) ExpectedVersion: idempotent
	// convergence must not require a fresh read.
	second, err := store.BindFactorySession(ctx, bindRequest(started, "factory-session-1"))
	if err != nil {
		t.Fatalf("BindFactorySession repeat: %v", err)
	}
	if second.Session.Version != first.Session.Version {
		t.Fatalf("repeat bind mutated version: got %d, want %d (no-op)", second.Session.Version, first.Session.Version)
	}
}

// TestStore_BindFactorySession_ConflictingIdentityNeverOverwrites proves a
// bind attempt carrying a *different* identity than the one already
// committed reports *FactorySessionConflictError and never overwrites the
// committed value.
func TestStore_BindFactorySession_ConflictingIdentityNeverOverwrites(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	if _, err := store.BindFactorySession(ctx, bindRequest(started, "factory-session-1")); err != nil {
		t.Fatalf("BindFactorySession first: %v", err)
	}

	_, err := store.BindFactorySession(ctx, bindRequest(started, "factory-session-2"))
	var conflict *chatsessions.FactorySessionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("BindFactorySession conflicting identity: got %v, want *FactorySessionConflictError", err)
	}
	if conflict.Bound != "factory-session-1" || conflict.Attempted != "factory-session-2" {
		t.Fatalf("FactorySessionConflictError = %+v, want Bound=factory-session-1 Attempted=factory-session-2", conflict)
	}

	store.mu.RLock()
	record := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if record.episodes[0].FactorySessionID != "factory-session-1" {
		t.Fatalf("episode FactorySessionID = %q after conflicting bind, want unchanged factory-session-1", record.episodes[0].FactorySessionID)
	}
}

// TestStore_BindFactorySession_StaleVersionIsTypedConflict proves a bind
// whose ExpectedVersion no longer matches (and whose episode is not already
// bound to the same identity) reports *ConflictError and mutates nothing.
func TestStore_BindFactorySession_StaleVersionIsTypedConflict(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	req := bindRequest(started, "factory-session-1")
	req.ExpectedVersion = started.Session.Version + 1

	_, err := store.BindFactorySession(ctx, req)
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("BindFactorySession stale version: got %v, want *ConflictError", err)
	}

	store.mu.RLock()
	record := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if record.episodes[0].FactorySessionID != "" {
		t.Fatalf("episode FactorySessionID = %q after stale-version bind, want blank", record.episodes[0].FactorySessionID)
	}
	if record.session.Version != started.Session.Version {
		t.Fatalf("session mutated by stale-version bind: version = %d, want %d", record.session.Version, started.Session.Version)
	}
}

// TestStore_BindFactorySession_TurnNoLongerActiveIsTypedConflict proves a
// bind whose TurnID is no longer the session's active turn (the turn already
// terminated) reports *ConflictError, even though the caller may not yet
// know the session moved on.
func TestStore_BindFactorySession_TurnNoLongerActiveIsTypedConflict(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	terminal, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: started.Session.ID,
		TurnID:    started.Turn.ID,
		Next:      chatsessions.TurnStateCanceled,
	})
	if err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}
	_ = terminal

	_, err = store.BindFactorySession(ctx, bindRequest(started, "factory-session-1"))
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("BindFactorySession after turn terminated: got %v, want *ConflictError", err)
	}
}

// TestStore_BindFactorySession_BlankFactorySessionIDIsValidationError proves
// a blank FactorySessionID is rejected before any mutation.
func TestStore_BindFactorySession_BlankFactorySessionIDIsValidationError(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	_, err := store.BindFactorySession(ctx, bindRequest(started, ""))
	var validation *chatsessions.ValidationError
	if !errors.As(err, &validation) || validation.Field != "FactorySessionID" {
		t.Fatalf("BindFactorySession blank identity: got %v, want *ValidationError on FactorySessionID", err)
	}
}

// TestStore_BindFactorySession_BlankTurnIDIsValidationError proves a blank
// TurnID is rejected before any mutation.
func TestStore_BindFactorySession_BlankTurnIDIsValidationError(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	req := bindRequest(started, "factory-session-1")
	req.TurnID = ""

	_, err := store.BindFactorySession(ctx, req)
	var validation *chatsessions.ValidationError
	if !errors.As(err, &validation) || validation.Field != "TurnID" {
		t.Fatalf("BindFactorySession blank turn id: got %v, want *ValidationError on TurnID", err)
	}
}

// TestStore_BindFactorySession_UnknownSessionIsTypedNotFound proves binding
// against an unknown SessionID reports *NotFoundError.
func TestStore_BindFactorySession_UnknownSessionIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

	_, err := store.BindFactorySession(ctx, chatsessions.BindFactorySessionRequest{
		SessionID: "does-not-exist", ExpectedVersion: 1, Episode: 1, TurnID: "turn-1", FactorySessionID: "factory-session-1",
	})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("BindFactorySession unknown session: got %v, want *NotFoundError", err)
	}
}

// TestStore_BindFactorySession_PriorEpisodeHistoryNeverMutated proves a bind
// against the session's current (second) episode never touches the prior,
// closed episode's stored value.
func TestStore_BindFactorySession_PriorEpisodeHistoryNeverMutated(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())

	newTarget := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/other"}
	changed, err := store.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       startTurnRequestID("req-target-1"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
		Target:          newTarget,
	})
	if err != nil {
		t.Fatalf("SetTarget: %v", err)
	}

	// Capture the prior episode's snapshot after SetTarget closed it --
	// that OPEN->CLOSED transition is SetTarget's own effect, not a mutation
	// this test is guarding against; the guard below is specifically that
	// BindFactorySession itself never touches this already-closed episode.
	store.mu.RLock()
	priorEpisode := store.sessions[session.ID].episodes[0]
	store.mu.RUnlock()
	started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("req-turn-1"),
		SessionID:       session.ID,
		ExpectedVersion: changed.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if started.Episode.Number != 2 {
		t.Fatalf("Episode.Number = %d, want 2 (the new episode SetTarget opened)", started.Episode.Number)
	}

	if _, err := store.BindFactorySession(ctx, bindRequest(started, "factory-session-2")); err != nil {
		t.Fatalf("BindFactorySession: %v", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.episodes) != 2 {
		t.Fatalf("episode count = %d, want 2", len(record.episodes))
	}
	if record.episodes[0] != priorEpisode {
		t.Fatalf("prior episode mutated: got %+v, want unchanged %+v", record.episodes[0], priorEpisode)
	}
	if record.episodes[1].FactorySessionID != "factory-session-2" {
		t.Fatalf("current episode FactorySessionID = %q, want factory-session-2", record.episodes[1].FactorySessionID)
	}
}

// TestStore_BindFactorySession_ConcurrentSameIdentityConverges proves
// concurrent bind attempts carrying the same winning identity all succeed
// and converge on exactly one committed value and one version increment.
func TestStore_BindFactorySession_ConcurrentSameIdentityConverges(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	const n = 25
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.BindFactorySession(ctx, bindRequest(started, "factory-session-1"))
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("BindFactorySession[%d]: got %v, want success (idempotent convergence)", i, err)
		}
	}

	store.mu.RLock()
	record := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if record.episodes[0].FactorySessionID != "factory-session-1" {
		t.Fatalf("episode FactorySessionID = %q, want factory-session-1", record.episodes[0].FactorySessionID)
	}
	if record.session.Version != started.Session.Version+1 {
		t.Fatalf("final version = %d, want exactly one increment past %d", record.session.Version, started.Session.Version)
	}
}

// TestStore_BindFactorySession_ConcurrentConflictingIdentitiesOneWinner
// proves concurrent bind attempts split between two different identities
// converge on exactly one committed identity: every caller for the losing
// identity observes *FactorySessionConflictError and the episode is never
// left carrying a mix of the two.
func TestStore_BindFactorySession_ConcurrentConflictingIdentitiesOneWinner(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "factory-session-a"
			if i%2 == 1 {
				id = "factory-session-b"
			}
			_, err := store.BindFactorySession(ctx, bindRequest(started, id))
			errs[i] = err
		}(i)
	}
	wg.Wait()

	store.mu.RLock()
	winner := store.sessions[started.Session.ID].episodes[0].FactorySessionID
	store.mu.RUnlock()
	if winner != "factory-session-a" && winner != "factory-session-b" {
		t.Fatalf("committed identity = %q, want one of factory-session-a/b", winner)
	}

	for i, err := range errs {
		wantID := "factory-session-a"
		if i%2 == 1 {
			wantID = "factory-session-b"
		}
		if wantID == winner {
			if err != nil {
				t.Fatalf("BindFactorySession[%d] for winning identity %q: got %v, want success", i, wantID, err)
			}
			continue
		}
		var conflict *chatsessions.FactorySessionConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("BindFactorySession[%d] for losing identity %q: got %v, want *FactorySessionConflictError", i, wantID, err)
		}
	}
}
