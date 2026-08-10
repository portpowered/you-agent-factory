package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// TestHandleSessionPromptFirstTurnStartsFactorySessionWithExactTargetRootAndContent
// proves the first admitted turn in an unbound episode calls
// StartAsync exactly once through the Factory Sessions-owned capability, with the
// episode's canonical Factory target, the session's exact editor working
// root, and the validated prompt content -- never a process cwd or
// substituted value.
// TestHandleSessionPromptFirstTurnStartsFactorySessionWithExactTargetRootAndContent
// proves the first admitted turn in an unbound episode starts a Factory
// Session with the episode's canonical target and the session's exact
// editor working root, then dispatches this turn's validated content into
// the returned identity via the immediate follow-up InvokeFactorySession call
// -- StartAsync carries no content of its own, since the shared
// factorysessions.Service.StartAsync it forwards to has no dedicated content
// field and cannot dispatch an ordinary packaged Factory at all (see
// ondemandtarget.Service.StartAsync's own doc comment).
func TestHandleSessionPromptFirstTurnStartsFactorySessionWithExactTargetRootAndContent(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startResult: factorysessions.AsyncStartResult{SessionID: "fs-1"}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want a successful final prompt response", rpcErr)
	}

	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count = %d, want exactly 1", len(factoryTarget.startCalls))
	}
	got := factoryTarget.startCalls[0]
	wantSource := factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "factory:@you/review"}
	if !reflect.DeepEqual(got.Source, wantSource) {
		t.Fatalf("StartAsync Source = %+v, want %+v", got.Source, wantSource)
	}
	if got.Args["workingRoot"] != "/work/project" {
		t.Fatalf("StartAsync Args[workingRoot] = %v, want /work/project", got.Args["workingRoot"])
	}
	if _, ok := got.Args["content"]; ok {
		t.Fatalf("StartAsync Args[content] = %#v, want no content key at all", got.Args["content"])
	}
	if got.RequestID != "session-1/episode/1" {
		t.Fatalf("StartAsync RequestID = %q, want the stable per-episode key session-1/episode/1 (not the admitted turn id, which changes on every retry)", got.RequestID)
	}

	if len(factoryTarget.invokeCalls) != 1 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 1", len(factoryTarget.invokeCalls))
	}
	invoked := factoryTarget.invokeCalls[0]
	if invoked.sessionID != "fs-1" {
		t.Fatalf("InvokeFactorySession sessionID = %q, want the identity StartAsync just returned", invoked.sessionID)
	}
	wantContent := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello there"}}
	if !reflect.DeepEqual(invoked.request.Content, wantContent) {
		t.Fatalf("InvokeFactorySession request.Content = %#v, want %#v", invoked.request.Content, wantContent)
	}
	if invoked.request.RequestID == nil || *invoked.request.RequestID != "turn-1" {
		t.Fatalf("InvokeFactorySession request.RequestID = %v, want the admitted turn id turn-1", invoked.request.RequestID)
	}
}

// TestHandleSessionPromptFirstTurnBindsReturnedFactorySessionID proves a
// successful Factory Session start binds the returned identity onto exactly
// the admitted session/episode/turn/version.
func TestHandleSessionPromptFirstTurnBindsReturnedFactorySessionID(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		// Two queued GetSession results, consumed front-first: the first
		// (admission-time) call observes version 3, and the second --
		// currentSessionVersion's fresh re-read after
		// dispatchFactoryInvocation returns (see its own doc comment) --
		// observes StartTurn's already-admitted version (4), not the stale
		// admission-time snapshot the first call returned.
		getSessionResults: []chatsessions.GetSessionResult{
			sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
			sessionAt("session-1", "factory:@you/review", 4, "/work/project"),
		},
		startTurnResult: admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startResult: factorysessions.AsyncStartResult{SessionID: "fs-1"}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want a successful final prompt response", rpcErr)
	}

	if !chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was not called, want exactly one binding attempt after a successful start")
	}
	want := chatsessions.BindFactorySessionRequest{
		SessionID: "session-1", ExpectedVersion: 4, Episode: 1, TurnID: "turn-1", FactorySessionID: "fs-1",
	}
	if chatSessions.bindFactorySessionReq != want {
		t.Fatalf("BindFactorySession request = %+v, want %+v", chatSessions.bindFactorySessionReq, want)
	}
}

// TestHandleSessionPromptLaterTurnInvokesBoundFactorySessionExactlyOnce
// proves a later turn in an episode that already carries a Factory Session
// ID calls InvokeFactorySession exactly once with that exact bound identity,
// the validated prompt content, the text source kind, and the admitted
// turn's ID as the correlated request ID -- and makes zero
// StartAsync or BindFactorySession calls, since a second Factory
// Session must never be started for an already-started episode.
func TestHandleSessionPromptLaterTurnInvokesBoundFactorySessionExactlyOnce(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startResult: factorysessions.AsyncStartResult{SessionID: "fs-new"}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "a later message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want a successful final prompt response", rpcErr)
	}

	if len(factoryTarget.startCalls) != 0 {
		t.Fatalf("StartAsync call count = %d, want 0 for an already-bound episode", len(factoryTarget.startCalls))
	}
	if chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was called, want no binding attempt for an already-bound episode")
	}

	if len(factoryTarget.invokeCalls) != 1 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 1", len(factoryTarget.invokeCalls))
	}
	got := factoryTarget.invokeCalls[0]
	if got.sessionID != "fs-already-bound" {
		t.Fatalf("InvokeFactorySession sessionID = %q, want the bound identity fs-already-bound", got.sessionID)
	}
	wantContent := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "a later message"}}
	if !reflect.DeepEqual(got.request.Content, wantContent) {
		t.Fatalf("InvokeFactorySession request.Content = %#v, want %#v", got.request.Content, wantContent)
	}
	if !got.request.ContentProvided {
		t.Fatal("InvokeFactorySession request.ContentProvided = false, want true")
	}
	if got.request.SourceKind == nil || *got.request.SourceKind != factorysessions.InvocationInputSourceKindText {
		t.Fatalf("InvokeFactorySession request.SourceKind = %v, want %q", got.request.SourceKind, factorysessions.InvocationInputSourceKindText)
	}
	if got.request.RequestID == nil || *got.request.RequestID != "turn-2" {
		t.Fatalf("InvokeFactorySession request.RequestID = %v, want the admitted turn id turn-2", got.request.RequestID)
	}
}

// TestHandleSessionPromptLaterTurnCallsResponseBridgeAroundInvokeFactorySession
// proves dispatchFactoryInvocation actually reaches an injected
// acp.ResponseBridge collaborator (rather than calling InvokeFactorySession
// directly) with the exact Chat Session identity, session version, and bound
// Factory Session identity this turn dispatches against, and that the
// bridge's own invoke callback -- not some independent path -- is what
// performs the real InvokeFactorySession call whose result becomes the final
// prompt response.
func TestHandleSessionPromptLaterTurnCallsResponseBridgeAroundInvokeFactorySession(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{invokeResult: fallbackInvokeResult("bridged response")}

	var bridgeCalls int
	var gotChatSessionID string
	var gotSessionVersion uint64
	var gotFactorySessionID string
	responseBridge := func(
		ctx context.Context,
		chatSessionID string,
		sessionVersion uint64,
		factorySessionID string,
		_ func(context.Context),
		invoke func(context.Context) (factorysessions.InvocationResult, error),
	) (factorysessions.InvocationResult, error) {
		bridgeCalls++
		gotChatSessionID = chatSessionID
		gotSessionVersion = sessionVersion
		gotFactorySessionID = factorySessionID
		return invoke(ctx)
	}

	server := newTestServerWithResponseBridge(chatSessions, catalog, factoryTarget, "/home/operator", responseBridge)

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "a later message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want a successful final prompt response", rpcErr)
	}

	if bridgeCalls != 1 {
		t.Fatalf("responseBridge called %d times, want exactly 1", bridgeCalls)
	}
	if gotChatSessionID != "session-1" {
		t.Errorf("responseBridge chatSessionID = %q, want %q", gotChatSessionID, "session-1")
	}
	if gotSessionVersion != 4 {
		t.Errorf("responseBridge sessionVersion = %d, want the admitted turn's own startResult.Session.Version 4", gotSessionVersion)
	}
	if gotFactorySessionID != "fs-already-bound" {
		t.Errorf("responseBridge factorySessionID = %q, want the bound identity fs-already-bound", gotFactorySessionID)
	}
	if len(factoryTarget.invokeCalls) != 1 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 1 (reached through the bridge's own invoke callback)", len(factoryTarget.invokeCalls))
	}
}

// TestHandleSessionPromptInvokeFactorySessionFailureMakesNoStartCall proves an
// InvokeFactorySession failure for a later turn reports a bounded failure and
// never falls back to starting a second Factory Session for the episode.
func TestHandleSessionPromptInvokeFactorySessionFailureMakesNoStartCall(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{invokeErr: errors.New("factory sessions boom")}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "a later message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure for an Invoke failure")
	}

	if len(factoryTarget.invokeCalls) != 1 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 1", len(factoryTarget.invokeCalls))
	}
	if len(factoryTarget.startCalls) != 0 {
		t.Fatalf("StartAsync call count = %d, want 0 after an Invoke failure", len(factoryTarget.startCalls))
	}
	if chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was called, want no binding attempt for an already-bound episode")
	}
}

// TestHandleSessionPromptInvokeMissingSessionFailureAdvancesTurnToFailed
// proves a later turn's invoke against an unknown or evicted Factory
// Session identity -- classified by the Factory Sessions-owned capability as
// the existing factorysessions.ErrSessionNotFound sentinel, not a generic
// dependency error -- still reports a bounded failure, still advances the
// admitted turn to TurnStateFailed (not silently stranded, not
// misclassified as canceled), and never falls back to starting a second
// Factory Session for the episode. classifyDependencyFailure collapses every
// non-context-cancellation dependency error to the same bounded internal
// error at this transport boundary by design, so this intentionally does not
// assert errors.Is on the RPC response itself -- see
// ondemandtarget.Service's own tests for the sentinel's errors.Is-compatible
// classification at the capability that actually produces it.
func TestHandleSessionPromptInvokeMissingSessionFailureAdvancesTurnToFailed(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-evicted"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{invokeErr: factorysessions.ErrSessionNotFound}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "a later message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure for a missing-session Invoke failure")
	}

	if len(factoryTarget.invokeCalls) != 1 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 1", len(factoryTarget.invokeCalls))
	}
	if len(factoryTarget.startCalls) != 0 {
		t.Fatalf("StartAsync call count = %d, want 0 after a missing-session Invoke failure", len(factoryTarget.startCalls))
	}
	if chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was called, want no binding attempt for an already-bound episode")
	}

	var terminalAdvance *chatsessions.AdvanceTurnRequest
	for i := range chatSessions.advanceTurnReqs {
		if chatSessions.advanceTurnReqs[i].Next != chatsessions.TurnStateRunning {
			terminalAdvance = &chatSessions.advanceTurnReqs[i]
			break
		}
	}
	if terminalAdvance == nil {
		t.Fatal("no terminal AdvanceTurn call observed, want exactly one terminal transition after the missing-session failure")
	}
	if terminalAdvance.Next != chatsessions.TurnStateFailed {
		t.Fatalf("terminal AdvanceTurn Next = %v, want TurnStateFailed for a non-cancellation dependency failure", terminalAdvance.Next)
	}
}

// TestHandleSessionPromptLaterTurnMapsInvocationOutcomeToFinalStopReason
// proves handleSessionPrompt's final "session/prompt" response for a later
// (invoke) turn carries only the deterministic ACP stop reason
// protocol.MapFactoryInvocationOutcome derives from the published
// InvocationResult terminal status -- completed, canceled, timed out, failed,
// and an unmapped future status all included -- and never a fabricated or
// raw result field.
func TestHandleSessionPromptLaterTurnMapsInvocationOutcomeToFinalStopReason(t *testing.T) {
	tests := []struct {
		name   string
		status factorysessions.InvocationTerminalStatus
		want   acpsdk.StopReason
	}{
		{"completed", factorysessions.InvocationTerminalStatusCompleted, acpsdk.StopReasonEndTurn},
		{"canceled", factorysessions.InvocationTerminalStatusCanceled, acpsdk.StopReasonCancelled},
		{"timed_out", factorysessions.InvocationTerminalStatusTimedOut, acpsdk.StopReasonCancelled},
		{"failed", factorysessions.InvocationTerminalStatusFailed, acpsdk.StopReasonEndTurn},
		{"unmapped_future_status", factorysessions.InvocationTerminalStatus("SOME_FUTURE_STATUS"), acpsdk.StopReasonEndTurn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chatSessions := &fakeChatSessionsService{
				getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
				startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
			}
			catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
			factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{
				Status: tc.status,
				Message: "internal detail that must never reach the client: " +
					"provider command /usr/local/bin/agent --token=sk-live-ABC123",
			}}
			server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "a later message"))
			result, rpcErr := server.handleSessionPrompt(context.Background(), env)
			wantPromptFailureForStatus(t, tc.status, rpcErr)
			if tc.status == factorysessions.InvocationTerminalStatusFailed {
				return
			}

			var resp acpsdk.PromptResponse
			if err := json.Unmarshal(result, &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.StopReason != tc.want {
				t.Fatalf("stopReason = %q, want %q", resp.StopReason, tc.want)
			}
			if strings.Contains(string(result), "sk-live-ABC123") {
				t.Fatalf("response leaked the raw invocation message: %s", result)
			}
		})
	}
}

// TestHandleSessionPromptFirstTurnResponseNeverLeaksFactorySessionIdentity
// proves handleSessionPrompt's final response for the first (start) turn in
// an episode is mapped from the follow-up InvokeFactorySession call's real
// published outcome -- not the start call's own identity-only
// AsyncStartResult, which carries no terminal status at all -- and never
// leaks the returned Factory Session identity or any other raw invocation
// field (message, error code) into the response.
func TestHandleSessionPromptFirstTurnResponseNeverLeaksFactorySessionIdentity(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult: factorysessions.AsyncStartResult{SessionID: "fs-secret-1"},
		invokeResult: factorysessions.InvocationResult{
			SessionID: "fs-secret-1",
			Status:    factorysessions.InvocationTerminalStatusCompleted,
			Message:   "internal diagnostic detail",
		},
	}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	var resp acpsdk.PromptResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want end_turn from the real invoke outcome", resp.StopReason)
	}
	if strings.Contains(string(result), "fs-secret-1") {
		t.Fatalf("response leaked the raw Factory Session identity: %s", result)
	}
	if strings.Contains(string(result), "internal diagnostic detail") {
		t.Fatalf("response leaked the raw invocation message: %s", result)
	}
}

// TestHandleSessionPromptFinalResponseCarriesOnlyStopReason proves the
// handler's final "session/prompt" response for an admitted, mapped turn is
// exactly the closed final-only shape this transport slice supports: a stop
// reason and nothing else -- no fabricated content, chunk, or progress
// field.
func TestHandleSessionPromptFinalResponseCarriesOnlyStopReason(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "the answer"},
		},
	}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "a later message"))
	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := decoded["stopReason"]; !ok {
		t.Fatalf("response = %s, want a stopReason field", result)
	}
	delete(decoded, "stopReason")
	if len(decoded) != 0 {
		t.Fatalf("response carried unexpected fields %v, want only stopReason", decoded)
	}
}

// TestHandleSessionPromptDeliversMappedTextAsOneNotification proves a
// successful dispatch whose mapped outcome carries text sends it through
// exactly one promptNotifier call, newline-joined in stable order, addressed
// to the admitted turn's own session, before the final response is built.
func TestHandleSessionPromptDeliversMappedTextAsOneNotification(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "hello"},
			{Type: work.WorkContentPartTypeText, Text: "world"},
		},
	}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}
	ctx := contextWithPromptNotifier(context.Background(), notify)
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "a later message"))

	if _, rpcErr := server.handleSessionPrompt(ctx, env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	if len(notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1", len(notified))
	}
	if notified[0].SessionId != "session-1" {
		t.Fatalf("notification SessionId = %q, want session-1", notified[0].SessionId)
	}
	chunk := notified[0].Update.AgentMessageChunk
	if chunk == nil {
		t.Fatal("notification Update.AgentMessageChunk = nil, want a populated chunk")
	}
	if chunk.Content.Text == nil || chunk.Content.Text.Text != "hello\nworld" {
		t.Fatalf("notification chunk text = %+v, want \"hello\\nworld\"", chunk.Content.Text)
	}
}

// TestHandleSessionPromptEmptyOutcomeTextSendsNoNotification proves a
// dispatch whose mapped outcome carries no text (an absent or
// unsupported-only primary result) never calls the attached promptNotifier.
func TestHandleSessionPromptEmptyOutcomeTextSendsNoNotification(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
	}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	notifyCalls := 0
	notify := func(acpsdk.SessionNotification) error {
		notifyCalls++
		return nil
	}
	ctx := contextWithPromptNotifier(context.Background(), notify)
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "a later message"))

	if _, rpcErr := server.handleSessionPrompt(ctx, env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}
	if notifyCalls != 0 {
		t.Fatalf("notify call count = %d, want 0 for an outcome with no text", notifyCalls)
	}
}

// TestHandleSessionPromptNotifyFailureTerminalizesToFailed proves a
// promptNotifier failure is treated like any other post-dispatch failure: it
// reports a bounded internal error and terminalizes the turn to FAILED
// instead of silently discarding the delivery failure and returning the
// dispatch's own successful response.
func TestHandleSessionPromptNotifyFailureTerminalizesToFailed(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "hello"},
		},
	}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	notify := func(acpsdk.SessionNotification) error {
		return errors.New("write pipe closed")
	}
	ctx := contextWithPromptNotifier(context.Background(), notify)
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "a later message"))

	result, rpcErr := server.handleSessionPrompt(ctx, env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure when notify fails")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil when notify fails", result)
	}

	wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-2", chatsessions.TurnStateRunning, chatsessions.TurnStateFailed)
}

// TestServeDeliversMappedTextAsOneSessionUpdateNotificationBeforeTheFinalResponse
// proves the end-to-end wiring through the real stdio.Server.Serve loop: a
// successful "session/prompt" JSON-RPC request whose mapped outcome carries
// text writes exactly one "session/update" notification line -- no "id"
// member, method "session/update", an agent_message_chunk update carrying the
// joined text -- followed by exactly one final response line, never
// interleaved and never reversed.
func TestServeDeliversMappedTextAsOneSessionUpdateNotificationBeforeTheFinalResponse(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "hello"},
			{Type: work.WorkContentPartTypeText, Text: "world"},
		},
	}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":%s}`+"\n",
		promptTextParams("session-1", "a later message"))
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(line), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	lines := nonEmptyResponseLines(t, out)
	if len(lines) != 2 {
		t.Fatalf("output line count = %d, want 2 (one notification, one response): %s", len(lines), out.String())
	}

	var notification map[string]json.RawMessage
	if err := json.Unmarshal(lines[0], &notification); err != nil {
		t.Fatalf("unmarshal notification line: %v", err)
	}
	if _, hasID := notification["id"]; hasID {
		t.Fatalf("notification line carried an \"id\" member, want a true JSON-RPC notification: %s", lines[0])
	}
	var method string
	if err := json.Unmarshal(notification["method"], &method); err != nil || method != acpsdk.ClientMethodSessionUpdate {
		t.Fatalf("notification method = %s, want %q", notification["method"], acpsdk.ClientMethodSessionUpdate)
	}
	if !bytes.Contains(lines[0], []byte(`"sessionUpdate":"agent_message_chunk"`)) {
		t.Fatalf("notification = %s, want an agent_message_chunk update", lines[0])
	}
	if !bytes.Contains(lines[0], []byte(`"text":"hello\nworld"`)) {
		t.Fatalf("notification = %s, want the newline-joined mapped text", lines[0])
	}
	if !bytes.Contains(lines[0], []byte(`"sessionId":"session-1"`)) {
		t.Fatalf("notification = %s, want sessionId session-1", lines[0])
	}

	var resp rpcMessage
	if err := json.Unmarshal(lines[1], &resp); err != nil {
		t.Fatalf("unmarshal response line: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("response error = %+v, want success", resp.Error)
	}
	var promptResp acpsdk.PromptResponse
	if err := json.Unmarshal(resp.Result, &promptResp); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if promptResp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want end_turn", promptResp.StopReason)
	}
}
