package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

func TestServeRoutesSessionCancelToCapturedFactorySession(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: chatsessions.GetSessionResult{
			Session: chatsessions.Session{ID: "session-1"},
			Episode: chatsessions.TargetEpisode{
				Number: 1, State: chatsessions.TargetEpisodeStateOpen,
				Target:           chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/review"},
				FactorySessionID: "fs-1",
			},
		},
	}
	factoryTarget := &fakeFactoryTargetService{}
	server := newTestServerWithFactoryTarget(chatSessions, nil, factoryTarget, "/home/operator")

	line := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"session-1"}}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no response for a session/cancel notification", out.Bytes())
	}

	if !chatSessions.getSessionCalled {
		t.Fatal("GetSession was not called, want the addressed session read to resolve its bound Factory Session identity")
	}
	if chatSessions.getSessionReq.SessionID != "session-1" {
		t.Fatalf("GetSession SessionID = %q, want session-1", chatSessions.getSessionReq.SessionID)
	}
	chatSessions.mu.Lock()
	controlRequests := append([]chatsessions.RequestControlRequest(nil), chatSessions.requestControlReqs...)
	controlAdvances := append([]chatsessions.AdvanceControlRequest(nil), chatSessions.advanceControlReqs...)
	chatSessions.mu.Unlock()
	if len(controlRequests) != 1 || controlRequests[0].Action != chatsessions.ControlActionCancel {
		t.Fatalf("RequestControl calls = %#v, want one CANCEL intent", controlRequests)
	}
	if controlRequests[0].RequestID.Kind != chatsessions.RequestIdentityKindTransportUUID {
		t.Fatalf("RequestControl identity = %#v, want notification transport UUID", controlRequests[0].RequestID)
	}
	if len(controlAdvances) != 2 || controlAdvances[0].Next != chatsessions.ControlIntentStateCommitted || controlAdvances[1].Next != chatsessions.ControlIntentStateCompleted {
		t.Fatalf("AdvanceControl calls = %#v, want COMMITTED then COMPLETED", controlAdvances)
	}
	if len(factoryTarget.cancelCalls) != 1 {
		t.Fatalf("Cancel call count = %d, want exactly 1 forwarded to the captured Factory Session", len(factoryTarget.cancelCalls))
	}
	if got := factoryTarget.cancelCalls[0].sessionID; got != "fs-1" {
		t.Fatalf("Cancel sessionID = %q, want the episode's bound identity fs-1", got)
	}
	if factoryTarget.cancelCalls[0].request.RequestID == "" {
		t.Fatal("Cancel request.RequestID is blank, want a non-blank correlated identity")
	}
}

// TestServeCancelReachesFactorySessionWhileItsOwnSessionPromptInvocationIsStillBlocked
// proves "session/cancel" is dispatched -- and reaches the captured Factory
// Session's Cancel operation -- while a "session/prompt" received earlier on
// the exact same connection is still blocked inside its own
// InvokeFactorySession call, not only after that call has already returned.
// This is the real ACP stdio protocol shape: a client always sends
// "session/cancel" while the prompt it means to cancel is still in flight,
// on the one connection carrying both. It also proves the still-blocked
// prompt, once its downstream call reports the outcome the cancellation
// caused, still terminalizes with the existing canceled-turn/stop-reason
// behavior -- cancellation's only observable effect from the caller's own
// side is that eventual "session/prompt" response, per handleSessionCancel's
// own doc comment.
func TestServeCancelReachesFactorySessionWhileItsOwnSessionPromptInvocationIsStillBlocked(t *testing.T) {
	getSessionResult := sessionAt("session-1", "factory:@you/review", 3, "/work/project")
	// handleSessionCancel resolves the bound Factory Session identity through
	// its own, separate GetSession call (not through admitPromptTurn's
	// StartTurn result), so this fake's Episode must carry the same
	// FactorySessionID startTurnResult's Episode below does.
	getSessionResult.Episode = chatsessions.TargetEpisode{
		Number: 1, State: chatsessions.TargetEpisodeStateOpen,
		Target:           chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
		FactorySessionID: "fs-already-bound",
	}
	chatSessions := &fakeChatSessionsService{
		getSessionResult: getSessionResult,
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		invokeEnter:   make(chan struct{}),
		invokeRelease: make(chan struct{}),
		cancelEntered: make(chan struct{}),
		invokeResult:  factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCanceled},
	}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(context.Background(), pr, out) }()

	promptLine := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":%s}`+"\n",
		promptTextParams("session-1", "a message to cancel"))
	if _, err := pw.Write([]byte(promptLine)); err != nil {
		t.Fatalf("write session/prompt line: %v", err)
	}

	select {
	case <-factoryTarget.invokeEnter:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for InvokeFactorySession to be entered")
	}

	cancelLine := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"session-1"}}` + "\n"
	if _, err := pw.Write([]byte(cancelLine)); err != nil {
		t.Fatalf("write session/cancel line: %v", err)
	}

	select {
	case <-factoryTarget.cancelEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Cancel to be entered")
	}

	// Cancel has already been recorded here, above, while
	// InvokeFactorySession -- confirmed still blocked by this same
	// assertion -- has not: proving cancellation reached the captured
	// runtime without ever waiting for the prompt's own call to return
	// first.
	factoryTarget.mu.Lock()
	cancelCallCount := len(factoryTarget.cancelCalls)
	cancelSessionID := ""
	if cancelCallCount > 0 {
		cancelSessionID = factoryTarget.cancelCalls[0].sessionID
	}
	invokeCallCount := len(factoryTarget.invokeCalls)
	factoryTarget.mu.Unlock()
	if cancelCallCount != 1 {
		t.Fatalf("Cancel call count = %d, want exactly 1 while the prompt invocation is still blocked", cancelCallCount)
	}
	if cancelSessionID != "fs-already-bound" {
		t.Fatalf("Cancel sessionID = %q, want the episode's bound identity fs-already-bound", cancelSessionID)
	}
	if invokeCallCount != 1 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 1 still in flight", invokeCallCount)
	}

	close(factoryTarget.invokeRelease)

	if err := pw.Close(); err != nil {
		t.Fatalf("close input pipe: %v", err)
	}
	if err := <-serveErr; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	assertCancelledPromptResponse(t, out)
}

// assertCancelledPromptResponse asserts out carries exactly one successful
// "session/prompt" response reporting acpsdk.StopReasonCancelled -- the
// shared tail assertion for the in-flight-cancellation tests above and
// below, which both drive a real Cancel-caused invocation outcome through
// to this same eventual response.
func assertCancelledPromptResponse(t *testing.T, out *bytes.Buffer) {
	t.Helper()
	resp := assertSingleResponseLine(t, out)
	if resp.Error != nil {
		t.Fatalf("response error = %+v, want success", resp.Error)
	}
	var promptResp acpsdk.PromptResponse
	if err := json.Unmarshal(resp.Result, &promptResp); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if promptResp.StopReason != acpsdk.StopReasonCancelled {
		t.Fatalf("stopReason = %q, want cancelled", promptResp.StopReason)
	}
}

func cancelNotificationEnvelope(t *testing.T, minted string, sessionID string) envelope.Envelope {
	t.Helper()
	id, err := identity.NewMinted(minted)
	if err != nil {
		t.Fatalf("NewMinted() error = %v", err)
	}
	return envelope.Envelope{
		Identity:       id,
		Method:         acpsdk.AgentMethodSessionCancel,
		Params:         json.RawMessage(`{"sessionId":"` + sessionID + `"}`),
		IsNotification: true,
	}
}

// TestHandleSessionCancelCommitsCapturedIntentBeforeFactoryCancel proves the
// control order at the ACP boundary: a notification's full transport identity
// is captured, the immutable CANCEL intent commits, and only then can the
// exact bound Factory Session observe Cancel.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestHandleSessionCancelCommitsCapturedIntentBeforeFactoryCancel(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-control-1")
	chatSessions := &controlRecordingChatSessions{Service: base}
	factoryTarget := &fakeFactoryTargetService{cancelEntered: make(chan struct{}), cancelRelease: make(chan struct{})}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)
	env := cancelNotificationEnvelope(t, "cancel-control-1", session.ID)

	done := make(chan struct{})
	go func() {
		server.handleSessionCancel(context.Background(), env)
		close(done)
	}()
	waitForChannel(t, factoryTarget.cancelEntered, "Factory Sessions Cancel")

	requests, advances := chatSessions.snapshotControls()
	if len(requests) != 1 {
		t.Fatalf("RequestControl calls = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.SessionID != session.ID || request.ExpectedVersion != session.Version || request.Action != chatsessions.ControlActionCancel {
		t.Fatalf("RequestControl request = %#v, want captured session/version CANCEL", request)
	}
	if request.RequestID.Kind != chatsessions.RequestIdentityKindTransportUUID || request.RequestID.TransportUUID == "" {
		t.Fatalf("RequestControl identity = %#v, want a non-blank transport UUID", request.RequestID)
	}
	if len(advances) != 1 || advances[0].Intent.State != chatsessions.ControlIntentStateCommitted {
		t.Fatalf("control advances before downstream Cancel = %#v, want only COMMITTED", advances)
	}

	factoryTarget.mu.Lock()
	cancelCalls := append([]cancelFactoryTargetCall(nil), factoryTarget.cancelCalls...)
	factoryTarget.mu.Unlock()
	if len(cancelCalls) != 1 || cancelCalls[0].sessionID != "fs-control-1" {
		t.Fatalf("Factory Sessions Cancel calls = %#v, want only captured fs-control-1", cancelCalls)
	}
	if cancelCalls[0].request.RequestID == "" {
		t.Fatal("Factory Sessions Cancel request id is blank")
	}
	if cancelCalls[0].request.TurnID != turn.ID {
		t.Fatalf("Factory Sessions Cancel turn id = %q, want captured turn %q", cancelCalls[0].request.TurnID, turn.ID)
	}

	close(factoryTarget.cancelRelease)
	waitForChannel(t, done, "cancel handler completion")
	_, advances = chatSessions.snapshotControls()
	if len(advances) != 2 || advances[1].Intent.State != chatsessions.ControlIntentStateCompleted {
		t.Fatalf("control advances = %#v, want COMMITTED then COMPLETED", advances)
	}
	if advances[0].Intent.TurnID != turn.ID || advances[0].Intent.TargetEpisode != turn.Episode {
		t.Fatalf("committed intent = %#v, want captured turn %q episode %d", advances[0].Intent, turn.ID, turn.Episode)
	}
}

// TestHandleSessionCancelCompletionRaceResolvesNoopWithoutFactoryEffect proves
// normal completion winning after capture but before commit causes the Store
// to derive NOOP and never calls the target execution capability.
func TestHandleSessionCancelCompletionRaceResolvesNoopWithoutFactoryEffect(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-control-noop")
	release := make(chan struct{})
	chatSessions := &controlRecordingChatSessions{Service: base, commitEntered: make(chan struct{}), commitRelease: release}
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)
	env := cancelNotificationEnvelope(t, "cancel-noop-1", session.ID)

	done := make(chan struct{})
	go func() {
		server.handleSessionCancel(context.Background(), env)
		close(done)
	}()
	waitForChannel(t, chatSessions.commitEntered, "captured control before commit")
	if _, err := base.AdvanceTurn(context.Background(), chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceTurn(COMPLETED) error = %v", err)
	}
	close(release)
	waitForChannel(t, done, "cancel handler completion")

	factoryTarget.mu.Lock()
	cancelCount := len(factoryTarget.cancelCalls)
	factoryTarget.mu.Unlock()
	if cancelCount != 0 {
		t.Fatalf("Factory Sessions Cancel calls = %d, want 0 after normal completion won", cancelCount)
	}
	_, advances := chatSessions.snapshotControls()
	if len(advances) != 2 || advances[1].Intent.State != chatsessions.ControlIntentStateNoop {
		t.Fatalf("control advances = %#v, want COMMITTED then NOOP", advances)
	}
}

// TestHandleSessionCancelSupersededRaceCannotReachReplacementTurn proves a
// REQUESTED control can lose to a successor before it commits, and its
// captured identity resolves SUPERSEDED without cancelling that successor.
func TestHandleSessionCancelSupersededRaceCannotReachReplacementTurn(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-control-old")
	release := make(chan struct{})
	chatSessions := &controlRecordingChatSessions{Service: base, commitEntered: make(chan struct{}), commitRelease: release}
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)
	env := cancelNotificationEnvelope(t, "cancel-superseded-1", session.ID)

	done := make(chan struct{})
	go func() {
		server.handleSessionCancel(context.Background(), env)
		close(done)
	}()
	waitForChannel(t, chatSessions.commitEntered, "captured control before commit")
	if _, err := base.AdvanceTurn(context.Background(), chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: turn.ID, Next: chatsessions.TurnStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceTurn(COMPLETED) error = %v", err)
	}
	current, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if _, err := base.StartTurn(context.Background(), chatsessions.StartTurnRequest{
		RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "control-setup", JSONRPCNumberID: "3"},
		SessionID:       session.ID,
		ExpectedVersion: current.Session.Version,
	}); err != nil {
		t.Fatalf("StartTurn(replacement) error = %v", err)
	}
	close(release)
	waitForChannel(t, done, "cancel handler completion")

	factoryTarget.mu.Lock()
	cancelCount := len(factoryTarget.cancelCalls)
	factoryTarget.mu.Unlock()
	if cancelCount != 0 {
		t.Fatalf("Factory Sessions Cancel calls = %d, want 0 for superseded control", cancelCount)
	}
	_, advances := chatSessions.snapshotControls()
	if len(advances) != 2 || advances[1].Intent.State != chatsessions.ControlIntentStateSuperseded {
		t.Fatalf("control advances = %#v, want COMMITTED then SUPERSEDED", advances)
	}
}

// TestHandleSessionCancelRepeatedIdentityDoesNotDuplicateFactoryCancel proves
// the same notification identity resolves through the immutable completed
// intent on redelivery rather than issuing a second downstream cancellation.
func TestHandleSessionCancelRepeatedIdentityDoesNotDuplicateFactoryCancel(t *testing.T) {
	base, session, _ := newActiveBoundControlSession(t, "fs-control-repeat")
	chatSessions := &controlRecordingChatSessions{Service: base}
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)
	env := cancelNotificationEnvelope(t, "cancel-repeat-1", session.ID)

	server.handleSessionCancel(context.Background(), env)
	server.handleSessionCancel(context.Background(), env)

	factoryTarget.mu.Lock()
	cancelCalls := append([]cancelFactoryTargetCall(nil), factoryTarget.cancelCalls...)
	factoryTarget.mu.Unlock()
	if len(cancelCalls) != 1 || cancelCalls[0].sessionID != "fs-control-repeat" {
		t.Fatalf("Factory Sessions Cancel calls = %#v, want exactly one captured cancellation", cancelCalls)
	}
	requests, advances := chatSessions.snapshotControls()
	if len(requests) != 2 || requests[0].RequestID != requests[1].RequestID {
		t.Fatalf("RequestControl calls = %#v, want the same identity on retry", requests)
	}
	if len(advances) != 2 || advances[0].Intent.State != chatsessions.ControlIntentStateCommitted || advances[1].Intent.State != chatsessions.ControlIntentStateCompleted {
		t.Fatalf("control advances = %#v, want one COMMITTED then one COMPLETED lifecycle", advances)
	}
}

// TestHandleSessionCancelCommittedSafetyCases proves a control that loses its
// target or collaborator after capture remains silent and cannot reach an
// unintended Factory Session. Each case exercises the notification boundary
// with a minted identity, so no JSON-RPC response is manufactured.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; preserve the move-only split and remove this exemption when the safety cases are refactored.
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestHandleSessionCancelCommittedSafetyCases(t *testing.T) {
	current := chatsessions.GetSessionResult{
		Session: chatsessions.Session{ID: "session-cancel-safety", State: chatsessions.SessionStateActive, Version: 7, ActiveTurnID: "turn-cancel-safety"},
		Episode: chatsessions.TargetEpisode{
			Number: 1, State: chatsessions.TargetEpisodeStateOpen,
			Target:           chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
			FactorySessionID: "fs-cancel-safety",
		},
		MostRecentTurnID: "turn-cancel-safety",
	}
	targetLost := current
	targetLost.Episode.FactorySessionID = ""
	successor := current
	successor.Session.ActiveTurnID = "turn-cancel-successor"
	successor.MostRecentTurnID = "turn-cancel-successor"

	tests := []struct {
		name            string
		chatSessions    *fakeChatSessionsService
		wantAdvanceCall int
	}{
		{
			name: "target disappears after commit",
			chatSessions: &fakeChatSessionsService{
				getSessionResult:  current,
				getSessionResults: []chatsessions.GetSessionResult{current, targetLost},
			},
			wantAdvanceCall: 1,
		},
		{
			name: "reread dependency failure after commit",
			chatSessions: &fakeChatSessionsService{
				getSessionResult: current,
				getSessionErrs:   []error{nil, errors.New("unsafe reread failure")},
			},
			wantAdvanceCall: 1,
		},
		{
			name: "request control failure",
			chatSessions: &fakeChatSessionsService{
				getSessionResult:  current,
				requestControlErr: errors.New("unsafe request control failure"),
			},
		},
		{
			name: "unexpected captured action",
			chatSessions: &fakeChatSessionsService{
				getSessionResult: current,
				requestControlResult: chatsessions.RequestControlResult{Intent: chatsessions.ControlIntent{
					SessionID: current.Session.ID, Action: chatsessions.ControlActionClose, State: chatsessions.ControlIntentStateRequested,
				}},
			},
		},
		{
			name: "commit failure",
			chatSessions: &fakeChatSessionsService{
				getSessionResult:  current,
				advanceControlErr: errors.New("unsafe commit failure"),
			},
			wantAdvanceCall: 1,
		},
		{
			name: "commit returns terminal control",
			chatSessions: &fakeChatSessionsService{
				getSessionResult: current,
				advanceControlResult: chatsessions.AdvanceControlResult{Intent: chatsessions.ControlIntent{
					State: chatsessions.ControlIntentStateNoop,
				}},
			},
			wantAdvanceCall: 1,
		},
		{
			name: "committed control becomes superseded on reread",
			chatSessions: &fakeChatSessionsService{
				getSessionResult:  current,
				getSessionResults: []chatsessions.GetSessionResult{current, successor},
			},
			wantAdvanceCall: 2,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factoryTarget := &fakeFactoryTargetService{}
			server := New(nil, test.chatSessions, nil, factoryTarget, nil, nil, nil, nil)
			server.handleSessionCancel(context.Background(), cancelNotificationEnvelope(t, fmt.Sprintf("cancel-safety-%d", index), current.Session.ID))

			factoryTarget.mu.Lock()
			cancelCalls := len(factoryTarget.cancelCalls)
			factoryTarget.mu.Unlock()
			if cancelCalls != 0 {
				t.Fatalf("Cancel calls = %d, want 0", cancelCalls)
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

// TestHandleSessionCancelRejectsUncorrelatedIdentityWithoutEffects proves a
// structurally invalid notification remains silent before any Chat or Factory
// operation is attempted.
func TestHandleSessionCancelRejectsUncorrelatedIdentityWithoutEffects(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	factoryTarget := &fakeFactoryTargetService{}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)

	server.handleSessionCancel(context.Background(), envelope.Envelope{
		Params: json.RawMessage(`{"sessionId":"session-cancel-identity"}`),
	})
	chatSessions.mu.Lock()
	getCalls := len(chatSessions.getSessionReqs)
	chatSessions.mu.Unlock()
	if getCalls != 0 {
		t.Fatalf("GetSession calls = %d, want 0 after identity rejection", getCalls)
	}
	factoryTarget.mu.Lock()
	cancelCalls := len(factoryTarget.cancelCalls)
	factoryTarget.mu.Unlock()
	if cancelCalls != 0 {
		t.Fatalf("Cancel calls = %d, want 0 after identity rejection", cancelCalls)
	}
}

func TestChatControlRequestIdentityKeepsDistinctNotificationsDistinct(t *testing.T) {
	first, err := identity.NewMinted("cancel-notification-a")
	if err != nil {
		t.Fatalf("NewMinted(first) error = %v", err)
	}
	second, err := identity.NewMinted("cancel-notification-b")
	if err != nil {
		t.Fatalf("NewMinted(second) error = %v", err)
	}
	firstID, err := chatControlRequestIdentity(first)
	if err != nil {
		t.Fatalf("chatControlRequestIdentity(first) error = %v", err)
	}
	secondID, err := chatControlRequestIdentity(second)
	if err != nil {
		t.Fatalf("chatControlRequestIdentity(second) error = %v", err)
	}
	if firstID == secondID {
		t.Fatalf("distinct notification identities mapped to one Chat identity: %#v", firstID)
	}
	if err := firstID.Validate(); err != nil {
		t.Fatalf("first Chat control identity is invalid: %v", err)
	}
}

// TestServeCancelReachesPendingFactorySessionDuringFirstTurnInvocation proves
// "session/cancel" reaches the exact runtime a first, currently-admitted
// turn started -- while that turn's own follow-up InvokeFactorySession call
// is still blocked and BindFactorySession has not yet committed it as the
// episode's FactorySessionID -- by routing through the episode's
// PendingFactorySessionID (recorded by RecordPendingFactorySession right
// after StartAsync succeeds; see startFactorySessionForEpisode and
// chatsessions.TargetEpisode.PendingFactorySessionID's own doc comments).
// This is the real-world cancellation window that matters most: a caller
// almost always cancels the very first prompt of a brand-new target episode
// while it is still running, not only a later, already-bound turn (which
// TestServeCancelReachesFactorySessionWhileItsOwnSessionPromptInvocationIsStillBlocked
// already covers). Before handleSessionCancel also consulted
// PendingFactorySessionID, this exact scenario was a silent no-op.
func TestServeCancelReachesPendingFactorySessionDuringFirstTurnInvocation(t *testing.T) {
	getSessionResult := sessionAt("session-1", "factory:@you/review", 3, "/work/project")
	// handleSessionCancel resolves the captured identity through its own,
	// separate GetSession call: the episode has not bound FactorySessionID
	// yet (the first turn's InvokeFactorySession/BindFactorySession sequence
	// is still in flight below), only the pending identity StartAsync
	// already returned and RecordPendingFactorySession already durably
	// recorded.
	getSessionResult.Episode = chatsessions.TargetEpisode{
		Number: 1, State: chatsessions.TargetEpisodeStateOpen,
		Target:                  chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
		PendingFactorySessionID: "fs-pending-1",
	}
	chatSessions := &fakeChatSessionsService{
		getSessionResult: getSessionResult,
		// The admitted StartTurn result's episode is genuinely unbound (blank
		// FactorySessionID), driving dispatchFactoryTurn into
		// startFactorySessionForEpisode -- the first-turn branch this test
		// means to exercise.
		startTurnResult: admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult:   factorysessions.AsyncStartResult{SessionID: "fs-pending-1"},
		invokeEnter:   make(chan struct{}),
		invokeRelease: make(chan struct{}),
		cancelEntered: make(chan struct{}),
		invokeResult:  factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCanceled},
	}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(context.Background(), pr, out) }()

	promptLine := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":%s}`+"\n",
		promptTextParams("session-1", "a message to cancel"))
	if _, err := pw.Write([]byte(promptLine)); err != nil {
		t.Fatalf("write session/prompt line: %v", err)
	}

	select {
	case <-factoryTarget.invokeEnter:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for InvokeFactorySession to be entered")
	}

	factoryTarget.mu.Lock()
	startCallCount := len(factoryTarget.startCalls)
	factoryTarget.mu.Unlock()
	if startCallCount != 1 {
		t.Fatalf("StartAsync call count = %d, want exactly 1 before its own follow-up invoke blocks", startCallCount)
	}

	cancelLine := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"session-1"}}` + "\n"
	if _, err := pw.Write([]byte(cancelLine)); err != nil {
		t.Fatalf("write session/cancel line: %v", err)
	}

	select {
	case <-factoryTarget.cancelEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Cancel to be entered")
	}

	// Cancel has already been recorded here, against the pending (not yet
	// bound) identity, while InvokeFactorySession -- confirmed still blocked
	// by this same assertion -- has not returned.
	factoryTarget.mu.Lock()
	cancelCallCount := len(factoryTarget.cancelCalls)
	cancelSessionID := ""
	if cancelCallCount > 0 {
		cancelSessionID = factoryTarget.cancelCalls[0].sessionID
	}
	invokeCallCount := len(factoryTarget.invokeCalls)
	factoryTarget.mu.Unlock()
	if cancelCallCount != 1 {
		t.Fatalf("Cancel call count = %d, want exactly 1 while the first turn's invocation is still blocked", cancelCallCount)
	}
	if cancelSessionID != "fs-pending-1" {
		t.Fatalf("Cancel sessionID = %q, want the episode's pending (not yet bound) identity fs-pending-1", cancelSessionID)
	}
	if invokeCallCount != 1 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 1 still in flight", invokeCallCount)
	}

	close(factoryTarget.invokeRelease)

	if err := pw.Close(); err != nil {
		t.Fatalf("close input pipe: %v", err)
	}
	if err := <-serveErr; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	assertCancelledPromptResponse(t, out)
}

// TestServeSessionCancelWithNoBoundFactorySessionIsNoOp proves a
// "session/cancel" notification for a Chat Session whose current episode has
// not yet bound a Factory Session identity (an episode still pending or
// never started) makes no Cancel call: there is no captured runtime to
// forward to, and this is a silent no-op rather than a panic or a fabricated
// call against a blank identity.
func TestServeSessionCancelWithNoBoundFactorySessionIsNoOp(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: chatsessions.GetSessionResult{
			Session: chatsessions.Session{ID: "session-1"},
			Episode: chatsessions.TargetEpisode{
				Number: 1, State: chatsessions.TargetEpisodeStateOpen,
				Target: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/review"},
			},
		},
	}
	factoryTarget := &fakeFactoryTargetService{}
	server := newTestServerWithFactoryTarget(chatSessions, nil, factoryTarget, "/home/operator")

	line := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"session-1"}}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no response for a session/cancel notification", out.Bytes())
	}
	if len(factoryTarget.cancelCalls) != 0 {
		t.Fatalf("Cancel call count = %d, want 0 when the episode has no bound Factory Session identity", len(factoryTarget.cancelCalls))
	}
}

// TestServeSessionCancelUnknownSessionIsNoOp proves a "session/cancel"
// notification addressed to an unknown Chat Session makes no Cancel call
// rather than panicking or forwarding a blank identity.
func TestServeSessionCancelUnknownSessionIsNoOp(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionErr: &chatsessions.NotFoundError{Value: "Session", ID: "session-1"}}
	factoryTarget := &fakeFactoryTargetService{}
	server := newTestServerWithFactoryTarget(chatSessions, nil, factoryTarget, "/home/operator")

	line := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"session-1"}}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no response for a session/cancel notification", out.Bytes())
	}
	if len(factoryTarget.cancelCalls) != 0 {
		t.Fatalf("Cancel call count = %d, want 0 for an unknown Chat Session", len(factoryTarget.cancelCalls))
	}
}

// TestServeSessionCancelMalformedParamsIsNoOp proves a "session/cancel"
// notification whose params cannot be unmarshaled into
// acpsdk.CancelNotification makes no GetSession or Cancel call rather than
// panicking or writing an error response for what is a notification with no
// response channel at all.
func TestServeSessionCancelMalformedParamsIsNoOp(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	factoryTarget := &fakeFactoryTargetService{}
	server := newTestServerWithFactoryTarget(chatSessions, nil, factoryTarget, "/home/operator")

	line := `{"jsonrpc":"2.0","method":"session/cancel","params":"not-an-object"}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no response for a session/cancel notification", out.Bytes())
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no lookup for malformed session/cancel params")
	}
	if len(factoryTarget.cancelCalls) != 0 {
		t.Fatalf("Cancel call count = %d, want 0 for malformed session/cancel params", len(factoryTarget.cancelCalls))
	}
}

// TestServeSessionCancelBlankSessionIDIsNoOp proves a "session/cancel"
// notification with a blank sessionId fails L1 V0 validation and makes no
// GetSession or Cancel call.
func TestServeSessionCancelBlankSessionIDIsNoOp(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	factoryTarget := &fakeFactoryTargetService{}
	server := newTestServerWithFactoryTarget(chatSessions, nil, factoryTarget, "/home/operator")

	line := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":""}}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no response for a session/cancel notification", out.Bytes())
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no lookup for a blank sessionId")
	}
	if len(factoryTarget.cancelCalls) != 0 {
		t.Fatalf("Cancel call count = %d, want 0 for a blank sessionId", len(factoryTarget.cancelCalls))
	}
}

// TestHandleSessionCancelDependencyFailureLeavesTurnRunningAndRetryable
// proves a Factory Sessions failure neither fabricates a cancelled turn nor
// resolves the captured intent. Retrying the same notification identity can
// later complete the original control without reaching another target.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestHandleSessionCancelDependencyFailureLeavesTurnRunningAndRetryable(t *testing.T) {
	base, session, turn := newActiveBoundControlSession(t, "fs-cancel-failure")
	chatSessions := &controlRecordingChatSessions{Service: base}
	factoryTarget := &fakeFactoryTargetService{cancelErr: errors.New("provider secret at /unsafe/path")}
	server := New(nil, chatSessions, nil, factoryTarget, nil, nil, nil, nil)
	env := cancelNotificationEnvelope(t, "cancel-failure-1", session.ID)

	server.handleSessionCancel(context.Background(), env)
	current, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after failed cancel: %v", err)
	}
	if current.Session.ActiveTurnID != turn.ID {
		t.Fatalf("failed cancel cleared active turn: got %q, want %q", current.Session.ActiveTurnID, turn.ID)
	}
	_, err = base.StartTurn(context.Background(), chatsessions.StartTurnRequest{
		RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "cancel-failure", JSONRPCNumberID: "2"},
		SessionID:       session.ID,
		ExpectedVersion: current.Session.Version,
	})
	var busyErr *chatsessions.BusyError
	if !errors.As(err, &busyErr) || busyErr.ActiveTurnID != turn.ID || busyErr.ActiveTurnState != chatsessions.TurnStateRunning {
		t.Fatalf("StartTurn after failed cancel error = %v, want running captured turn to remain active", err)
	}
	_, advances := chatSessions.snapshotControls()
	if len(advances) != 1 || advances[0].Intent.State != chatsessions.ControlIntentStateCommitted {
		t.Fatalf("control advances = %#v, want only COMMITTED after failed Factory cancel", advances)
	}

	factoryTarget.mu.Lock()
	factoryTarget.cancelErr = nil
	factoryTarget.mu.Unlock()
	server.handleSessionCancel(context.Background(), env)
	factoryTarget.mu.Lock()
	cancelCalls := append([]cancelFactoryTargetCall(nil), factoryTarget.cancelCalls...)
	factoryTarget.mu.Unlock()
	if len(cancelCalls) != 2 || cancelCalls[0].sessionID != "fs-cancel-failure" || cancelCalls[1].sessionID != "fs-cancel-failure" {
		t.Fatalf("Cancel calls = %#v, want retry of only the captured target", cancelCalls)
	}
	_, advances = chatSessions.snapshotControls()
	if len(advances) != 2 || advances[1].Intent.State != chatsessions.ControlIntentStateCompleted {
		t.Fatalf("control advances = %#v, want COMMITTED then COMPLETED after retry", advances)
	}
	retried, err := base.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after retried cancel: %v", err)
	}
	if retried.Session.State != chatsessions.SessionStateActive || retried.Session.ActiveTurnID != turn.ID {
		t.Fatalf("retried cancel fabricated terminal Chat state: Session=%#v, want the still-running captured prompt to remain authoritative until its own result arrives", retried.Session)
	}
}

// serveUntilAsyncWriteFailureWithReaderStillOpen drives a Server through
// exactly the sequence TestServeAsyncWriteFailureStopsAllFurtherConnectionActivity's
// subtests need: one "session/prompt" is admitted and invoked against a
// captured Factory Session bound to "fs-already-bound", its response write
// fails, and Serve returns that failure -- all proven through real events
// (the buffered done receive), never a sleep -- while the input side of the
// pipe was, until that receive, still open and unread-from, exactly the
// "output already broken, input still open" case serveConnection's own doc
// comment describes. By the time done has received, serveConnection has
// already called closeInputForShutdown and joined its read loop (see
// serveConnection's own doc comment), so pr -- the *io.PipeReader Serve was
// given -- is already closed. The returned pw is still a live
// *io.PipeWriter; every subtest below writes to it to prove that closure,
// not a sleep, is what the read loop's own termination leaves observable.
func serveUntilAsyncWriteFailureWithReaderStillOpen(t *testing.T) (chatSessions *fakeChatSessionsService, factoryTarget *fakeFactoryTargetService, pw *io.PipeWriter, writer *countingErrorWriter) {
	t.Helper()

	getSessionResult := sessionAt("session-1", "factory:@you/review", 3, "/work/project")
	// handleSessionCancel resolves the bound Factory Session identity through
	// its own, separate GetSession call, so this fake's Episode must carry
	// the same FactorySessionID startTurnResult's Episode below does.
	getSessionResult.Episode = chatsessions.TargetEpisode{
		Number: 1, State: chatsessions.TargetEpisodeStateOpen,
		Target:           chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
		FactorySessionID: "fs-already-bound",
	}
	chatSessions = &fakeChatSessionsService{
		getSessionResult: getSessionResult,
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget = &fakeFactoryTargetService{
		invokeResult:  factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted},
		cancelEntered: make(chan struct{}),
	}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	wantErr := errors.New("acp test: simulated async write failure")
	writer = &countingErrorWriter{err: wantErr}

	var pr *io.PipeReader
	pr, pw = io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), pr, writer) }()

	promptLine := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":%s}`+"\n",
		promptTextParams("session-1", "a message whose response write fails"))
	if _, err := pw.Write([]byte(promptLine)); err != nil {
		t.Fatalf("write session/prompt line: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, wantErr) {
			t.Fatalf("Serve() error = %v, want %v", err, wantErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the dispatched prompt's response write failed")
	}
	if writer.calls != 1 {
		t.Fatalf("writer was called %d times before Serve returned, want exactly 1", writer.calls)
	}
	return chatSessions, factoryTarget, pw, writer
}

// writeAfterServeReturned writes line to pw from its own goroutine and
// returns the write's outcome, bounded by a generous hang-safety timeout that
// is not itself the assertion: on a correctly joined connection, pr is
// already closed by the time this is called, so io.Pipe's own synchronous
// contract makes the write return io.ErrClosedPipe immediately, with nothing
// to wait for. The timeout only guards against the read loop having been
// left alive and blocked reading (in which case the write would hang,
// waiting for a reader that will never come), which this helper reports as a
// test failure rather than a false pass.
func writeAfterServeReturned(t *testing.T, pw *io.PipeWriter, line string) error {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		_, err := pw.Write([]byte(line))
		result <- err
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("write after Serve returned did not complete -- the read loop goroutine may still be alive and blocked reading, not joined by Serve's return")
		return nil
	}
}

// TestServeAsyncWriteFailureStopsAllFurtherConnectionActivity proves that
// once Serve has returned because a dispatched "session/prompt" response
// write failed, its read loop goroutine has already fully terminated -- not
// merely been gated from acting -- so later input written to the same
// connection stream can never reach a handler: no "session/cancel" forward,
// no ordinary request effect (a "session/new" is used as the representative
// case), and no new "session/prompt" dispatch. serveConnection's own doc
// comment describes why: a dispatched write failure makes serveConnection
// call closeInputForShutdown and join its read loop before returning, so pr
// is already closed by the time each subtest below writes to pw. io.Pipe
// makes that closure observable without any sleep or timing window: a write
// to a PipeWriter whose PipeReader is closed returns io.ErrClosedPipe
// immediately, precisely because there is no live reader goroutine left to
// ever consume it.
func TestServeAsyncWriteFailureStopsAllFurtherConnectionActivity(t *testing.T) {
	t.Run("SessionCancelIsNotForwarded", func(t *testing.T) {
		_, factoryTarget, pw, writer := serveUntilAsyncWriteFailureWithReaderStillOpen(t)

		cancelLine := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"session-1"}}` + "\n"
		if err := writeAfterServeReturned(t, pw, cancelLine); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write session/cancel line after Serve returned: err = %v, want io.ErrClosedPipe (the read loop must already be joined, not merely gated)", err)
		}

		factoryTarget.mu.Lock()
		cancelCallCount := len(factoryTarget.cancelCalls)
		factoryTarget.mu.Unlock()
		if cancelCallCount != 0 {
			t.Fatalf("Cancel call count = %d, want 0 for a session/cancel notification that could never be delivered after Serve returned", cancelCallCount)
		}
		if writer.calls != 1 {
			t.Fatalf("writer was called %d times after later input arrived, want still exactly 1 (no new response write)", writer.calls)
		}
	})

	t.Run("SessionPromptIsNotDispatched", func(t *testing.T) {
		_, factoryTarget, pw, writer := serveUntilAsyncWriteFailureWithReaderStillOpen(t)

		anotherPromptLine := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":%s}`+"\n",
			promptTextParams("session-1", "a second message sent after Serve returned"))
		if err := writeAfterServeReturned(t, pw, anotherPromptLine); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write a second session/prompt line after Serve returned: err = %v, want io.ErrClosedPipe (the read loop must already be joined, not merely gated)", err)
		}

		factoryTarget.mu.Lock()
		invokeCallCount := len(factoryTarget.invokeCalls)
		factoryTarget.mu.Unlock()
		if invokeCallCount != 1 {
			t.Fatalf("InvokeFactorySession call count = %d, want exactly the 1 call made before Serve returned, no dispatch for a second session/prompt line that could never be delivered", invokeCallCount)
		}
		if writer.calls != 1 {
			t.Fatalf("writer was called %d times after later input arrived, want still exactly 1 (no new response write)", writer.calls)
		}
	})

	t.Run("OrdinaryRequestIsNotDispatched", func(t *testing.T) {
		chatSessions, _, pw, writer := serveUntilAsyncWriteFailureWithReaderStillOpen(t)

		sessionNewLine := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"session/new","params":%s}`+"\n", validSessionNewParams)
		if err := writeAfterServeReturned(t, pw, sessionNewLine); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write a session/new line after Serve returned: err = %v, want io.ErrClosedPipe (the read loop must already be joined, not merely gated)", err)
		}

		chatSessions.mu.Lock()
		createCalled := chatSessions.createCalled
		chatSessions.mu.Unlock()
		if createCalled {
			t.Fatal("CreateSession was called for a session/new request that could never be delivered after Serve had already returned")
		}
		if writer.calls != 1 {
			t.Fatalf("writer was called %d times after later input arrived, want still exactly 1 (no new response write)", writer.calls)
		}
	})
}
