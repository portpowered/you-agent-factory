package stdio

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

type promptHistoryFailure struct {
	sequence          error
	advanceStreamHead error
}

var promptHistoryFailures = struct {
	sync.Mutex
	byChatSessions map[*fakeChatSessionsService]promptHistoryFailure
}{byChatSessions: make(map[*fakeChatSessionsService]promptHistoryFailure)}

func configurePromptHistoryFailure(t *testing.T, chatSessions *fakeChatSessionsService, failure promptHistoryFailure) {
	t.Helper()
	promptHistoryFailures.Lock()
	promptHistoryFailures.byChatSessions[chatSessions] = failure
	promptHistoryFailures.Unlock()
	t.Cleanup(func() {
		promptHistoryFailures.Lock()
		delete(promptHistoryFailures.byChatSessions, chatSessions)
		promptHistoryFailures.Unlock()
	})
}

func promptHistoryFailureFor(chatSessions *fakeChatSessionsService) promptHistoryFailure {
	promptHistoryFailures.Lock()
	defer promptHistoryFailures.Unlock()
	return promptHistoryFailures.byChatSessions[chatSessions]
}

// recordFakePromptHistory supplies the shared ACP handler fake's narrow
// user-recording behavior. Keeping it here lets prompt-history tests control
// its failures without coupling unrelated session/new tests to that fixture.
func recordFakePromptHistory(chatSessions *fakeChatSessionsService, _ context.Context, req chatsessions.SequenceRequest) (chatsessions.SequenceResult, error) {
	if err := promptHistoryFailureFor(chatSessions).sequence; err != nil {
		return chatsessions.SequenceResult{}, err
	}
	chatSessions.mu.Lock()
	defer chatSessions.mu.Unlock()
	sequence := events.AggregateSequence(chatSessions.getSessionResult.Session.StreamHead + 1)
	return chatsessions.SequenceResult{
		SessionID: req.SessionID, ItemID: fmt.Sprintf("prompt-history-%d", sequence), AggregateSequence: sequence,
		Outcome: chatsessions.SequenceOutcomeAccepted,
	}, nil
}

func advanceFakePromptHistoryStreamHead(chatSessions *fakeChatSessionsService, _ context.Context, req chatsessions.AdvanceStreamHeadRequest) (chatsessions.AdvanceStreamHeadResult, error) {
	if err := promptHistoryFailureFor(chatSessions).advanceStreamHead; err != nil {
		return chatsessions.AdvanceStreamHeadResult{}, err
	}
	chatSessions.mu.Lock()
	defer chatSessions.mu.Unlock()
	chatSessions.getSessionResult.Session.StreamHead = uint64(req.AggregateSequence)
	chatSessions.getSessionResult.Session.Version++
	return chatsessions.AdvanceStreamHeadResult{
		Session: chatSessions.getSessionResult.Session, Outcome: chatsessions.AdvanceStreamHeadOutcomeAdvanced,
	}, nil
}

func TestHandleSessionPromptHistoryRecordingFailureStopsBeforeFactoryDispatch(t *testing.T) {
	tests := []struct {
		name    string
		failure promptHistoryFailure
	}{
		{name: "sequence failure", failure: promptHistoryFailure{sequence: errors.New("sequence prompt history")}},
		{name: "stream head failure", failure: promptHistoryFailure{advanceStreamHead: errors.New("advance prompt history stream head")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factoryTarget := &fakeFactoryTargetService{}
			server, _ := newStreamingTestServer(t, factoryTarget, "turn-1")
			chatSessions := server.chatSessions.(*fakeChatSessionsService)
			configurePromptHistoryFailure(t, chatSessions, test.failure)

			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams(streamingTestSessionID, "retain this question"))
			if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr == nil {
				t.Fatal("handleSessionPrompt() error = nil, want a bounded history-recording failure")
			}
			if got := len(factoryTarget.startCalls) + len(factoryTarget.invokeCalls); got != 0 {
				t.Fatalf("Factory Session calls = %d, want 0 after history recording failed", got)
			}
			wantAdvanceTurnSequence(t, chatSessions, streamingTestSessionID, "turn-1",
				chatsessions.TurnStateRunning, chatsessions.TurnStateFailed)
		})
	}
}
