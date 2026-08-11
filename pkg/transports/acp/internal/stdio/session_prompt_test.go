package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
	store, err := newChatSessionsStore("session")
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
	store, err := newChatSessionsStore("session")
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
	store, err := newChatSessionsStore("session")
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
	store, err := newChatSessionsStore("session")
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
