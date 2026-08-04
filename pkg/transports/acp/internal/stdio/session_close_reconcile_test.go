package stdio

import (
	"context"
	"errors"
	"testing"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestReconcileCanceledCloseAfterBindFailurePreservesCapturedOutcome(t *testing.T) {
	start := chatsessions.StartTurnResult{
		Session: chatsessions.Session{ID: "session-1"},
		Turn:    chatsessions.Turn{ID: "turn-1"},
		Episode: chatsessions.TargetEpisode{Number: 1},
	}
	chatSessions := &fakeChatSessionsService{getSessionResult: chatsessions.GetSessionResult{
		Session:          chatsessions.Session{ID: start.Session.ID, Version: 7, State: chatsessions.SessionStateClosed},
		Episode:          chatsessions.TargetEpisode{Number: start.Episode.Number, State: chatsessions.TargetEpisodeStateClosed},
		MostRecentTurnID: start.Turn.ID,
	}}
	server := New(nil, chatSessions, nil, &fakeFactoryTargetService{}, nil, nil, nil)

	got, ok := server.reconcileCanceledCloseAfterBindFailure(
		context.Background(), start,
		factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCanceled},
		true,
		&chatsessions.ConflictError{Value: "Turn", ID: start.Turn.ID},
	)
	if !ok {
		t.Fatal("reconcileCanceledCloseAfterBindFailure() = false, want the closed captured lifecycle accepted")
	}
	if got.terminal != chatsessions.TurnStateCanceled || got.sessionVersion != 7 || !got.liveDelivered {
		t.Fatalf("reconciled outcome = %#v, want canceled at closed session version 7", got)
	}
}

func TestReconcileCanceledCloseAfterBindFailureRejectsAnythingButCapturedCloseRace(t *testing.T) {
	start := chatsessions.StartTurnResult{
		Session: chatsessions.Session{ID: "session-1"},
		Turn:    chatsessions.Turn{ID: "turn-1"},
		Episode: chatsessions.TargetEpisode{Number: 1},
	}
	chatSessions := &fakeChatSessionsService{getSessionResult: chatsessions.GetSessionResult{
		Session:          chatsessions.Session{ID: start.Session.ID, Version: 7, State: chatsessions.SessionStateActive, ActiveTurnID: start.Turn.ID},
		Episode:          chatsessions.TargetEpisode{Number: start.Episode.Number, State: chatsessions.TargetEpisodeStateOpen},
		MostRecentTurnID: start.Turn.ID,
	}}
	server := New(nil, chatSessions, nil, &fakeFactoryTargetService{}, nil, nil, nil)

	for _, test := range []struct {
		name    string
		outcome factorysessions.InvocationResult
		err     error
	}{
		{name: "completed invocation", outcome: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, err: &chatsessions.ConflictError{Value: "Turn", ID: start.Turn.ID}},
		{name: "different conflict", outcome: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCanceled}, err: &chatsessions.ConflictError{Value: "Turn", ID: "replacement-turn"}},
		{name: "non-conflict failure", outcome: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCanceled}, err: errors.New("bind failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := server.reconcileCanceledCloseAfterBindFailure(context.Background(), start, test.outcome, false, test.err); ok {
				t.Fatal("reconcileCanceledCloseAfterBindFailure() = true, want the original bind failure retained")
			}
		})
	}
}
