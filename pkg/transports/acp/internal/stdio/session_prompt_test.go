package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionswire "github.com/portpowered/infinite-you/pkg/services/chat_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// sequentialIDGenerator returns a chatsessionswire.IDGenerator that produces
// prefix-1, prefix-2, ... on successive calls, for tests that construct a
// real chatsessions.Store and need deterministic, human-readable identities.
func sequentialIDGenerator(prefix string) chatsessionswire.IDGenerator {
	n := 0
	return func() string {
		n++
		return prefix + "-" + strconv.Itoa(n)
	}
}

// fixedClock returns a chatsessionswire.Clock that always reports at, for
// tests that construct a real chatsessions.Store and do not exercise
// timestamp behavior.
func fixedClock(at time.Time) chatsessionswire.Clock {
	return func() time.Time { return at }
}

// stubEventsAppender is a minimal chatsessionswire.EventsAppender double for
// tests that construct a real chatsessions.Store but do not themselves
// exercise Sequence.
type stubEventsAppender struct{}

func (stubEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, nil
}

// stubEventsReader is a minimal chatsessionswire.EventsReader double for
// tests that construct a real chatsessions.Store but do not themselves
// exercise AcknowledgeAttachment.
type stubEventsReader struct{}

func (stubEventsReader) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, nil
}

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

// promptTextParams builds a raw session/prompt params payload carrying one
// text content block, the shape a client sends for a typed "/factory
// <value>" command or an ordinary chat message alike.
func promptTextParams(sessionID, text string) string {
	raw, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestParseFactoryCommandRecognizesExactFormOnly(t *testing.T) {
	tests := []struct {
		name        string
		content     []session.TextContent
		wantValue   string
		wantMatched bool
		wantErr     bool
	}{
		{name: "exact command", content: []session.TextContent{{Text: "/factory factory:@you/review"}}, wantValue: "factory:@you/review", wantMatched: true},
		{name: "missing value", content: []session.TextContent{{Text: "/factory"}}, wantMatched: true, wantErr: true},
		{name: "extra token", content: []session.TextContent{{Text: "/factory a b"}}, wantMatched: true, wantErr: true},
		{name: "unrelated prompt", content: []session.TextContent{{Text: "hello there"}}, wantMatched: false},
		{name: "similar but different command", content: []session.TextContent{{Text: "/factories factory:@you/review"}}, wantMatched: false},
		{name: "multiple content blocks", content: []session.TextContent{{Text: "/factory"}, {Text: "factory:@you/review"}}, wantMatched: false},
		{name: "leading whitespace", content: []session.TextContent{{Text: "   /factory factory:@you/review  "}}, wantValue: "factory:@you/review", wantMatched: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, matched, err := parseFactoryCommand(tc.content)
			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tc.wantMatched)
			}
			if tc.wantErr && err == nil {
				t.Fatal("err = nil, want a malformed-command error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if value != tc.wantValue {
				t.Fatalf("value = %q, want %q", value, tc.wantValue)
			}
		})
	}
}

func TestHandleSessionPromptFactoryCommandDelegatesToChangeTarget(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	var resp acpsdk.PromptResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want end_turn", resp.StopReason)
	}

	if !chatSessions.setTargetCalled {
		t.Fatal("SetTarget was not called, want exactly one target change")
	}
	wantTarget := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"}
	if chatSessions.setTargetReq.Target != wantTarget {
		t.Fatalf("SetTarget Target = %+v, want %+v", chatSessions.setTargetReq.Target, wantTarget)
	}
	if chatSessions.setTargetReq.ExpectedVersion != 3 {
		t.Fatalf("SetTarget ExpectedVersion = %d, want 3", chatSessions.setTargetReq.ExpectedVersion)
	}
	if chatSessions.startTurnCalled {
		t.Fatal("StartTurn was called, want no prompt turn started for the /factory fallback command")
	}
}

func TestHandleSessionPromptFactoryCommandMissingValueRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a missing /factory value")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a malformed /factory command")
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for a malformed /factory command")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for a malformed /factory command", len(catalog.calls))
	}
}

func TestHandleSessionPromptFactoryCommandExtraTokensRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/review extra"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for an extra-token /factory command")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for an extra-token /factory command")
	}
}

func TestHandleSessionPromptFactoryCommandDisallowedTargetRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: "factory:@you/not-allowed",
		Err:    chatsessions.ErrFactoryTargetNotAllowed,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/not-allowed"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a disallowed target")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for a disallowed target")
	}
}

func TestHandleSessionPromptFactoryCommandStaleVersionRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
		setTargetErr:     &chatsessions.ConflictError{Value: "Session", ID: "session-1", Expected: 3, Actual: 4},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a stale expected version")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
}

func TestHandleSessionPromptFactoryCommandWorkingRootIncompatibleRejects(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: "factory:@you/pinned",
		Err:    chatsessions.ErrFactoryTargetWorkingRootIncompatible,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/pinned"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a working-root-incompatible target")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for a working-root-incompatible target")
	}
}

func TestHandleSessionPromptFactoryCommandProjectionFailureRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/review",
		Choices:       nil,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection when the catalog projects no picker choices")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want zero mutation calls when picker projection fails")
	}
}

func TestHandleSessionPromptFactoryCommandFailureNeverLeaksRawValueOrRoot(t *testing.T) {
	sensitiveTarget := "factory:@you/sk_live_should_never_leak"
	sensitiveRoot := "/home/operator/should-never-leak"
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, sensitiveRoot)}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: sensitiveTarget,
		Err:    chatsessions.ErrFactoryTargetNotInstalled,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory "+sensitiveTarget))

	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection")
	}
	encoded, err := json.Marshal(rpcErr)
	if err != nil {
		t.Fatalf("marshal rpc error: %v", err)
	}
	if strings.Contains(string(encoded), sensitiveTarget) {
		t.Fatalf("rpc error %s leaked the raw requested target", encoded)
	}
	if strings.Contains(string(encoded), sensitiveRoot) {
		t.Fatalf("rpc error %s leaked the raw working root", encoded)
	}
}

// TestHandleSessionPromptNonCommandContentAdmitsOneVersionGuardedTurn proves
// ordinary prompt content -- anything not an exact "/factory <value>"
// command attempt -- now reads the addressed Chat Session and calls its
// canonical StartTurn exactly once with the full request identity, the real
// session id, and the version observed from that read, and never reaches
// the "/factory" changeTarget path or the Factory target catalog at all.
// This Server has no Factory Sessions delegation collaborator configured
// (newTestServer), so admission success still reports a bounded
// (non-method-not-found) internal error rather than fabricated success.
func TestHandleSessionPromptNonCommandContentAdmitsOneVersionGuardedTurn(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
		startTurnResult:  chatsessions.StartTurnResult{Turn: chatsessions.Turn{State: chatsessions.TurnStateAdmitted}},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	connID := identity.NewConnectionID()
	env := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure: no Factory Sessions delegation collaborator is configured")
	}
	if rpcErr.Code == -32601 {
		t.Fatal("error code = method-not-found (-32601), want ordinary prompt content to be admitted, not rejected as unsupported")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil", result)
	}

	if !chatSessions.getSessionCalled {
		t.Fatal("GetSession was not called, want the addressed Chat Session to be read before admission")
	}
	if !chatSessions.startTurnCalled {
		t.Fatal("StartTurn was not called, want exactly one version-guarded turn admission")
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no target change for ordinary prompt content")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for ordinary prompt content", len(catalog.calls))
	}

	wantIdentity := chatsessions.RequestIdentity{
		Kind:            chatsessions.RequestIdentityKindJSONRPCNumber,
		ConnectionID:    string(connID),
		JSONRPCNumberID: "1",
	}
	if chatSessions.startTurnReq.RequestID != wantIdentity {
		t.Fatalf("StartTurn RequestID = %+v, want %+v", chatSessions.startTurnReq.RequestID, wantIdentity)
	}
	if chatSessions.startTurnReq.SessionID != "session-1" {
		t.Fatalf("StartTurn SessionID = %q, want session-1", chatSessions.startTurnReq.SessionID)
	}
	if chatSessions.startTurnReq.ExpectedVersion != 3 {
		t.Fatalf("StartTurn ExpectedVersion = %d, want the observed session version 3", chatSessions.startTurnReq.ExpectedVersion)
	}
}

// TestHandleSessionPromptUnknownSessionRejectsWithNoStartTurnCall proves an
// unknown session (GetSession's *chatsessions.NotFoundError) is rejected
// before StartTurn is ever called, so no Factory effect can follow.
func TestHandleSessionPromptUnknownSessionRejectsWithNoStartTurnCall(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionErr: &chatsessions.NotFoundError{Value: "Session", ID: "session-1"}}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for an unknown session")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.startTurnCalled {
		t.Fatal("StartTurn was called, want no admission for an unknown session")
	}
}

// TestHandleSessionPromptStaleVersionRejectsWithProtocolSafeClassification
// proves a stale expected version (StartTurn's *chatsessions.ConflictError)
// is rejected with a bounded, protocol-safe classification.
func TestHandleSessionPromptStaleVersionRejectsWithProtocolSafeClassification(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
		startTurnErr:     &chatsessions.ConflictError{Value: "Session", ID: "session-1", Expected: 3, Actual: 4},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a stale expected version")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
}

// TestHandleSessionPromptAdmissionFailureClassification proves
// classifyTurnAdmissionFailure's remaining causes -- a StartTurn
// *chatsessions.ValidationError, a GetSession context.Canceled, and any
// other unclassified GetSession failure -- all reject with no Factory
// effect, matching the same bounded-classification contract as the
// NotFoundError/ConflictError/BusyError causes covered above.
func TestHandleSessionPromptAdmissionFailureClassification(t *testing.T) {
	tests := []struct {
		name          string
		getSessionErr error
		startTurnErr  error
	}{
		{"validation_error", nil, &chatsessions.ValidationError{Value: "Session", Field: "WorkingRoot", Err: chatsessions.ErrRequiredValue}},
		{"get_session_canceled", context.Canceled, nil},
		{"get_session_unclassified", errors.New("store unavailable"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatSessions := &fakeChatSessionsService{
				getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
				getSessionErr:    tt.getSessionErr,
				startTurnErr:     tt.startTurnErr,
			}
			catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
			factoryTarget := &fakeFactoryTargetService{}
			server := newTestServerWithFactoryTarget(chatSessions, catalog, factoryTarget, "/home/operator")

			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "hello there"))

			result, rpcErr := server.handleSessionPrompt(context.Background(), env)
			if rpcErr == nil {
				t.Fatal("handleSessionPrompt() error = nil, want a rejection")
			}
			if result != nil {
				t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
			}
			if len(factoryTarget.startCalls) != 0 || len(factoryTarget.invokeCalls) != 0 {
				t.Fatal("Factory effect observed, want zero Factory calls on admission rejection")
			}
		})
	}
}

// TestHandleSessionPromptBusyRejectsSequentialAndConcurrentAdmission proves
// that once a session has a non-terminal active turn, StartTurn's
// *chatsessions.BusyError classifies as a bounded protocol-safe rejection
// with no Factory effect, whether observed from a second sequential prompt
// or from concurrent prompts racing against the same busy session.
func TestHandleSessionPromptBusyRejectsSequentialAndConcurrentAdmission(t *testing.T) {
	busyErr := &chatsessions.BusyError{Value: "Session", ID: "session-1", ActiveTurnID: "turn-1", ActiveTurnState: chatsessions.TurnStateAdmitted}
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
		startTurnErr:     busyErr,
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a busy session")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}

	var wg sync.WaitGroup
	errs := make([]*acpsdk.RequestError, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			concurrentEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "hello again"))
			_, errs[i] = server.handleSessionPrompt(context.Background(), concurrentEnv)
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e == nil {
			t.Fatalf("concurrent handleSessionPrompt() call %d error = nil, want a rejection for a busy session", i)
		}
	}
}

// TestHandleSessionPromptDuplicateDeliveryRejectsSecondCall proves that
// re-delivering the exact same request (same connection, same wire id, same
// content) against an already-busy session is rejected the same way any
// other busy admission attempt is, with no additional Factory effect.
func TestHandleSessionPromptDuplicateDeliveryRejectsSecondCall(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
		startTurnErr:     &chatsessions.BusyError{Value: "Session", ID: "session-1", ActiveTurnID: "turn-1", ActiveTurnState: chatsessions.TurnStateAdmitted},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	connID := identity.NewConnectionID()
	env := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr == nil {
		t.Fatal("first handleSessionPrompt() error = nil, want a rejection")
	}
	if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr == nil {
		t.Fatal("duplicate handleSessionPrompt() error = nil, want the redelivered request rejected the same way")
	}
	if chatSessions.startTurnReq.RequestID.ConnectionID != string(connID) {
		t.Fatalf("StartTurn RequestID.ConnectionID = %q, want %q", chatSessions.startTurnReq.RequestID.ConnectionID, connID)
	}
}

// TestHandleSessionPromptEqualWireIDsAcrossConnectionsRemainDistinct proves
// the same bare JSON-RPC id (1) received on two different connections
// converts to two distinct chatsessions.RequestIdentity values when
// admitting an ordinary prompt turn, matching "session/new"'s existing
// identity-collision-safety guarantee.
func TestHandleSessionPromptEqualWireIDsAcrossConnectionsRemainDistinct(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
		startTurnResult:  chatsessions.StartTurnResult{Turn: chatsessions.Turn{State: chatsessions.TurnStateAdmitted}},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	firstConn := identity.NewConnectionID()
	firstEnv := numberIdentityEnvelope(t, firstConn, 1, acpsdk.AgentMethodSessionPrompt, promptTextParams("session-1", "hello there"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), firstEnv); rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want the bounded missing-collaborator rejection")
	}
	firstIdentity := chatSessions.startTurnReq.RequestID

	secondConn := identity.NewConnectionID()
	secondEnv := numberIdentityEnvelope(t, secondConn, 1, acpsdk.AgentMethodSessionPrompt, promptTextParams("session-1", "hello there"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), secondEnv); rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want the bounded missing-collaborator rejection")
	}
	secondIdentity := chatSessions.startTurnReq.RequestID

	if firstIdentity == secondIdentity {
		t.Fatalf("wire id 1 on two different connections converted to the same RequestIdentity %+v", firstIdentity)
	}
}

// TestHandleSessionPromptWithoutCollaboratorsReportsBoundedFailure proves an
// ordinary prompt against a Server with no chatSessions collaborator
// configured reports a bounded internal failure rather than panicking or
// dispatching a Factory effect.
func TestHandleSessionPromptWithoutCollaboratorsReportsBoundedFailure(t *testing.T) {
	server := New(nil, nil, nil, nil, nil)
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure when collaborators are unset")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil", result)
	}
}

func TestHandleSessionPromptMalformedParamsRejectsBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt, `{not json`)

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for malformed params")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for malformed params")
	}
}

func TestHandleSessionPromptValidationFailureRejectsBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a blank sessionId")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a validation failure")
	}
}

func TestHandleSessionPromptIdentityFailureReturnsNoEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := mintedIdentityEnvelope(t, acpsdk.AgentMethodSessionPrompt, promptTextParams("session-1", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a non-correlated identity")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a rejected identity")
	}
}

func TestServeDispatchesSessionPromptFactoryCommandOverRealJSONRPCFraming(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	input := `{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":` +
		promptTextParams("session-1", "/factory factory:@you/review") + `}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	resp := assertSingleResponseLine(t, out)
	if string(resp.ID) != "1" {
		t.Fatalf("id = %s, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("error = %+v, want a successful result", resp.Error)
	}
	if !chatSessions.setTargetCalled {
		t.Fatal("SetTarget was not called over the real Serve path")
	}
}

// TestFactoryFallbackAndSetConfigOptionProduceEquivalentEffects is the
// parity test story 004 asks for: the same session, catalog, and requested
// target run through both entry paths -- "session/set_config_option" and
// the "/factory <value>" prompt fallback -- and produce the exact same
// SetTarget call (target, session id, expected version), proving neither
// path duplicates catalog filtering, reference parsing, expected-version
// policy, state mutation, or error translation, and that success mutates
// target exactly once per path with no prompt turn or Factory execution.
func TestFactoryFallbackAndSetConfigOptionProduceEquivalentEffects(t *testing.T) {
	buildFixtures := func() (*fakeChatSessionsService, *fakeFactoryTargetCatalogService, *Server) {
		chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
		catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
		return chatSessions, catalog, newTestServer(chatSessions, catalog, "/home/operator")
	}

	t.Run("success", func(t *testing.T) {
		viaConfigOption, _, configServer := buildFixtures()
		env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
			setConfigOptionParams("session-1", "factory:@you/review"))
		if _, rpcErr := configServer.handleSessionSetConfigOption(context.Background(), env); rpcErr != nil {
			t.Fatalf("handleSessionSetConfigOption() error = %+v, want success", rpcErr)
		}

		viaPrompt, _, promptServer := buildFixtures()
		promptEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
			promptTextParams("session-1", "/factory factory:@you/review"))
		if _, rpcErr := promptServer.handleSessionPrompt(context.Background(), promptEnv); rpcErr != nil {
			t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
		}

		if viaConfigOption.setTargetReq.Target != viaPrompt.setTargetReq.Target {
			t.Fatalf("SetTarget Target differs: config_option=%+v prompt=%+v", viaConfigOption.setTargetReq.Target, viaPrompt.setTargetReq.Target)
		}
		if viaConfigOption.setTargetReq.SessionID != viaPrompt.setTargetReq.SessionID {
			t.Fatalf("SetTarget SessionID differs: config_option=%q prompt=%q", viaConfigOption.setTargetReq.SessionID, viaPrompt.setTargetReq.SessionID)
		}
		if viaConfigOption.setTargetReq.ExpectedVersion != viaPrompt.setTargetReq.ExpectedVersion {
			t.Fatalf("SetTarget ExpectedVersion differs: config_option=%d prompt=%d", viaConfigOption.setTargetReq.ExpectedVersion, viaPrompt.setTargetReq.ExpectedVersion)
		}
		if viaPrompt.startTurnCalled {
			t.Fatal("StartTurn was called via the /factory prompt fallback, want no prompt turn")
		}
	})

	failureTable := []struct {
		name  string
		value string
		err   error
	}{
		{name: "disallowed", value: "factory:@you/not-allowed", err: &chatsessions.FactoryTargetCatalogError{Target: "factory:@you/not-allowed", Err: chatsessions.ErrFactoryTargetNotAllowed}},
		{name: "working-root-incompatible", value: "factory:@you/pinned", err: &chatsessions.FactoryTargetCatalogError{Target: "factory:@you/pinned", Err: chatsessions.ErrFactoryTargetWorkingRootIncompatible}},
		{name: "not-installed", value: "factory:@you/missing", err: &chatsessions.FactoryTargetCatalogError{Target: "factory:@you/missing", Err: chatsessions.ErrFactoryTargetNotInstalled}},
	}
	for _, tc := range failureTable {
		t.Run(tc.name, func(t *testing.T) {
			viaConfigOption, catalogA, configServer := buildFixtures()
			catalogA.err = tc.err
			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
				setConfigOptionParams("session-1", tc.value))
			_, configErr := configServer.handleSessionSetConfigOption(context.Background(), env)
			if configErr == nil {
				t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection")
			}

			viaPrompt, catalogB, promptServer := buildFixtures()
			catalogB.err = tc.err
			promptEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "/factory "+tc.value))
			_, promptErr := promptServer.handleSessionPrompt(context.Background(), promptEnv)
			if promptErr == nil {
				t.Fatal("handleSessionPrompt() error = nil, want a rejection")
			}

			if configErr.Code != promptErr.Code {
				t.Fatalf("error classification differs: config_option=%d prompt=%d", configErr.Code, promptErr.Code)
			}
			if viaConfigOption.setTargetCalled || viaPrompt.setTargetCalled {
				t.Fatal("SetTarget was called, want no mutation on either path for a rejected target")
			}
			if viaPrompt.startTurnCalled {
				t.Fatal("StartTurn was called via the /factory prompt fallback, want no prompt turn")
			}
		})
	}
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
			if rpcErr != nil {
				t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
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

// wantAdvanceTurnSequence asserts AdvanceTurn was called exactly once per
// entry in want, in order, each against sessionID/turnID, ending with the
// corresponding TurnState -- proving an admitted turn passes through the
// legal RUNNING intermediate state on its way to an explicit terminal state,
// never skipped and never left non-terminal.
func wantAdvanceTurnSequence(t *testing.T, chatSessions *fakeChatSessionsService, sessionID, turnID string, want ...chatsessions.TurnState) {
	t.Helper()
	if len(chatSessions.advanceTurnReqs) != len(want) {
		t.Fatalf("AdvanceTurn call count = %d, want %d (%v)", len(chatSessions.advanceTurnReqs), len(want), want)
	}
	for i, req := range chatSessions.advanceTurnReqs {
		if req.SessionID != sessionID {
			t.Fatalf("AdvanceTurn[%d] SessionID = %q, want %q", i, req.SessionID, sessionID)
		}
		if req.TurnID != turnID {
			t.Fatalf("AdvanceTurn[%d] TurnID = %q, want %q", i, req.TurnID, turnID)
		}
		if req.Next != want[i] {
			t.Fatalf("AdvanceTurn[%d] Next = %q, want %q", i, req.Next, want[i])
		}
	}
}

// TestHandleSessionPromptFirstTurnAdvancesByInvokeOutcome proves a
// first-turn (start) admission's terminal advancement tracks the follow-up
// InvokeFactorySession call's own published terminal status exactly, the same
// way a later (invoke) turn's does -- not a hardcoded TurnStateCompleted
// regardless of outcome. StartAsync itself only opens the runtime
// (AsyncStartResult carries no terminal status at all); this turn's actual
// published outcome is entirely the immediate follow-up invoke's.
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
			if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr != nil {
				t.Fatalf("handleSessionPrompt() error = %+v, want success (a published Factory failure still yields a normal final ACP response)", rpcErr)
			}

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
			if _, rpcErr := server.handleSessionPrompt(context.Background(), env); rpcErr != nil {
				t.Fatalf("handleSessionPrompt() error = %+v, want success (a Factory-level failure still yields a normal final ACP response)", rpcErr)
			}

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
	server := New(nil, faulty, catalog, factoryTarget, func() (string, error) { return "/home/operator", nil })

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
	firstServer := New(nil, faulty, catalog, factoryTarget, func() (string, error) { return "/home/operator", nil })

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
	secondServer := New(nil, store, catalog, factoryTarget, func() (string, error) { return "/home/operator", nil })

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
	server := New(nil, faulty, catalog, factoryTarget, func() (string, error) { return "/home/operator", nil })

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
	server := New(nil, faulty, catalog, factoryTarget, func() (string, error) { return "/home/operator", nil })

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
