package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

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
