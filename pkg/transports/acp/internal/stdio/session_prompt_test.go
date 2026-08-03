package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

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
// Downstream Factory dispatch is not yet implemented, so admission success
// still reports a bounded (non-method-not-found) internal error rather than
// fabricated success.
func TestHandleSessionPromptNonCommandContentAdmitsOneVersionGuardedTurn(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	connID := identity.NewConnectionID()
	env := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a bounded failure: Factory dispatch is not yet implemented")
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
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	firstConn := identity.NewConnectionID()
	firstEnv := numberIdentityEnvelope(t, firstConn, 1, acpsdk.AgentMethodSessionPrompt, promptTextParams("session-1", "hello there"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), firstEnv); rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want the bounded not-yet-implemented rejection")
	}
	firstIdentity := chatSessions.startTurnReq.RequestID

	secondConn := identity.NewConnectionID()
	secondEnv := numberIdentityEnvelope(t, secondConn, 1, acpsdk.AgentMethodSessionPrompt, promptTextParams("session-1", "hello there"))
	if _, rpcErr := server.handleSessionPrompt(context.Background(), secondEnv); rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want the bounded not-yet-implemented rejection")
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
	server := New(nil, nil, nil, nil)
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
