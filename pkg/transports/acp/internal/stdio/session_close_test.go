package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)
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
	factoryTarget.mu.Lock()
	terminateCalls := append([]terminateFactoryTargetCall(nil), factoryTarget.terminateCalls...)
	factoryTarget.mu.Unlock()
	if len(terminateCalls) != 1 || terminateCalls[0].sessionID != "fs-close-bound" {
		t.Fatalf("TerminateFactorySession calls = %#v, want only captured fs-close-bound", terminateCalls)
	}
	if got := terminateCalls[0].request; got.TurnID != turn.ID || got.RequestID != factoryTerminateRequestID(request.RequestID) {
		t.Fatalf("TerminateFactorySession control = %#v, want captured turn and stable control identity", got)
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

// TestHandleSessionCloseFencesTargetReplacementAfterTurnCompletion proves the
// public ACP close path keeps the captured episode authoritative from
// COMMITTED through downstream close. A normally completed prompt releases
// ActiveTurnID, but SetTarget remains rejected until close terminalizes the
// same captured lifecycle instead of leaving a closed Factory target beside a
// newly open Chat target.
func TestHandleSessionCloseFencesTargetReplacementAfterTurnCompletion(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-close-target-fence")
	factoryTarget := &fakeFactoryTargetService{closeEntered: make(chan struct{}), closeRelease: make(chan struct{})}
	server := New(nil, base, nil, factoryTarget, nil, nil, nil, nil)
	type closeResult struct{ err *acpsdk.RequestError }
	done := make(chan closeResult, 1)
	go func() {
		_, err := server.dispatchRequest(context.Background(), closeRequestEnvelope(t, 411, session.ID))
		done <- closeResult{err: err}
	}()

	waitForChannel(t, factoryTarget.closeEntered, "Factory Sessions CloseFactorySession")
	if _, err := base.AdvanceTurn(context.Background(), chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceTurn(COMPLETED): %v", err)
	}
	released, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after completion: %v", err)
	}
	if _, err := base.SetTarget(context.Background(), chatsessions.SetTargetRequest{
		RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "close-target-fence", JSONRPCNumberID: "1"},
		SessionID: session.ID, ExpectedVersion: released.Session.Version,
		Target: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/other"},
	}); !errors.Is(err, chatsessions.ErrBusy) {
		t.Fatalf("SetTarget while ACP CLOSE is committed: got %v, want ErrBusy", err)
	}

	close(factoryTarget.closeRelease)
	response := <-done
	if response.err != nil {
		t.Fatalf("session/close error = %+v, want success", response.err)
	}
	closed, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after close: %v", err)
	}
	if closed.Session.State != chatsessions.SessionStateClosed || closed.Episode.State != chatsessions.TargetEpisodeStateClosed {
		t.Fatalf("ACP close left a partial lifecycle: Session=%#v Episode=%#v", closed.Session, closed.Episode)
	}
	factoryTarget.mu.Lock()
	closeCalls := append([]string(nil), factoryTarget.closeCalls...)
	factoryTarget.mu.Unlock()
	if len(closeCalls) != 1 || closeCalls[0] != "fs-close-target-fence" {
		t.Fatalf("CloseFactorySession calls = %#v, want only the captured target", closeCalls)
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
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)

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
	server := New(nil, base, nil, factoryTarget, nil, nil, nil, nil)

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

// TestHandleSessionCloseRejectsInvalidInputsWithoutFactoryEffects proves the
// request boundary rejects malformed or unavailable close targets before it
// can request a control intent or reach Factory Sessions.
func TestHandleSessionCloseRejectsInvalidInputsWithoutFactoryEffects(t *testing.T) {
	tests := []struct {
		name         string
		env          envelope.Envelope
		chatSessions *fakeChatSessionsService
		factory      factorysessions.TargetExecutionService
		wantCode     int
		wantGet      bool
		wantControls int
	}{
		{
			name:         "malformed params",
			env:          numberIdentityEnvelope(t, "close-invalid", 51, acpsdk.AgentMethodSessionClose, `{"sessionId":`),
			chatSessions: &fakeChatSessionsService{},
			factory:      &fakeFactoryTargetService{},
			wantCode:     -32602,
		},
		{
			name:         "blank session id",
			env:          closeRequestEnvelope(t, 52, ""),
			chatSessions: &fakeChatSessionsService{},
			factory:      &fakeFactoryTargetService{},
			wantCode:     -32602,
		},
		{
			name: "unknown session",
			env:  closeRequestEnvelope(t, 53, "session-not-found"),
			chatSessions: &fakeChatSessionsService{
				getSessionErr: &chatsessions.NotFoundError{Value: "Session", ID: "session-not-found"},
			},
			factory:  &fakeFactoryTargetService{},
			wantCode: -32602,
			wantGet:  true,
		},
		{
			name: "unbound target episode",
			env:  closeRequestEnvelope(t, 54, "session-unbound"),
			chatSessions: &fakeChatSessionsService{getSessionResult: chatsessions.GetSessionResult{
				Session: chatsessions.Session{ID: "session-unbound", State: chatsessions.SessionStateActive, Version: 3, ActiveTurnID: "turn-unbound"},
				Episode: chatsessions.TargetEpisode{
					Number: 1, State: chatsessions.TargetEpisodeStateOpen,
					Target: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
				},
			}},
			factory:      &fakeFactoryTargetService{},
			wantCode:     -32602,
			wantGet:      true,
			wantControls: 0,
		},
		{
			name:         "missing target collaborator",
			env:          closeRequestEnvelope(t, 55, "session-any"),
			chatSessions: &fakeChatSessionsService{},
			factory:      nil,
			wantCode:     -32603,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := New(nil, test.chatSessions, nil, test.factory, nil, nil, nil, nil)
			_, rpcErr := server.dispatchRequest(context.Background(), test.env)
			if rpcErr == nil || rpcErr.Code != test.wantCode {
				t.Fatalf("session/close error = %+v, want code %d", rpcErr, test.wantCode)
			}

			test.chatSessions.mu.Lock()
			gotGet := test.chatSessions.getSessionCalled
			gotControls := len(test.chatSessions.requestControlReqs)
			test.chatSessions.mu.Unlock()
			if gotGet != test.wantGet {
				t.Fatalf("GetSession called = %t, want %t", gotGet, test.wantGet)
			}
			if gotControls != test.wantControls {
				t.Fatalf("RequestControl calls = %d, want %d", gotControls, test.wantControls)
			}
			if target, ok := test.factory.(*fakeFactoryTargetService); ok {
				target.mu.Lock()
				closeCalls := len(target.closeCalls)
				target.mu.Unlock()
				if closeCalls != 0 {
					t.Fatalf("CloseFactorySession calls = %d, want 0", closeCalls)
				}
			}
		})
	}
}

// TestHandleSessionCloseWithoutActiveTurnLeavesLifecycleOpen proves close
// refuses a session that has no active turn without inventing a control or
// closing the episode's already-bound Factory Session.
func TestHandleSessionCloseWithoutActiveTurnLeavesLifecycleOpen(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-close-no-active")
	if _, err := base.AdvanceTurn(context.Background(), chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceTurn(COMPLETED): %v", err)
	}
	chatSessions := &controlRecordingChatSessions{Service: base}
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)

	_, rpcErr := server.dispatchRequest(context.Background(), closeRequestEnvelope(t, 56, session.ID))
	if rpcErr == nil || rpcErr.Code != -32602 {
		t.Fatalf("session/close error = %+v, want invalid params", rpcErr)
	}
	requests, _ := chatSessions.snapshotControls()
	if len(requests) != 0 {
		t.Fatalf("RequestControl calls = %#v, want none", requests)
	}
	factoryTarget.mu.Lock()
	closeCalls := len(factoryTarget.closeCalls)
	factoryTarget.mu.Unlock()
	if closeCalls != 0 {
		t.Fatalf("CloseFactorySession calls = %d, want 0", closeCalls)
	}
	current, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if current.Session.State != chatsessions.SessionStateActive || current.Episode.State != chatsessions.TargetEpisodeStateOpen {
		t.Fatalf("close changed terminal-less lifecycle: Session=%#v Episode=%#v", current.Session, current.Episode)
	}
}

// TestHandleSessionCloseStaleCaptureCannotReachReplacement proves a stale
// version between the initial read and RequestControl is rejected before the
// factory target. The newer replacement turn remains the active authority.
func TestHandleSessionCloseStaleCaptureCannotReachReplacement(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-close-stale")
	release := make(chan struct{})
	chatSessions := &controlRecordingChatSessions{
		Service:        base,
		requestEntered: make(chan struct{}),
		requestRelease: release,
	}
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)
	env := closeRequestEnvelope(t, 57, session.ID)

	type closeResult struct{ err *acpsdk.RequestError }
	done := make(chan closeResult, 1)
	go func() {
		_, err := server.dispatchRequest(context.Background(), env)
		done <- closeResult{err: err}
	}()
	waitForChannel(t, chatSessions.requestEntered, "session/close before RequestControl")
	if _, err := base.AdvanceTurn(context.Background(), chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceTurn(COMPLETED): %v", err)
	}
	current, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession before replacement: %v", err)
	}
	replacement, err := base.StartTurn(context.Background(), chatsessions.StartTurnRequest{
		RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "close-stale", JSONRPCNumberID: "58"},
		SessionID:       session.ID,
		ExpectedVersion: current.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn(replacement): %v", err)
	}
	close(release)
	result := <-done
	if result.err == nil || result.err.Code != -32602 {
		t.Fatalf("stale session/close error = %+v, want invalid params", result.err)
	}

	factoryTarget.mu.Lock()
	closeCalls := len(factoryTarget.closeCalls)
	factoryTarget.mu.Unlock()
	if closeCalls != 0 {
		t.Fatalf("CloseFactorySession calls = %d, want 0", closeCalls)
	}
	updated, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after stale close: %v", err)
	}
	if updated.Session.ActiveTurnID != replacement.Turn.ID || updated.Session.State != chatsessions.SessionStateActive || updated.Episode.State != chatsessions.TargetEpisodeStateOpen {
		t.Fatalf("stale close changed replacement lifecycle: Session=%#v Episode=%#v", updated.Session, updated.Episode)
	}
}

// TestHandleSessionCloseDependencyFailureLeavesLifecycleRetryable proves a
// Factory Sessions close error does not partially close the Chat aggregate,
// exposes only the bounded protocol failure, and leaves the captured intent
// retryable by its exact request identity.
func TestHandleSessionCloseDependencyFailureLeavesLifecycleRetryable(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-close-failure")
	chatSessions := &controlRecordingChatSessions{Service: base}
	failureText := "provider credential at /unsafe/path"
	factoryTarget := &fakeFactoryTargetService{closeErr: errors.New(failureText)}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)
	env := closeRequestEnvelope(t, 59, session.ID)

	_, rpcErr := server.dispatchRequest(context.Background(), env)
	if rpcErr == nil || rpcErr.Code != -32603 {
		t.Fatalf("failed session/close error = %+v, want internal error", rpcErr)
	}
	encoded, err := json.Marshal(rpcErr)
	if err != nil {
		t.Fatalf("marshal RequestError: %v", err)
	}
	if strings.Contains(string(encoded), failureText) || strings.Contains(string(encoded), "/unsafe/path") {
		t.Fatalf("close failure leaked dependency detail: %s", encoded)
	}
	current, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after failed close: %v", err)
	}
	if current.Session.State != chatsessions.SessionStateActive || current.Session.ActiveTurnID != turn.ID || current.Episode.State != chatsessions.TargetEpisodeStateOpen {
		t.Fatalf("failed close partially changed Chat lifecycle: Session=%#v Episode=%#v", current.Session, current.Episode)
	}
	_, advances := chatSessions.snapshotControls()
	if len(advances) != 1 || advances[0].Intent.State != chatsessions.ControlIntentStateCommitted {
		t.Fatalf("control advances = %#v, want only COMMITTED after failed Factory close", advances)
	}

	factoryTarget.mu.Lock()
	factoryTarget.closeErr = nil
	factoryTarget.mu.Unlock()
	result, retryErr := server.dispatchRequest(context.Background(), env)
	if retryErr != nil {
		t.Fatalf("retried session/close error = %+v, want success", retryErr)
	}
	var response acpsdk.CloseSessionResponse
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatalf("unmarshal retried CloseSessionResponse: %v", err)
	}
	factoryTarget.mu.Lock()
	closeCalls := append([]string(nil), factoryTarget.closeCalls...)
	factoryTarget.mu.Unlock()
	if len(closeCalls) != 2 || closeCalls[0] != "fs-close-failure" || closeCalls[1] != "fs-close-failure" {
		t.Fatalf("CloseFactorySession calls = %#v, want two retries for only the captured target", closeCalls)
	}
	closed, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after retried close: %v", err)
	}
	if closed.Session.State != chatsessions.SessionStateClosed || closed.Episode.State != chatsessions.TargetEpisodeStateClosed || closed.Session.ActiveTurnID != "" {
		t.Fatalf("retried close Chat lifecycle = Session=%#v Episode=%#v, want terminally closed captured lifecycle", closed.Session, closed.Episode)
	}
	_, advances = chatSessions.snapshotControls()
	if len(advances) != 2 || advances[1].Intent.State != chatsessions.ControlIntentStateCompleted {
		t.Fatalf("control advances after retry = %#v, want COMMITTED then COMPLETED", advances)
	}
}

// TestHandleSessionCloseCommittedIntentSafetyCases proves every re-read and
// intent-state rejection after the immutable CLOSE capture stays bounded and
// cannot reach an unrelated Factory Session.
func TestHandleSessionCloseCommittedIntentSafetyCases(t *testing.T) {
	current := chatsessions.GetSessionResult{
		Session: chatsessions.Session{ID: "session-close-safety", State: chatsessions.SessionStateActive, Version: 7, ActiveTurnID: "turn-close-safety"},
		Episode: chatsessions.TargetEpisode{
			Number: 1, State: chatsessions.TargetEpisodeStateOpen,
			Target:           chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
			FactorySessionID: "fs-close-safety",
		},
		MostRecentTurnID: "turn-close-safety",
	}
	closed := current
	closed.Session.State = chatsessions.SessionStateClosed
	closed.Session.ActiveTurnID = ""
	successor := current
	successor.Session.ActiveTurnID = "turn-successor"
	successor.MostRecentTurnID = "turn-successor"
	targetLost := current
	targetLost.Episode.FactorySessionID = ""

	tests := []struct {
		name            string
		chatSessions    *fakeChatSessionsService
		wantCode        int
		wantCloseCalls  int
		wantAdvanceCall int
	}{
		{
			name: "completed redelivery is success without downstream effect",
			chatSessions: &fakeChatSessionsService{
				getSessionResult: current,
				requestControlResult: chatsessions.RequestControlResult{Intent: chatsessions.ControlIntent{
					SessionID: current.Session.ID, Action: chatsessions.ControlActionClose, State: chatsessions.ControlIntentStateCompleted,
				}},
			},
			wantCode: 0,
		},
		{
			name: "session closes while committed control is reread",
			chatSessions: &fakeChatSessionsService{
				getSessionResult:  current,
				getSessionResults: []chatsessions.GetSessionResult{current, closed},
			},
			wantCode:        0,
			wantAdvanceCall: 1,
		},
		{
			name: "reread dependency failure after commit is bounded without downstream effect",
			chatSessions: &fakeChatSessionsService{
				getSessionResult: current,
				getSessionErrs:   []error{nil, errors.New("unsafe close reread failure")},
			},
			wantCode:        -32603,
			wantAdvanceCall: 1,
		},
		{
			name: "replacement after commit is invalid params without downstream effect",
			chatSessions: &fakeChatSessionsService{
				getSessionResult:  current,
				getSessionResults: []chatsessions.GetSessionResult{current, successor},
			},
			wantCode:        -32602,
			wantAdvanceCall: 2,
		},
		{
			name: "target disappears after commit is invalid params without downstream effect",
			chatSessions: &fakeChatSessionsService{
				getSessionResult:  current,
				getSessionResults: []chatsessions.GetSessionResult{current, targetLost},
			},
			wantCode:        -32602,
			wantAdvanceCall: 1,
		},
		{
			name: "unexpected captured action is invalid params without downstream effect",
			chatSessions: &fakeChatSessionsService{
				getSessionResult: current,
				requestControlResult: chatsessions.RequestControlResult{Intent: chatsessions.ControlIntent{
					SessionID: current.Session.ID, Action: chatsessions.ControlActionCancel, State: chatsessions.ControlIntentStateRequested,
				}},
			},
			wantCode: -32602,
		},
		{
			name: "terminal captured intent is invalid params without downstream effect",
			chatSessions: &fakeChatSessionsService{
				getSessionResult: current,
				requestControlResult: chatsessions.RequestControlResult{Intent: chatsessions.ControlIntent{
					SessionID: current.Session.ID, Action: chatsessions.ControlActionClose, State: chatsessions.ControlIntentStateNoop,
				}},
			},
			wantCode: -32602,
		},
		{
			name: "commit dependency failure is bounded without downstream effect",
			chatSessions: &fakeChatSessionsService{
				getSessionResult:  current,
				advanceControlErr: errors.New("unsafe provider error"),
			},
			wantCode:        -32603,
			wantAdvanceCall: 1,
		},
		{
			name: "terminalization rejection after factory close is invalid params",
			chatSessions: &fakeChatSessionsService{
				getSessionResult: current,
				advanceControlResults: []chatsessions.AdvanceControlResult{
					{Intent: chatsessions.ControlIntent{
						SessionID: current.Session.ID, TurnID: current.MostRecentTurnID, TargetEpisode: current.Episode.Number,
						State: chatsessions.ControlIntentStateCommitted,
					}},
					{Intent: chatsessions.ControlIntent{State: chatsessions.ControlIntentStateNoop}},
				},
			},
			wantCode:        -32602,
			wantCloseCalls:  1,
			wantAdvanceCall: 2,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factoryTarget := &fakeFactoryTargetService{}
			server := New(nil, test.chatSessions, nil, factoryTarget, nil, nil, nil, nil)
			result, rpcErr := server.dispatchRequest(context.Background(), closeRequestEnvelope(t, int64(70+index), current.Session.ID))
			if test.wantCode == 0 {
				if rpcErr != nil {
					t.Fatalf("session/close error = %+v, want success", rpcErr)
				}
				var response acpsdk.CloseSessionResponse
				if err := json.Unmarshal(result, &response); err != nil {
					t.Fatalf("unmarshal CloseSessionResponse: %v", err)
				}
			} else if rpcErr == nil || rpcErr.Code != test.wantCode {
				t.Fatalf("session/close error = %+v, want code %d", rpcErr, test.wantCode)
			}

			factoryTarget.mu.Lock()
			closeCalls := len(factoryTarget.closeCalls)
			factoryTarget.mu.Unlock()
			if closeCalls != test.wantCloseCalls {
				t.Fatalf("CloseFactorySession calls = %d, want %d", closeCalls, test.wantCloseCalls)
			}
			test.chatSessions.mu.Lock()
			advanceCalls := len(test.chatSessions.advanceControlReqs)
			test.chatSessions.mu.Unlock()
			if advanceCalls != test.wantAdvanceCall {
				t.Fatalf("AdvanceControl calls = %d, want %d", advanceCalls, test.wantAdvanceCall)
			}
		})
	}
}

// TestHandleSessionCloseRejectsUncorrelatedRequestIdentityWithoutEffects
// proves a request that cannot supply the connection-scoped Chat identity is
// bounded before it can read or control any lifecycle state.
func TestHandleSessionCloseRejectsUncorrelatedRequestIdentityWithoutEffects(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)

	_, rpcErr := server.handleSessionClose(context.Background(), envelope.Envelope{
		Params: json.RawMessage(`{"sessionId":"session-close-identity"}`),
	})
	if rpcErr == nil || rpcErr.Code != -32603 {
		t.Fatalf("session/close error = %+v, want bounded internal error", rpcErr)
	}
	chatSessions.mu.Lock()
	getCalls := len(chatSessions.getSessionReqs)
	chatSessions.mu.Unlock()
	if getCalls != 0 {
		t.Fatalf("GetSession calls = %d, want 0 after identity rejection", getCalls)
	}
	factoryTarget.mu.Lock()
	closeCalls := len(factoryTarget.closeCalls)
	factoryTarget.mu.Unlock()
	if closeCalls != 0 {
		t.Fatalf("CloseFactorySession calls = %d, want 0 after identity rejection", closeCalls)
	}
}

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
	server := New(nil, chatSessions, nil, &fakeFactoryTargetService{}, nil, nil, nil, nil)

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
	server := New(nil, chatSessions, nil, &fakeFactoryTargetService{}, nil, nil, nil, nil)

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

var _ factorysessions.TargetExecutionService = (*fakeFactoryTargetService)(nil)
