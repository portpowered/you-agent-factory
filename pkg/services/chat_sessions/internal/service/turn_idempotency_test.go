package service

import (
	"context"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// TestStore_AdvanceTurn_RepeatedTerminalStateIsIdempotent proves a prompt
// finalizer may observe that the captured close already recorded the same
// terminal state without rewriting the turn, terminal sequence, or session.
func TestStore_AdvanceTurn_RepeatedTerminalStateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, session := newStartTurnTestSession(t, time.Now())
	started, err := store.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID:       startTurnRequestID("idempotent-terminal"),
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if _, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: started.Session.ID, TurnID: started.Turn.ID, Next: chatsessions.TurnStateCanceled,
	}); err != nil {
		t.Fatalf("AdvanceTurn(CANCELED): %v", err)
	}

	store.mu.RLock()
	before := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	repeated, err := store.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: started.Session.ID, TurnID: started.Turn.ID, Next: chatsessions.TurnStateCanceled,
	})
	if err != nil {
		t.Fatalf("repeated AdvanceTurn(CANCELED): %v", err)
	}
	store.mu.RLock()
	after := store.sessions[started.Session.ID]
	store.mu.RUnlock()
	if repeated.Turn != before.turns[started.Turn.ID] || after.session != before.session || after.turnSequence != before.turnSequence || after.turns[started.Turn.ID] != before.turns[started.Turn.ID] {
		t.Fatalf("repeated terminal advance rewrote state: result=%#v before=%#v after=%#v", repeated.Turn, before, after)
	}
}
