package stdio

import (
	"context"
	"encoding/json"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
)

func closeRequestEnvelope(t *testing.T, requestID int64, sessionID string) envelope.Envelope {
	t.Helper()
	return numberIdentityEnvelope(t, "close-conn", requestID, acpsdk.AgentMethodSessionClose, `{"sessionId":"`+sessionID+`"}`)
}

// TestHandleSessionCloseCommitsBeforeFactoryClose proves the ACP close path
// captures and commits a CLOSE intent before it reaches the bound Factory
// Session, then exposes success only after Chat Sessions is terminal.
func TestHandleSessionCloseCommitsBeforeFactoryClose(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-close-bound")
	chatSessions := &controlRecordingChatSessions{Service: base}
	factoryTarget := &fakeFactoryTargetService{closeEntered: make(chan struct{}), closeRelease: make(chan struct{})}
	server := New(nil, chatSessions, nil, factoryTarget, nil)
	env := closeRequestEnvelope(t, 41, session.ID)

	type closeResult struct {
		result json.RawMessage
		err    *acpsdk.RequestError
	}
	done := make(chan closeResult, 1)
	go func() {
		result, err := server.dispatchRequest(context.Background(), env)
		done <- closeResult{result: result, err: err}
	}()

	waitForChannel(t, factoryTarget.closeEntered, "Factory Sessions CloseFactorySession")
	requests, advances := chatSessions.snapshotControls()
	if len(requests) != 1 {
		t.Fatalf("RequestControl calls = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.Action != chatsessions.ControlActionClose || request.SessionID != session.ID || request.ExpectedVersion != session.Version {
		t.Fatalf("RequestControl = %#v, want captured CLOSE for session/version", request)
	}
	if request.RequestID.Kind != chatsessions.RequestIdentityKindJSONRPCNumber || request.RequestID.JSONRPCNumberID != "41" {
		t.Fatalf("RequestControl identity = %#v, want the request's full JSON-RPC identity", request.RequestID)
	}
	if len(advances) != 1 || advances[0].Intent.State != chatsessions.ControlIntentStateCommitted || advances[0].Intent.TurnID != turn.ID {
		t.Fatalf("advances before Factory close returns = %#v, want captured COMMITTED intent", advances)
	}

	close(factoryTarget.closeRelease)
	response := <-done
	if response.err != nil {
		t.Fatalf("session/close error = %+v, want success", response.err)
	}
	var closeResponse acpsdk.CloseSessionResponse
	if err := json.Unmarshal(response.result, &closeResponse); err != nil {
		t.Fatalf("unmarshal CloseSessionResponse: %v", err)
	}
	factoryTarget.mu.Lock()
	closeCalls := append([]string(nil), factoryTarget.closeCalls...)
	factoryTarget.mu.Unlock()
	if len(closeCalls) != 1 || closeCalls[0] != "fs-close-bound" {
		t.Fatalf("CloseFactorySession calls = %#v, want only captured fs-close-bound", closeCalls)
	}

	closed, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after close: %v", err)
	}
	if closed.Session.State != chatsessions.SessionStateClosed || closed.Episode.State != chatsessions.TargetEpisodeStateClosed {
		t.Fatalf("close did not terminalize Chat lifecycle: Session=%#v Episode=%#v", closed.Session, closed.Episode)
	}
	_, advances = chatSessions.snapshotControls()
	if len(advances) != 2 || advances[1].Intent.State != chatsessions.ControlIntentStateCompleted {
		t.Fatalf("advances = %#v, want COMMITTED then COMPLETED", advances)
	}
}

// TestHandleSessionCloseUsesCapturedPendingFactorySession proves first-turn
// close reaches the pending identity that Chat Sessions recorded before the
// eventual bind, not a blank or newly selected Factory Session.
func TestHandleSessionCloseUsesCapturedPendingFactorySession(t *testing.T) {
	current := chatsessions.GetSessionResult{
		Session: chatsessions.Session{
			ID: "session-pending", State: chatsessions.SessionStateActive, Version: 7, ActiveTurnID: "turn-pending",
		},
		Episode: chatsessions.TargetEpisode{
			Number: 3, State: chatsessions.TargetEpisodeStateOpen,
			Target:                  chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
			PendingFactorySessionID: "fs-close-pending",
		},
		MostRecentTurnID: "turn-pending",
	}
	chatSessions := &fakeChatSessionsService{getSessionResult: current}
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, chatSessions, nil, factoryTarget, nil)

	result, rpcErr := server.dispatchRequest(context.Background(), closeRequestEnvelope(t, 42, current.Session.ID))
	if rpcErr != nil {
		t.Fatalf("session/close error = %+v, want success", rpcErr)
	}
	var response acpsdk.CloseSessionResponse
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatalf("unmarshal CloseSessionResponse: %v", err)
	}
	factoryTarget.mu.Lock()
	closeCalls := append([]string(nil), factoryTarget.closeCalls...)
	factoryTarget.mu.Unlock()
	if len(closeCalls) != 1 || closeCalls[0] != "fs-close-pending" {
		t.Fatalf("CloseFactorySession calls = %#v, want pending fs-close-pending", closeCalls)
	}
}

// TestHandleSessionCloseIsIdempotentAndRejectsPostClosePrompt proves a
// repeated close neither reaches Factory Sessions again nor reopens Chat
// state, and that a later prompt is rejected before StartAsync or Invoke.
func TestHandleSessionCloseIsIdempotentAndRejectsPostClosePrompt(t *testing.T) {
	base, session, _ := newActiveBoundControlSession(t, "fs-close-repeat")
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, base, nil, factoryTarget, nil)

	for _, requestID := range []int64{43, 44} {
		result, rpcErr := server.dispatchRequest(context.Background(), closeRequestEnvelope(t, requestID, session.ID))
		if rpcErr != nil {
			t.Fatalf("session/close request %d error = %+v, want success", requestID, rpcErr)
		}
		var response acpsdk.CloseSessionResponse
		if err := json.Unmarshal(result, &response); err != nil {
			t.Fatalf("unmarshal CloseSessionResponse request %d: %v", requestID, err)
		}
	}
	factoryTarget.mu.Lock()
	closeCount := len(factoryTarget.closeCalls)
	startCount := len(factoryTarget.startCalls)
	invokeCount := len(factoryTarget.invokeCalls)
	factoryTarget.mu.Unlock()
	if closeCount != 1 {
		t.Fatalf("CloseFactorySession calls = %d, want exactly one across repeated close", closeCount)
	}

	promptEnv := numberIdentityEnvelope(t, "close-conn", 45, acpsdk.AgentMethodSessionPrompt, promptTextParams(session.ID, "must not restart"))
	_, promptErr := server.dispatchRequest(context.Background(), promptEnv)
	if promptErr == nil || promptErr.Code != -32602 {
		t.Fatalf("post-close session/prompt error = %+v, want invalid params", promptErr)
	}
	factoryTarget.mu.Lock()
	defer factoryTarget.mu.Unlock()
	if len(factoryTarget.startCalls) != startCount || len(factoryTarget.invokeCalls) != invokeCount {
		t.Fatalf("post-close prompt reached Factory Sessions: StartAsync=%d Invoke=%d, want unchanged %d/%d",
			len(factoryTarget.startCalls), len(factoryTarget.invokeCalls), startCount, invokeCount)
	}
}

var _ factorysessions.TargetExecutionService = (*fakeFactoryTargetService)(nil)
