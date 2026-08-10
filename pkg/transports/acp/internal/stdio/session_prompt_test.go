// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionswire "github.com/portpowered/infinite-you/pkg/services/chat_sessions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// firstCallFailingChatSessions wraps a real chatsessions.Service and injects
// injectErr into exactly the first call to the named method, letting every
// other call (including every later call to that same method) pass through
// unmodified. It exists to prove recovery behavior against the real
// chatsessions.Store's own state machine, not a call-recording fake that
// cannot itself strand or release busy state.
type firstCallFailingChatSessions struct {
	chatsessions.Service
	mu        sync.Mutex
	method    string
	injectErr error
	triggered bool
}

func (f *firstCallFailingChatSessions) AdvanceTurn(ctx context.Context, req chatsessions.AdvanceTurnRequest) (chatsessions.AdvanceTurnResult, error) {
	if f.method == "AdvanceTurn" {
		f.mu.Lock()
		if !f.triggered {
			f.triggered = true
			f.mu.Unlock()
			return chatsessions.AdvanceTurnResult{}, f.injectErr
		}
		f.mu.Unlock()
	}
	return f.Service.AdvanceTurn(ctx, req)
}

// nthAdvanceTurnFailingChatSessions wraps a real chatsessions.Service and
// injects injectErr into exactly the failOnCall'th call to AdvanceTurn
// (1-indexed) across the whole wrapped instance's lifetime, letting every
// other call pass through unmodified. It exists to prove that a single
// injected terminal-transition failure -- followed by this transport's own
// recovery attempt -- does not strand the session busy for a later prompt.
type nthAdvanceTurnFailingChatSessions struct {
	chatsessions.Service
	mu         sync.Mutex
	failOnCall int
	injectErr  error
	calls      int
}

func (f *nthAdvanceTurnFailingChatSessions) AdvanceTurn(ctx context.Context, req chatsessions.AdvanceTurnRequest) (chatsessions.AdvanceTurnResult, error) {
	f.mu.Lock()
	f.calls++
	fail := f.calls == f.failOnCall
	f.mu.Unlock()
	if fail {
		return chatsessions.AdvanceTurnResult{}, f.injectErr
	}
	return f.Service.AdvanceTurn(ctx, req)
}

// firstCallFailingBindFactorySession wraps a real chatsessions.Service and
// injects injectErr into exactly the first call to BindFactorySession,
// letting every later call pass through unmodified. It exists to prove that
// the reconciliation record a failed first bind leaves behind
// (Episode.PendingFactorySessionID, durably committed via
// RecordPendingFactorySession) survives being observed by a brand-new
// *Server instance sharing the same underlying chatsessions.Store -- not
// just a later call against the same Server -- since that durability is
// exactly what this finding requires.
type firstCallFailingBindFactorySession struct {
	chatsessions.Service
	mu        sync.Mutex
	injectErr error
	triggered bool
}

func (f *firstCallFailingBindFactorySession) BindFactorySession(ctx context.Context, req chatsessions.BindFactorySessionRequest) (chatsessions.BindFactorySessionResult, error) {
	f.mu.Lock()
	if !f.triggered {
		f.triggered = true
		f.mu.Unlock()
		return chatsessions.BindFactorySessionResult{}, f.injectErr
	}
	f.mu.Unlock()
	return f.Service.BindFactorySession(ctx, req)
}

// admittedTurnResult builds a chatsessions.StartTurnResult matching what a
// real StartTurn admission against sessionAt(id, target, version, root)
// would return: the same Session identity/version/root, a newly admitted
// Turn, and the current TargetEpisode snapshot (whose FactorySessionID is
// factorySessionID -- blank for a brand-new, unbound episode).
func admittedTurnResult(id, target string, version uint64, workingRoot, turnID, factorySessionID string) chatsessions.StartTurnResult {
	targetRef := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: target}
	return chatsessions.StartTurnResult{
		Session: chatsessions.Session{
			ID: id, State: chatsessions.SessionStateActive,
			SelectedTarget: targetRef, TargetEpisode: 1, ActiveTurnID: turnID,
			Version: version, WorkingRoot: workingRoot,
		},
		Turn: chatsessions.Turn{
			ID: turnID, Episode: 1, State: chatsessions.TurnStateAdmitted,
		},
		Episode: chatsessions.TargetEpisode{
			Number: 1, State: chatsessions.TargetEpisodeStateOpen,
			Target: targetRef, FactorySessionID: factorySessionID,
		},
	}
}

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

// TestHandleSessionPromptSequentialTurnsStartThenInvokeExactlyOnce proves the
// full first-turn-starts / later-turn-invokes sequence across two real
// handleSessionPrompt calls against the same session: the first turn's
// unbound episode starts exactly one Factory Session, and the second turn --
// observing that same episode now carrying the returned identity -- invokes
// it exactly once and starts none, without ever mutating the first turn's
// start call.
func TestHandleSessionPromptSequentialTurnsStartThenInvokeExactlyOnce(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResults: []chatsessions.StartTurnResult{
			admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
			admittedTurnResult("session-1", "factory:@you/review", 5, "/work/project", "turn-2", "fs-1"),
		},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startResult: factorysessions.AsyncStartResult{SessionID: "fs-1"}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	firstEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "first message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), firstEnv); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() (first turn) error = %+v, want a successful final prompt response", rpcErr)
	}
	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count after first turn = %d, want exactly 1", len(factoryTarget.startCalls))
	}
	// The first turn's own content dispatches through the immediate
	// follow-up InvokeFactorySession call StartAsync's own identity
	// feeds into -- see startFactorySessionForEpisode's doc comment -- so
	// one invoke call is already expected here, before the second turn.
	if len(factoryTarget.invokeCalls) != 1 {
		t.Fatalf("InvokeFactorySession call count after first turn = %d, want exactly 1 (the first turn's own content dispatch)", len(factoryTarget.invokeCalls))
	}
	if got := factoryTarget.invokeCalls[0].sessionID; got != "fs-1" {
		t.Fatalf("InvokeFactorySession sessionID = %q, want the identity StartAsync just returned (fs-1)", got)
	}

	secondEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 2, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "second message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() (second turn) error = %+v, want a successful final prompt response", rpcErr)
	}
	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count after second turn = %d, want still exactly 1 (no second start)", len(factoryTarget.startCalls))
	}
	if len(factoryTarget.invokeCalls) != 2 {
		t.Fatalf("InvokeFactorySession call count after second turn = %d, want exactly 2 (the first turn's dispatch plus the second turn's own invoke)", len(factoryTarget.invokeCalls))
	}
	if got := factoryTarget.invokeCalls[1].sessionID; got != "fs-1" {
		t.Fatalf("InvokeFactorySession sessionID = %q, want the identity bound by the first turn's start (fs-1)", got)
	}
}

// TestHandleSessionPromptRedeliveredRequestMakesNoFactoryCall proves that
// when StartTurn reports a redelivered RequestID whose originally admitted
// turn has already terminalized (a returned Turn.State other than ADMITTED,
// exactly what chat_sessions/internal/service.Store.StartTurn now returns for
// a reused RequestID), this transport neither calls AdvanceTurn nor dispatches
// any Factory effect, and still returns a deterministic final response
// derived from that turn's own recorded terminal state -- exercising
// turnStateStopReason's full mapping: TurnStateCanceled to the distinct
// cancelled stop reason, and every other terminal state (including
// TurnStateFailed) to the same end_turn safe fallback TurnStateCompleted
// uses.
func TestHandleSessionPromptRedeliveredRequestMakesNoFactoryCall(t *testing.T) {
	tests := []struct {
		name       string
		turnState  chatsessions.TurnState
		wantReason acpsdk.StopReason
	}{
		{"completed", chatsessions.TurnStateCompleted, acpsdk.StopReasonEndTurn},
		{"canceled", chatsessions.TurnStateCanceled, acpsdk.StopReasonCancelled},
		{"failed", chatsessions.TurnStateFailed, acpsdk.StopReasonEndTurn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replayed := admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", "fs-1")
			replayed.Turn.State = tt.turnState
			chatSessions := &fakeChatSessionsService{
				getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
				startTurnResult:  replayed,
			}
			catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
			factoryTarget := &fakeFactoryTargetService{}
			server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "hello there"))
			result, rpcErr := server.handleSessionPrompt(context.Background(), env)
			if rpcErr != nil {
				t.Fatalf("handleSessionPrompt() error = %+v, want a deterministic final response for a redelivered request", rpcErr)
			}
			if chatSessions.advanceTurnCalled {
				t.Fatal("AdvanceTurn was called, want no turn-state mutation for a redelivered already-terminal request")
			}
			if len(factoryTarget.startCalls) != 0 {
				t.Fatalf("StartAsync call count = %d, want 0 for a redelivered request", len(factoryTarget.startCalls))
			}
			if len(factoryTarget.invokeCalls) != 0 {
				t.Fatalf("InvokeFactorySession call count = %d, want 0 for a redelivered request", len(factoryTarget.invokeCalls))
			}

			var resp acpsdk.PromptResponse
			if err := json.Unmarshal(result, &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.StopReason != tt.wantReason {
				t.Fatalf("StopReason = %q, want %q for the original %s turn", resp.StopReason, tt.wantReason, tt.turnState)
			}
		})
	}
}

// TestHandleSessionPromptRedeliveredBusyRequestRejectsAsBusy proves that when
// StartTurn reports a redelivered RequestID whose originally admitted turn is
// still busy (RUNNING), this transport rejects the request the same way a
// genuinely distinct concurrent duplicate would, with zero Factory effect.
func TestHandleSessionPromptRedeliveredBusyRequestRejectsAsBusy(t *testing.T) {
	replayed := admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", "")
	replayed.Turn.State = chatsessions.TurnStateRunning
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  replayed,
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded busy rejection for a redelivered in-flight request")
	}
	if chatSessions.advanceTurnCalled {
		t.Fatal("AdvanceTurn was called, want no turn-state mutation for a redelivered busy request")
	}
	if len(factoryTarget.startCalls) != 0 || len(factoryTarget.invokeCalls) != 0 {
		t.Fatalf("Factory calls = start:%d invoke:%d, want 0 and 0 for a redelivered busy request",
			len(factoryTarget.startCalls), len(factoryTarget.invokeCalls))
	}
}

// TestHandleSessionPromptFactoryStartFailureMakesNoBindCall proves a
// StartAsync failure reports a bounded failure and never calls
// BindFactorySession.
func TestHandleSessionPromptFactoryStartFailureMakesNoBindCall(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startErr: errors.New("factory sessions boom")}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure for a Factory start failure")
	}
	if chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was called, want no binding attempt after a Factory start failure")
	}
}

// TestHandleSessionPromptEmptyFactorySessionIdentityFailsSafely proves a
// StartAsync success carrying a blank SessionID fails safely and
// never calls BindFactorySession, rather than committing an empty identity.
func TestHandleSessionPromptEmptyFactorySessionIdentityFailsSafely(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startResult: factorysessions.AsyncStartResult{SessionID: ""}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure for an empty returned Factory Session identity")
	}
	if chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was called, want no binding attempt for an empty returned identity")
	}
}

// TestHandleSessionPromptRecordPendingFailureAfterStartMakesNoBindCall
// proves that when RecordPendingFactorySession itself fails (a Go-level
// error) right after a successful start, the handler reports a bounded
// failure and never proceeds to dispatch or bind against the just-started
// identity -- an unrecorded pending identity must not be treated as safely
// reconcilable.
func TestHandleSessionPromptRecordPendingFailureAfterStartMakesNoBindCall(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult:               sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:                admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
		recordPendingFactorySessionErr: errors.New("record pending boom"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startResult: factorysessions.AsyncStartResult{SessionID: "fs-1"}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure when RecordPendingFactorySession fails")
	}
	if len(factoryTarget.invokeCalls) != 0 {
		t.Fatalf("InvokeFactorySession call count = %d, want 0 when the pending identity was never durably recorded", len(factoryTarget.invokeCalls))
	}
	if chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was called, want no binding attempt when RecordPendingFactorySession fails")
	}
}

// TestHandleSessionPromptRetryAfterRecordPendingFailureReusesStableRequestID
// proves that a later, uniquely identified prompt for the same still-unbound
// episode (observed after a RecordPendingFactorySession failure left the
// episode with no pending or bound identity at all) calls StartAsync
// again with the exact same RequestID as the original attempt -- a stable key
// derived only from the session and episode, never the admitted Turn's own
// ID, which differs on every retry. This is what lets
// ondemandtarget.Service.StartAsync's own request-scoped deduplication (see
// that package's TestStartAsyncSameRequestIDConvergesOnASingleActivation)
// converge the retry onto the exact same runtime instead of opening a second
// one for the same episode; this test proves the transport's half of that
// contract (the stable key), not the activation service's own dedup logic,
// which is a different package's responsibility.
func TestHandleSessionPromptRetryAfterRecordPendingFailureReusesStableRequestID(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResults: []chatsessions.StartTurnResult{
			admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
			admittedTurnResult("session-1", "factory:@you/review", 5, "/work/project", "turn-2", ""),
		},
		recordPendingFactorySessionErrs: []error{errors.New("record pending boom"), nil},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult:  factorysessions.AsyncStartResult{SessionID: "fs-1"},
		invokeResult: factorysessions.InvocationResult{SessionID: "fs-1", Status: factorysessions.InvocationTerminalStatusCompleted},
	}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	firstEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "first message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), firstEnv); rpcErr == nil {
		t.Fatal("first handleSessionPrompt() error = nil, want a bounded failure from the injected RecordPendingFactorySession fault")
	}

	secondEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "second message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want the retry to succeed", rpcErr)
	}

	if len(factoryTarget.startCalls) != 2 {
		t.Fatalf("StartAsync call count = %d, want exactly 2 (the original attempt plus the retry)", len(factoryTarget.startCalls))
	}
	first, second := factoryTarget.startCalls[0], factoryTarget.startCalls[1]
	if first.RequestID == "" || first.RequestID != second.RequestID {
		t.Fatalf("StartAsync RequestID[0] = %q, RequestID[1] = %q, want the identical stable per-episode key on both calls", first.RequestID, second.RequestID)
	}
	if !chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was not called, want the retry to bind after RecordPendingFactorySession succeeds the second time")
	}
	if chatSessions.bindFactorySessionReq.FactorySessionID != "fs-1" {
		t.Fatalf("BindFactorySession FactorySessionID = %q, want fs-1", chatSessions.bindFactorySessionReq.FactorySessionID)
	}
}

// TestHandleSessionPromptFirstTurnInvokeFailureMakesNoBindCall proves that
// when the first turn's own follow-up InvokeFactorySession call (the one that
// actually dispatches this turn's content into the just-started identity)
// fails, the handler reports a bounded failure and never proceeds to bind.
func TestHandleSessionPromptFirstTurnInvokeFailureMakesNoBindCall(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult: factorysessions.AsyncStartResult{SessionID: "fs-1"},
		invokeErr:   errors.New("dispatch boom"),
	}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure when the first turn's own invoke fails")
	}
	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count = %d, want exactly 1", len(factoryTarget.startCalls))
	}
	if len(factoryTarget.invokeCalls) != 1 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 1", len(factoryTarget.invokeCalls))
	}
	if chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was called, want no binding attempt when the dispatch itself failed")
	}
}

// TestHandleSessionPromptBindConflictClosesTheJustStartedRuntime proves that
// when BindFactorySession fails with *chatsessions.FactorySessionConflictError
// -- a different identity already won the episode -- the handler compensates
// by closing the exact runtime identity StartAsync just returned and
// abandons its pending-start reconciliation record, since the next call for
// this episode will correctly observe and invoke the already-bound winner
// instead of ever needing this loser again.
func TestHandleSessionPromptBindConflictClosesTheJustStartedRuntime(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
		bindFactorySessionErr: &chatsessions.FactorySessionConflictError{
			SessionID: "session-1", Episode: 4, Bound: "fs-winner", Attempted: "fs-orphan-candidate",
		},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startResult: factorysessions.AsyncStartResult{SessionID: "fs-orphan-candidate"}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure when BindFactorySession fails")
	}

	if len(factoryTarget.closeCalls) != 1 {
		t.Fatalf("CloseFactorySession call count = %d, want exactly 1", len(factoryTarget.closeCalls))
	}
	if factoryTarget.closeCalls[0] != "fs-orphan-candidate" {
		t.Fatalf("CloseFactorySession sessionID = %q, want the exact identity StartAsync returned", factoryTarget.closeCalls[0])
	}

	wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-1", chatsessions.TurnStateRunning, chatsessions.TurnStateFailed)
}

// TestHandleSessionPromptBindFailureRetryReusesPendingFactorySession proves
// that when BindFactorySession fails for any reason *other* than a genuine
// different-identity conflict (for example a transient version race), the
// handler does not close the just-started runtime, and a later uniquely
// identified retry for the same still-unbound episode dispatches through
// InvokeFactorySession against that exact pending identity instead of calling
// StartAsync a second time -- so a bind failure can never cause two
// Factory Sessions to exist for one episode.
func TestHandleSessionPromptBindFailureRetryReusesPendingFactorySession(t *testing.T) {
	secondTurn := admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "")
	// The real chatsessions.Store durably records the first turn's started
	// (not yet committed) identity via RecordPendingFactorySession before
	// the failed bind attempt; a later admitted turn's own fresh episode
	// snapshot observes it through Episode.PendingFactorySessionID -- this
	// fake does not run real Store logic, so the second canned result
	// simulates exactly what that durable record produces.
	secondTurn.Episode.PendingFactorySessionID = "fs-pending"
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResults: []chatsessions.StartTurnResult{
			admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
			secondTurn,
		},
		bindFactorySessionErrs: []error{errors.New("bind failed: version race"), nil},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult:  factorysessions.AsyncStartResult{SessionID: "fs-pending"},
		invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted},
	}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	firstEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "first message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), firstEnv); rpcErr == nil {
		t.Fatal("first handleSessionPrompt() error = nil, want a bounded failure when BindFactorySession fails")
	}
	if len(factoryTarget.closeCalls) != 0 {
		t.Fatalf("CloseFactorySession call count = %d, want 0: a non-conflict bind failure must not abandon the pending runtime", len(factoryTarget.closeCalls))
	}

	secondEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "second message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want success once the retried bind succeeds", rpcErr)
	}

	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count = %d, want exactly 1 (the retry reuses the pending identity via invoke, never starting a second Factory Session)", len(factoryTarget.startCalls))
	}
	// The first (failed-bind) attempt already dispatches its own content via
	// invoke right after starting, before the bind attempt that then fails;
	// the retry's own invoke against the same pending identity is the second.
	if len(factoryTarget.invokeCalls) != 2 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 2 (the first attempt's dispatch plus the retry's)", len(factoryTarget.invokeCalls))
	}
	for i, call := range factoryTarget.invokeCalls {
		if call.sessionID != "fs-pending" {
			t.Fatalf("InvokeFactorySession[%d] sessionID = %q, want fs-pending", i, call.sessionID)
		}
	}
}

// TestHandleSessionPromptWithoutFactoryTargetCollaboratorAdmitsButFailsBound
// proves a Server with no Factory Sessions collaborator still admits the
// turn (StartTurn is called) before reporting a bounded failure, matching
// every other dependency-unavailable path in this package.
func TestHandleSessionPromptWithoutFactoryTargetCollaboratorAdmitsButFailsBound(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure with no Factory target collaborator")
	}
	if !chatSessions.startTurnCalled {
		t.Fatal("StartTurn was not called, want the turn admitted before the missing-collaborator failure")
	}
}

func TestHandleSessionPromptFirstTurnAdvancesByInvokeOutcome(t *testing.T) {
	tests := []struct {
		name   string
		status factorysessions.InvocationTerminalStatus
		want   chatsessions.TurnState
	}{
		{"completed", factorysessions.InvocationTerminalStatusCompleted, chatsessions.TurnStateCompleted},
		{"canceled", factorysessions.InvocationTerminalStatusCanceled, chatsessions.TurnStateCanceled},
		{"timed_out", factorysessions.InvocationTerminalStatusTimedOut, chatsessions.TurnStateCanceled},
		{"failed", factorysessions.InvocationTerminalStatusFailed, chatsessions.TurnStateFailed},
		{"unmapped_future_status", factorysessions.InvocationTerminalStatus("SOME_FUTURE_STATUS"), chatsessions.TurnStateFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chatSessions := &fakeChatSessionsService{
				getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
				startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
			}
			catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
			factoryTarget := &fakeFactoryTargetService{
				startResult:  factorysessions.AsyncStartResult{SessionID: "fs-1"},
				invokeResult: factorysessions.InvocationResult{SessionID: "fs-1", Status: tc.status},
			}
			server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "hello there"))
			_, rpcErr := server.handleSessionPrompt(context.Background(), env)
			wantPromptFailureForStatus(t, tc.status, rpcErr)

			wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-1", chatsessions.TurnStateRunning, tc.want)
		})
	}
}

// TestHandleSessionPromptLaterTurnAdvancesByInvocationOutcome proves a later
// (invoke) turn's terminal advancement tracks the Factory Session's own
// published terminal status exactly -- completed advances to COMPLETED,
// caller-canceled/timed-out both advance to CANCELED, and a genuine Factory
// failure (or any unmapped future status) advances to FAILED -- even though
// InvokeFactorySession itself returns no Go error and the ACP response still
// carries its own (separately mapped, safe-fallback) stop reason.
func TestHandleSessionPromptLaterTurnAdvancesByInvocationOutcome(t *testing.T) {
	tests := []struct {
		name   string
		status factorysessions.InvocationTerminalStatus
		want   chatsessions.TurnState
	}{
		{"completed", factorysessions.InvocationTerminalStatusCompleted, chatsessions.TurnStateCompleted},
		{"canceled", factorysessions.InvocationTerminalStatusCanceled, chatsessions.TurnStateCanceled},
		{"timed_out", factorysessions.InvocationTerminalStatusTimedOut, chatsessions.TurnStateCanceled},
		{"failed", factorysessions.InvocationTerminalStatusFailed, chatsessions.TurnStateFailed},
		{"unmapped_future_status", factorysessions.InvocationTerminalStatus("SOME_FUTURE_STATUS"), chatsessions.TurnStateFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chatSessions := &fakeChatSessionsService{
				getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
				startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-2", "fs-already-bound"),
			}
			catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
			factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{Status: tc.status}}
			server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "a later message"))
			_, rpcErr := server.handleSessionPrompt(context.Background(), env)
			wantPromptFailureForStatus(t, tc.status, rpcErr)

			wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-2", chatsessions.TurnStateRunning, tc.want)
		})
	}
}

// TestHandleSessionPromptFactoryDispatchFailureAdvancesTurnToFailed proves a
// Go-level Factory dispatch failure (start or invoke) both reports a bounded
// internal error to the client and terminalizes the admitted turn to
// TurnStateFailed, so a retried request is never blocked behind a stranded
// non-terminal turn.
func TestHandleSessionPromptFactoryDispatchFailureAdvancesTurnToFailed(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		chatSessions := &fakeChatSessionsService{
			getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
			startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
		}
		catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
		factoryTarget := &fakeFactoryTargetService{startErr: errors.New("factory sessions boom")}
		server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

		env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
			promptTextParams("session-1", "hello there"))
		if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr == nil {
			t.Fatal("handleSessionPrompt() error = nil, want a bounded failure")
		}

		wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-1", chatsessions.TurnStateRunning, chatsessions.TurnStateFailed)
	})

	t.Run("invoke", func(t *testing.T) {
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
			t.Fatal("handleSessionPrompt() error = nil, want a bounded failure")
		}

		wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-2", chatsessions.TurnStateRunning, chatsessions.TurnStateFailed)
	})
}

// TestHandleSessionPromptFactoryDispatchCancellationAdvancesTurnToCanceled
// proves a Factory dispatch failure whose cause is context.Canceled or
// context.DeadlineExceeded terminalizes the admitted turn to
// TurnStateCanceled (not TurnStateFailed) while still classifying the ACP
// response as the request-cancelled outcome -- the cause remains discoverable
// internally via errors.Is even though it never reaches the client's error
// text.
func TestHandleSessionPromptFactoryDispatchCancellationAdvancesTurnToCanceled(t *testing.T) {
	tests := []struct {
		name  string
		cause error
	}{
		{"canceled", context.Canceled},
		{"deadline_exceeded", context.DeadlineExceeded},
		{"wrapped_canceled", fmt.Errorf("factory sessions: %w", context.Canceled)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chatSessions := &fakeChatSessionsService{
				getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
				startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
			}
			catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
			factoryTarget := &fakeFactoryTargetService{startErr: tc.cause}
			server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "hello there"))
			_, rpcErr := server.handleSessionPrompt(context.Background(), env)
			if rpcErr == nil {
				t.Fatal("handleSessionPrompt() error = nil, want the request-cancelled outcome")
			}
			if rpcErr.Code != acpsdk.NewRequestCancelled(nil).Code {
				t.Fatalf("error code = %d, want the request-cancelled classification %d", rpcErr.Code, acpsdk.NewRequestCancelled(nil).Code)
			}

			wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-1", chatsessions.TurnStateRunning, chatsessions.TurnStateCanceled)
		})
	}
}

// TestHandleSessionPromptRunningTransitionFailureMakesNoFactoryDispatchCall
// proves that when advancing a freshly admitted turn to TurnStateRunning
// itself fails, the handler reports a bounded internal error, never attempts
// a Factory dispatch call (StartAsync/InvokeFactorySession) for a
// turn that is not actually confirmed running, and makes one recovery
// AdvanceTurn(CANCELED) attempt -- the one legal terminal transition from
// ADMITTED -- so the session's busy state is not stranded forever.
func TestHandleSessionPromptRunningTransitionFailureMakesNoFactoryDispatchCall(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
		advanceTurnErr:   errors.New("advance turn boom"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{startResult: factorysessions.AsyncStartResult{SessionID: "fs-1"}}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure when the RUNNING transition fails")
	}

	if len(factoryTarget.startCalls) != 0 {
		t.Fatalf("StartAsync call count = %d, want 0 when the turn never confirmed running", len(factoryTarget.startCalls))
	}
	if chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was called, want no binding attempt when the turn never confirmed running")
	}
	wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-1",
		chatsessions.TurnStateRunning, chatsessions.TurnStateCanceled)
}

// TestHandleSessionPromptRunningTransitionFailureRecoveryAdmitsLaterPrompt
// proves the RUNNING-transition recovery attempt actually releases the
// session's busy state against the real chatsessions.Store (not just the
// call-recording fake): after an injected single-shot AdvanceTurn failure,
// a later uniquely identified prompt on the same session is admitted and
// dispatches, instead of being rejected as busy forever.
func TestHandleSessionPromptRunningTransitionFailureRecoveryAdmitsLaterPrompt(t *testing.T) {
	store, err := chatsessionswire.NewService(sequentialIDGenerator("session"), fixedClock(time.Unix(0, 1)), stubEventsAppender{}, stubEventsReader{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	created, err := store.CreateSession(context.Background(), chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-setup", JSONRPCNumberID: "0"},
		WorkingRoot:   "/work/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	faulty := &firstCallFailingChatSessions{
		Service:   store,
		method:    "AdvanceTurn",
		injectErr: errors.New("advance turn boom"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult:  factorysessions.AsyncStartResult{SessionID: "fs-1"},
		invokeResult: factorysessions.InvocationResult{SessionID: "fs-1", Status: factorysessions.InvocationTerminalStatusCompleted},
	}
	server := New(nil, faulty, catalog, factoryTarget, nil, func() (string, error) { return "/home/operator", nil }, nil, nil)

	firstEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(created.Session.ID, "first message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), firstEnv); rpcErr == nil {
		t.Fatal("first handleSessionPrompt() error = nil, want a bounded failure from the injected RUNNING-transition fault")
	}

	secondEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(created.Session.ID, "second message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want the session admitted (not stranded busy)", rpcErr)
	}
	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count = %d, want exactly 1 (only the second, successfully admitted turn dispatches)", len(factoryTarget.startCalls))
	}
}

// TestHandleSessionPromptPendingFactorySessionSurvivesNewServerInstance
// proves the reconciliation record a failed bind leaves behind is durable
// Chat/Factory Sessions authority state, not this Server instance's own
// memory: after an injected single-shot BindFactorySession failure on a
// real chatsessions.Store, a brand-new *Server constructed over that same
// store (simulating a transport restart) still observes the pending
// identity through the second turn's own admitted episode snapshot and
// invokes it instead of starting a second Factory Session.
func TestHandleSessionPromptPendingFactorySessionSurvivesNewServerInstance(t *testing.T) {
	store, err := chatsessionswire.NewService(sequentialIDGenerator("session"), fixedClock(time.Unix(0, 1)), stubEventsAppender{}, stubEventsReader{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	created, err := store.CreateSession(context.Background(), chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-setup", JSONRPCNumberID: "0"},
		WorkingRoot:   "/work/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	faulty := &firstCallFailingBindFactorySession{
		Service:   store,
		injectErr: errors.New("bind failed: version race"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult:  factorysessions.AsyncStartResult{SessionID: "fs-pending"},
		invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted},
	}
	firstServer := New(nil, faulty, catalog, factoryTarget, nil, func() (string, error) { return "/home/operator", nil }, nil, nil)

	firstEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(created.Session.ID, "first message"))
	if _, rpcErr := firstServer.handleSessionPrompt(context.Background(), firstEnv); rpcErr == nil {
		t.Fatal("first handleSessionPrompt() error = nil, want a bounded failure when the injected bind fault fires")
	}
	if len(factoryTarget.closeCalls) != 0 {
		t.Fatalf("CloseFactorySession call count = %d, want 0: a non-conflict bind failure must not abandon the pending runtime", len(factoryTarget.closeCalls))
	}

	// A brand-new Server, sharing only the underlying store (not the failed
	// firstServer instance or its wrapper), stands in for a restarted
	// transport process.
	secondServer := New(nil, store, catalog, factoryTarget, nil, func() (string, error) { return "/home/operator", nil }, nil, nil)

	secondEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(created.Session.ID, "second message"))
	if _, rpcErr := secondServer.handleSessionPrompt(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want success once the retried bind succeeds", rpcErr)
	}

	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count = %d, want exactly 1 (the new Server instance reuses the durably recorded pending identity via invoke, never starting a second Factory Session)", len(factoryTarget.startCalls))
	}
	// The first server's own attempt already dispatched via invoke right
	// after starting, before its bind attempt failed; the second server's
	// own invoke against the same durably-recorded pending identity is the
	// second call.
	if len(factoryTarget.invokeCalls) != 2 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 2 (the first attempt's dispatch plus the second server's)", len(factoryTarget.invokeCalls))
	}
	for i, call := range factoryTarget.invokeCalls {
		if call.sessionID != "fs-pending" {
			t.Fatalf("InvokeFactorySession[%d] sessionID = %q, want fs-pending", i, call.sessionID)
		}
	}
}

// TestHandleSessionPromptTerminalTransitionFailurePropagatesBoundedError
// proves that when the final terminalizing AdvanceTurn call itself fails
// after a successful Factory dispatch, the handler reports that failure as a
// bounded internal error instead of silently discarding it and returning the
// dispatch's own successful response -- an unterminalized turn must never be
// masked by an otherwise-successful outcome -- and makes one recovery
// AdvanceTurn(FAILED) attempt so the session's busy state is not stranded.
func TestHandleSessionPromptTerminalTransitionFailurePropagatesBoundedError(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
		startTurnResult:  admittedTurnResult("session-1", "factory:@you/review", 4, "/work/project", "turn-1", ""),
		advanceTurnErrs:  []error{nil, errors.New("terminal advance boom"), nil},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult:  factorysessions.AsyncStartResult{SessionID: "fs-1"},
		invokeResult: factorysessions.InvocationResult{SessionID: "fs-1", Status: factorysessions.InvocationTerminalStatusCompleted},
	}
	server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))
	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure when the terminal AdvanceTurn call fails")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil when terminalization fails", result)
	}

	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count = %d, want exactly 1 (the dispatch itself succeeded)", len(factoryTarget.startCalls))
	}
	if !chatSessions.bindFactorySessionCalled {
		t.Fatal("BindFactorySession was not called, want the successful dispatch to still bind before terminalization is attempted")
	}
	wantAdvanceTurnSequence(t, chatSessions, "session-1", "turn-1",
		chatsessions.TurnStateRunning, chatsessions.TurnStateCompleted, chatsessions.TurnStateFailed)
}

// TestHandleSessionPromptTerminalTransitionFailureRecoveryAdmitsLaterPrompt
// proves the terminal-transition recovery attempt actually releases the
// session's busy state against the real chatsessions.Store: after an
// injected single-shot terminal AdvanceTurn failure, a later uniquely
// identified prompt on the same session is admitted and dispatches, instead
// of being rejected as busy forever. Because the first turn's dispatch (and
// its BindFactorySession) already succeeded before its terminalization
// faulted, the episode is already bound by the time the second turn is
// admitted, so the second turn correctly takes the invoke branch -- proving
// not just that admission recovers, but that recovery never causes a second
// Factory Session to be started for the same episode.
func TestHandleSessionPromptTerminalTransitionFailureRecoveryAdmitsLaterPrompt(t *testing.T) {
	store, err := chatsessionswire.NewService(sequentialIDGenerator("session"), fixedClock(time.Unix(0, 1)), stubEventsAppender{}, stubEventsReader{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	created, err := store.CreateSession(context.Background(), chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-setup", JSONRPCNumberID: "0"},
		WorkingRoot:   "/work/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	faulty := &nthAdvanceTurnFailingChatSessions{
		Service: store, failOnCall: 2, injectErr: errors.New("terminal advance boom"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult:  factorysessions.AsyncStartResult{SessionID: "fs-1"},
		invokeResult: factorysessions.InvocationResult{SessionID: "fs-1", Status: factorysessions.InvocationTerminalStatusCompleted},
	}
	server := New(nil, faulty, catalog, factoryTarget, nil, func() (string, error) { return "/home/operator", nil }, nil, nil)

	firstEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(created.Session.ID, "first message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), firstEnv); rpcErr == nil {
		t.Fatal("first handleSessionPrompt() error = nil, want a bounded failure from the injected terminal-transition fault")
	}

	secondEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(created.Session.ID, "second message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want the session admitted (not stranded busy)", rpcErr)
	}
	if len(factoryTarget.startCalls) != 1 {
		t.Fatalf("StartAsync call count = %d, want exactly 1 (the first turn's dispatch itself succeeded before its terminalization faulted)", len(factoryTarget.startCalls))
	}
	// The first turn's own content already dispatched via invoke right
	// after its successful start; the second turn's own invoke against the
	// already-bound Factory Session (reused, not a second start) is the
	// second call.
	if len(factoryTarget.invokeCalls) != 2 {
		t.Fatalf("InvokeFactorySession call count = %d, want exactly 2 (the first turn's dispatch plus the second turn's reuse)", len(factoryTarget.invokeCalls))
	}
}

// TestHandleSessionPromptFailedTerminalTransitionFailureRecoveryAdmitsLaterPrompt
// proves that when the *intended* terminal transition is already FAILED (a
// genuine dispatch failure, not a downstream success) and that exact
// AdvanceTurn(FAILED) call itself fails, recovery still releases the
// session's busy state -- via the CANCELED fallback in terminalRecoveryOrder
// -- instead of stranding the turn forever, which a version of this recovery
// that only ever retried FAILED itself could never do. Proven against a real
// chatsessions.Store (not a call-recording fake) so the recovered
// RUNNING->CANCELED transition is a real, legal state change, and a later
// uniquely identified prompt is genuinely admitted afterward.
func TestHandleSessionPromptFailedTerminalTransitionFailureRecoveryAdmitsLaterPrompt(t *testing.T) {
	store, err := chatsessionswire.NewService(sequentialIDGenerator("session"), fixedClock(time.Unix(0, 1)), stubEventsAppender{}, stubEventsReader{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	created, err := store.CreateSession(context.Background(), chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-setup", JSONRPCNumberID: "0"},
		WorkingRoot:   "/work/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Call 1 is the ADMITTED->RUNNING transition (succeeds); call 2 is the
	// terminal AdvanceTurn(FAILED) attempt this dispatch failure targets --
	// inject the fault there so the primary terminal attempt is exactly the
	// FAILED case round-4's review found unrecoverable.
	faulty := &nthAdvanceTurnFailingChatSessions{
		Service: store, failOnCall: 2, injectErr: errors.New("terminal advance boom"),
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	factoryTarget := &fakeFactoryTargetService{
		startResult: factorysessions.AsyncStartResult{SessionID: "fs-1"},
		// The first turn's own dispatch fails (its intended terminal is
		// therefore FAILED); the second turn's reuse of the already-pending
		// identity succeeds, so this test can prove admission recovers
		// without conflating it with a second genuine dispatch failure.
		invokeErrs: []error{errors.New("dispatch boom"), nil},
		invokeResult: factorysessions.InvocationResult{
			SessionID: "fs-1", Status: factorysessions.InvocationTerminalStatusCompleted,
		},
	}
	server := New(nil, faulty, catalog, factoryTarget, nil, func() (string, error) { return "/home/operator", nil }, nil, nil)

	firstEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(created.Session.ID, "first message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), firstEnv); rpcErr == nil {
		t.Fatal("first handleSessionPrompt() error = nil, want a bounded failure from the injected dispatch fault")
	}

	secondEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(created.Session.ID, "second message"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want the session admitted (not stranded busy) after a recovered FAILED-transition failure", rpcErr)
	}
}

// TestServeRoutesSessionCancelToCapturedFactorySession proves a well-formed
// "session/cancel" notification, dispatched through the full Serve/
// dispatchRequest path (not a direct handleSessionCancel call), reaches the
// exact Factory Session identity a prior turn bound to the addressed Chat
// Session's current episode -- forwarding the caller's context and a
// ControlRequest correlated to the addressed session -- and, per JSON-RPC
// 2.0's notification contract, writes no response line at all.
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

// controlRecordingChatSessions observes control requests and their committed
// outcomes while delegating every lifecycle decision to a real Chat Sessions
// store. commitEntered/commitRelease, when configured, pause exactly after a
// control has been REQUESTED and immediately before it commits, letting the
// race tests below move the captured turn through a real terminal or
// replacement transition without a timing sleep.
type controlRecordingChatSessions struct {
	chatsessions.Service

	requestEntered chan struct{}
	requestRelease <-chan struct{}
	requestOnce    sync.Once

	commitEntered chan struct{}
	commitRelease <-chan struct{}
	commitOnce    sync.Once

	mu       sync.Mutex
	requests []chatsessions.RequestControlRequest
	advances []chatsessions.AdvanceControlResult
}

func (s *controlRecordingChatSessions) RequestControl(
	ctx context.Context,
	req chatsessions.RequestControlRequest,
) (chatsessions.RequestControlResult, error) {
	if s.requestEntered != nil {
		s.requestOnce.Do(func() { close(s.requestEntered) })
		if s.requestRelease != nil {
			<-s.requestRelease
		}
	}
	result, err := s.Service.RequestControl(ctx, req)
	if err == nil {
		s.mu.Lock()
		s.requests = append(s.requests, req)
		s.mu.Unlock()
	}
	return result, err
}

func (s *controlRecordingChatSessions) AdvanceControl(
	ctx context.Context,
	req chatsessions.AdvanceControlRequest,
) (chatsessions.AdvanceControlResult, error) {
	if req.Next == chatsessions.ControlIntentStateCommitted && s.commitEntered != nil {
		s.commitOnce.Do(func() { close(s.commitEntered) })
		if s.commitRelease != nil {
			<-s.commitRelease
		}
	}
	result, err := s.Service.AdvanceControl(ctx, req)
	if err == nil {
		s.mu.Lock()
		s.advances = append(s.advances, result)
		s.mu.Unlock()
	}
	return result, err
}

func (s *controlRecordingChatSessions) snapshotControls() (
	[]chatsessions.RequestControlRequest,
	[]chatsessions.AdvanceControlResult,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	requests := append([]chatsessions.RequestControlRequest(nil), s.requests...)
	advances := append([]chatsessions.AdvanceControlResult(nil), s.advances...)
	return requests, advances
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

func waitForChannel(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
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

// wantPromptFailureForStatus asserts the prompt answer that matches a Factory
// invocation's terminal status.
//
// ACP has no failure StopReason, so a FAILED invocation cannot be reported
// inside a successful prompt response; it answers with the protocol's own
// failure channel instead, carrying only the closed InvocationErrorCode
// vocabulary. Every other terminal status still yields a normal response.
func wantPromptFailureForStatus(t *testing.T, status factorysessions.InvocationTerminalStatus, rpcErr *acpsdk.RequestError) {
	t.Helper()
	if status != factorysessions.InvocationTerminalStatusFailed {
		if rpcErr != nil {
			t.Fatalf("handleSessionPrompt() error = %+v, want a normal final ACP response for %q", rpcErr, status)
		}
		return
	}
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a failed invocation to fail the prompt rather than report end_turn")
	}
	encoded, marshalErr := json.Marshal(rpcErr)
	if marshalErr != nil {
		t.Fatalf("marshal prompt failure: %v", marshalErr)
	}
	if !strings.Contains(string(encoded), "INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("prompt failure = %s, want the bounded invocation error code", encoded)
	}
}
