package service

import (
	"context"
	"errors"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

func pendingRequest(started chatsessions.StartTurnResult, factorySessionID string) chatsessions.RecordPendingFactorySessionRequest {
	return chatsessions.RecordPendingFactorySessionRequest{
		SessionID:        started.Session.ID,
		ExpectedVersion:  started.Session.Version,
		Episode:          started.Episode.Number,
		TurnID:           started.Turn.ID,
		FactorySessionID: factorySessionID,
	}
}

// TestStore_RecordPendingFactorySession_CommitsOntoCurrentEpisode proves a
// valid record sets PendingFactorySessionID on exactly the current episode
// without advancing Session.Version -- this is incidental reconciliation
// bookkeeping, not a state transition.
func TestStore_RecordPendingFactorySession_CommitsOntoCurrentEpisode(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	result, err := store.RecordPendingFactorySession(ctx, pendingRequest(started, "fs-pending-1"))
	if err != nil {
		t.Fatalf("RecordPendingFactorySession: %v", err)
	}
	if result.Session.Version != started.Session.Version {
		t.Fatalf("Session.Version = %d, want unchanged from %d", result.Session.Version, started.Session.Version)
	}

	store.mu.RLock()
	record := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if record.episodes[0].PendingFactorySessionID != "fs-pending-1" {
		t.Fatalf("episode PendingFactorySessionID = %q, want fs-pending-1", record.episodes[0].PendingFactorySessionID)
	}
}

// TestStore_RecordPendingFactorySession_IdempotentSameIdentityConverges
// proves a repeat record carrying the exact value already stored succeeds
// without mutation, regardless of a now-stale ExpectedVersion.
func TestStore_RecordPendingFactorySession_IdempotentSameIdentityConverges(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	if _, err := store.RecordPendingFactorySession(ctx, pendingRequest(started, "fs-pending-1")); err != nil {
		t.Fatalf("RecordPendingFactorySession first: %v", err)
	}

	// Repeat with the original (now-stale, since version tracking would
	// otherwise have moved on) ExpectedVersion: idempotent convergence must
	// not require a fresh read.
	if _, err := store.RecordPendingFactorySession(ctx, pendingRequest(started, "fs-pending-1")); err != nil {
		t.Fatalf("RecordPendingFactorySession repeat: %v", err)
	}
}

// TestStore_RecordPendingFactorySession_ClearsWithBlankIdentity proves a
// blank FactorySessionID clears a previously recorded pending identity
// (unlike BindFactorySession, a blank value is not rejected as invalid --
// it is the documented explicit-clear signal).
func TestStore_RecordPendingFactorySession_ClearsWithBlankIdentity(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	if _, err := store.RecordPendingFactorySession(ctx, pendingRequest(started, "fs-pending-1")); err != nil {
		t.Fatalf("RecordPendingFactorySession record: %v", err)
	}
	if _, err := store.RecordPendingFactorySession(ctx, pendingRequest(started, "")); err != nil {
		t.Fatalf("RecordPendingFactorySession clear: %v", err)
	}

	store.mu.RLock()
	record := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if record.episodes[0].PendingFactorySessionID != "" {
		t.Fatalf("episode PendingFactorySessionID = %q, want cleared to blank", record.episodes[0].PendingFactorySessionID)
	}
}

// TestStore_RecordPendingFactorySession_StaleVersionIsTypedConflict proves a
// record whose ExpectedVersion no longer matches (and whose episode is not
// already carrying the same value) reports *ConflictError and mutates
// nothing.
func TestStore_RecordPendingFactorySession_StaleVersionIsTypedConflict(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	req := pendingRequest(started, "fs-pending-1")
	req.ExpectedVersion = started.Session.Version + 1

	_, err := store.RecordPendingFactorySession(ctx, req)
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("RecordPendingFactorySession stale version: got %v, want *ConflictError", err)
	}

	store.mu.RLock()
	record := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if record.episodes[0].PendingFactorySessionID != "" {
		t.Fatalf("episode PendingFactorySessionID = %q after stale-version record, want blank", record.episodes[0].PendingFactorySessionID)
	}
}

// TestStore_RecordPendingFactorySession_TurnNoLongerActiveIsTypedConflict
// proves a record whose TurnID is no longer the session's active turn
// reports *ConflictError.
func TestStore_RecordPendingFactorySession_TurnNoLongerActiveIsTypedConflict(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: started.Session.ID,
		TurnID:    started.Turn.ID,
		Next:      chatsessions.TurnStateCanceled,
	}); err != nil {
		t.Fatalf("AdvanceTurn to terminal: %v", err)
	}

	_, err := store.RecordPendingFactorySession(ctx, pendingRequest(started, "fs-pending-1"))
	var conflict *chatsessions.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("RecordPendingFactorySession after turn terminated: got %v, want *ConflictError", err)
	}
}

// TestStore_RecordPendingFactorySession_UnknownSessionIsTypedNotFound proves
// recording against an unknown SessionID reports *NotFoundError.
func TestStore_RecordPendingFactorySession_UnknownSessionIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

	_, err := store.RecordPendingFactorySession(ctx, chatsessions.RecordPendingFactorySessionRequest{
		SessionID: "does-not-exist", ExpectedVersion: 1, Episode: 1, TurnID: "turn-1", FactorySessionID: "fs-pending-1",
	})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("RecordPendingFactorySession unknown session: got %v, want *NotFoundError", err)
	}
}

// TestStore_RecordPendingFactorySession_ThenBindClearsPending proves a
// successful BindFactorySession commit clears the episode's
// PendingFactorySessionID that RecordPendingFactorySession set, since the
// pending record's purpose is moot once the identity is durably committed.
func TestStore_RecordPendingFactorySession_ThenBindClearsPending(t *testing.T) {
	ctx := context.Background()
	store, started := startedTurnForBinding(t, time.Now())

	if _, err := store.RecordPendingFactorySession(ctx, pendingRequest(started, "fs-pending-1")); err != nil {
		t.Fatalf("RecordPendingFactorySession: %v", err)
	}
	if _, err := store.BindFactorySession(ctx, bindRequest(started, "fs-pending-1")); err != nil {
		t.Fatalf("BindFactorySession: %v", err)
	}

	store.mu.RLock()
	record := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if record.episodes[0].PendingFactorySessionID != "" {
		t.Fatalf("episode PendingFactorySessionID = %q after a successful bind, want cleared to blank", record.episodes[0].PendingFactorySessionID)
	}
	if record.episodes[0].FactorySessionID != "fs-pending-1" {
		t.Fatalf("episode FactorySessionID = %q, want fs-pending-1", record.episodes[0].FactorySessionID)
	}
}
