// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	acpsdk "github.com/coder/acp-go-sdk"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"strings"
	"testing"
)

func defaultTestCatalogResult() chatsessions.ResolveFactoryTargetCatalogResult {
	return chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/factory-builder",
		Choices: []chatsessions.FactoryTargetCatalogChoice{
			{Value: "factory:@you/factory-builder", Name: "Factory Builder"},
			{Value: "factory:@you/review", Name: "Review"},
		},
	}
}

func stringIdentityEnvelope(t *testing.T, connID identity.ConnectionID, wireID string, method string, params string) envelope.Envelope {
	t.Helper()
	reqIdentity, err := identity.NewCorrelated(connID, identity.NewStringJSONRPCID(wireID))
	if err != nil {
		t.Fatalf("NewCorrelated: %v", err)
	}
	return envelope.Envelope{Identity: reqIdentity, Method: method, Params: json.RawMessage(params)}
}

func TestHandleSessionNewRejectsNonEmptyMcpServersBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew,
		`{"cwd":"/work/project","mcpServers":[{"name":"x","command":"y"}]}`)

	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for non-empty mcpServers")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0: mcpServers rejection must happen before catalog resolution", len(catalog.calls))
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation for a rejected mcpServers request")
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestHandleSessionNewCreatesOneSessionAndReturnsProjectedPicker(t *testing.T) {
	chatSessions := &fakeChatSessionsService{sessionID: "session-42"}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	connID := identity.NewConnectionID()
	env := numberIdentityEnvelope(t, connID, 7, acpsdk.AgentMethodSessionNew, validSessionNewParams)

	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr != nil {
		t.Fatalf("handleSessionNew() error = %+v, want success", rpcErr)
	}

	var resp acpsdk.NewSessionResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(resp.SessionId) != "session-42" {
		t.Fatalf("sessionId = %q, want session-42", resp.SessionId)
	}
	if len(resp.ConfigOptions) != 1 || resp.ConfigOptions[0].Select == nil {
		t.Fatalf("configOptions = %+v, want exactly one select option", resp.ConfigOptions)
	}
	option := resp.ConfigOptions[0].Select
	if option.Id != "target" || option.Name != "Factory" || option.Type != "select" {
		t.Fatalf("option = %+v, want id=target name=Factory type=select", option)
	}
	if string(option.CurrentValue) != "factory:@you/factory-builder" {
		t.Fatalf("currentValue = %q, want factory:@you/factory-builder", option.CurrentValue)
	}
	if option.Options.Ungrouped == nil || len(*option.Options.Ungrouped) != 2 {
		t.Fatalf("options = %+v, want 2 ungrouped choices", option.Options)
	}

	if len(catalog.calls) != 1 {
		t.Fatalf("catalog resolved %d times, want exactly 1", len(catalog.calls))
	}
	if catalog.calls[0].ClientWorkingRoot != "/work/project" {
		t.Fatalf("ClientWorkingRoot = %q, want the validated editor cwd /work/project", catalog.calls[0].ClientWorkingRoot)
	}

	if !chatSessions.createCalled {
		t.Fatal("CreateSession was not called, want exactly one session creation")
	}
	wantTarget := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/factory-builder"}
	if chatSessions.created.InitialTarget != wantTarget {
		t.Fatalf("InitialTarget = %+v, want %+v", chatSessions.created.InitialTarget, wantTarget)
	}
	if chatSessions.created.WorkingRoot != "/work/project" {
		t.Fatalf("WorkingRoot = %q, want the validated editor cwd /work/project", chatSessions.created.WorkingRoot)
	}

	wantIdentity := chatsessions.RequestIdentity{
		Kind:            chatsessions.RequestIdentityKindJSONRPCNumber,
		ConnectionID:    string(connID),
		JSONRPCNumberID: "7",
	}
	if chatSessions.created.RequestID != wantIdentity {
		t.Fatalf("RequestID = %+v, want %+v", chatSessions.created.RequestID, wantIdentity)
	}
}

func TestHandleSessionNewConvertsStringWireIDs(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	connID := identity.NewConnectionID()
	env := stringIdentityEnvelope(t, connID, "req-abc", acpsdk.AgentMethodSessionNew, validSessionNewParams)

	if _, rpcErr := server.handleSessionNew(context.Background(), env); rpcErr != nil {
		t.Fatalf("handleSessionNew() error = %+v, want success", rpcErr)
	}

	wantIdentity := chatsessions.RequestIdentity{
		Kind:            chatsessions.RequestIdentityKindJSONRPCString,
		ConnectionID:    string(connID),
		JSONRPCStringID: "req-abc",
	}
	if chatSessions.created.RequestID != wantIdentity {
		t.Fatalf("RequestID = %+v, want %+v", chatSessions.created.RequestID, wantIdentity)
	}
}

func TestHandleSessionNewKeepsEqualWireIDsDistinctAcrossConnections(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	firstConn := identity.NewConnectionID()
	firstEnv := numberIdentityEnvelope(t, firstConn, 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	if _, rpcErr := server.handleSessionNew(context.Background(), firstEnv); rpcErr != nil {
		t.Fatalf("first handleSessionNew() error = %+v", rpcErr)
	}
	firstIdentity := chatSessions.created.RequestID

	secondConn := identity.NewConnectionID()
	secondEnv := numberIdentityEnvelope(t, secondConn, 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	if _, rpcErr := server.handleSessionNew(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionNew() error = %+v", rpcErr)
	}
	secondIdentity := chatSessions.created.RequestID

	if firstIdentity == secondIdentity {
		t.Fatalf("wire id 1 on two different connections converted to the same RequestIdentity %+v", firstIdentity)
	}
}

func TestHandleSessionNewCatalogFailureReturnsNoSuccessAndNoSessionCreation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{Err: chatsessions.ErrFactoryTargetCatalogUnavailable}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for a catalog failure")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on catalog failure", result)
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation after a catalog failure")
	}
}

func TestHandleSessionNewCreateSessionFailureReturnsNoSuccess(t *testing.T) {
	chatSessions := &fakeChatSessionsService{createErr: errors.New("create session boom")}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for a session creation failure")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on session creation failure", result)
	}
}

func TestHandleSessionNewFailureNeverLeaksRawCause(t *testing.T) {
	sensitive := "sk_live_should_never_leak"
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{err: errors.New(sensitive)}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	_, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection")
	}
	encoded, err := json.Marshal(rpcErr)
	if err != nil {
		t.Fatalf("marshal rpc error: %v", err)
	}
	if strings.Contains(string(encoded), sensitive) {
		t.Fatalf("rpc error %s leaked the raw dependency cause", encoded)
	}
}

// TestServeDispatchesSessionNewOverRealJSONRPCFraming proves session/new is
// reachable through the same newline-delimited JSON-RPC stdio framing
// "initialize" already uses, not just through handleSessionNew called
// directly: the whole Serve -> scan line -> dispatchRequest ->
// handleSessionNew -> writeResponse path produces one complete, correctly
// correlated, successful JSON-RPC response.
func TestServeDispatchesSessionNewOverRealJSONRPCFraming(t *testing.T) {
	chatSessions := &fakeChatSessionsService{sessionID: "session-real"}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	input := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/work/project","mcpServers":[]}}` + "\n"
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

	var sessionResp acpsdk.NewSessionResponse
	if err := json.Unmarshal(resp.Result, &sessionResp); err != nil {
		t.Fatalf("unmarshal session/new result: %v", err)
	}
	if string(sessionResp.SessionId) != "session-real" {
		t.Fatalf("sessionId = %q, want session-real", sessionResp.SessionId)
	}
	if !chatSessions.createCalled {
		t.Fatal("CreateSession was not called over the real Serve path")
	}
}

// TestServeRespondsMethodNotFoundForEveryUnimplementedMethodStillExcludesSessionNew
// guards against a regression that would put "session/new" back on the
// unimplemented-methods list in TestServeRespondsMethodNotFoundForEveryUnimplementedMethod:
// a server with no session/new collaborators configured must still dispatch
// the method (and report a bounded internal failure), never method-not-found.
func TestServeRespondsMethodNotFoundForEveryUnimplementedMethodStillExcludesSessionNew(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":9,"method":"session/new","params":{"cwd":"/work/project","mcpServers":[]}}` + "\n"
	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	resp := assertSingleResponseLine(t, out)
	if resp.Error == nil {
		t.Fatal("error = nil, want a bounded failure for an unconfigured session/new server")
	}
	if resp.Error.Code == -32601 {
		t.Fatal("error code = method-not-found (-32601), want session/new to be dispatched, not rejected as unsupported")
	}
}

func TestHandleSessionNewWithoutCollaboratorsReportsBoundedFailure(t *testing.T) {
	server := New(nil, nil, nil, nil, nil, nil, nil, nil)
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)

	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a bounded failure when collaborators are unset")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil", result)
	}
}

func TestChatRequestIdentityRejectsNonCorrelatedIdentity(t *testing.T) {
	minted, err := identity.NewMinted("transport-minted-id")
	if err != nil {
		t.Fatalf("identity.NewMinted: %v", err)
	}
	if _, err := chatRequestIdentity(minted); err == nil {
		t.Fatal("chatRequestIdentity() error = nil, want a rejection for a non-connection-correlated identity")
	}
}

func TestHandleSessionNewIdentityFailureReturnsNoEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := mintedIdentityEnvelope(t, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for a non-correlated identity")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0: identity conversion must fail before catalog resolution", len(catalog.calls))
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation for a rejected identity")
	}
}

func TestClassifyDependencyFailureMapsContextCauseToRequestCancelled(t *testing.T) {
	cancelled := classifyDependencyFailure(context.Canceled)
	deadlineExceeded := classifyDependencyFailure(fmt.Errorf("wrapped: %w", context.DeadlineExceeded))
	generic := classifyDependencyFailure(errors.New("boom"))

	wantCancelled := acpsdk.NewRequestCancelled(map[string]any{"reason": "cancelled"})
	if cancelled.Code != wantCancelled.Code {
		t.Fatalf("classifyDependencyFailure(context.Canceled) code = %d, want %d (request-cancelled)", cancelled.Code, wantCancelled.Code)
	}
	if deadlineExceeded.Code != wantCancelled.Code {
		t.Fatalf("classifyDependencyFailure(wrapped DeadlineExceeded) code = %d, want %d (request-cancelled)", deadlineExceeded.Code, wantCancelled.Code)
	}
	if generic.Code == wantCancelled.Code {
		t.Fatalf("classifyDependencyFailure(plain error) code = %d, want a code distinct from request-cancelled", generic.Code)
	}
}

func TestHandleSessionNewResolveHomeDirFailureReturnsNoEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := New(nil, chatSessions, catalog, nil, nil, func() (string, error) { return "", errors.New("resolve home dir boom") }, nil, nil)

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection when resolveHomeDir fails")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 when resolveHomeDir fails", len(catalog.calls))
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation when resolveHomeDir fails")
	}
}

func TestHandleSessionNewBlankHomeDirFailureReturnsNoEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := New(nil, chatSessions, catalog, nil, nil, func() (string, error) { return "", nil }, nil, nil)

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection when resolveHomeDir returns a blank home directory")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for a blank home directory", len(catalog.calls))
	}
}

func TestHandleSessionNewConfigProjectionFailureReturnsNoSessionCreation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/factory-builder",
		Choices:       nil,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection when the catalog projects no picker choices")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation when picker projection fails")
	}
}

func TestHandleSessionSetConfigOptionSucceedsAndRevalidatesThroughCatalog(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr != nil {
		t.Fatalf("handleSessionSetConfigOption() error = %+v, want success", rpcErr)
	}

	var resp acpsdk.SetSessionConfigOptionResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.ConfigOptions) != 1 || resp.ConfigOptions[0].Select == nil {
		t.Fatalf("configOptions = %+v, want exactly one select option", resp.ConfigOptions)
	}
	if string(resp.ConfigOptions[0].Select.CurrentValue) != "factory:@you/review" {
		t.Fatalf("currentValue = %q, want factory:@you/review", resp.ConfigOptions[0].Select.CurrentValue)
	}

	if !chatSessions.getSessionCalled {
		t.Fatal("GetSession was not called, want the addressed session to be read")
	}
	if len(catalog.calls) != 1 {
		t.Fatalf("catalog resolved %d times, want exactly 1", len(catalog.calls))
	}
	if catalog.calls[0].ClientWorkingRoot != "/work/project" {
		t.Fatalf("ClientWorkingRoot = %q, want the session's recorded working root /work/project", catalog.calls[0].ClientWorkingRoot)
	}
	if catalog.calls[0].CurrentTarget != "factory:@you/review" {
		t.Fatalf("CurrentTarget = %q, want the requested value revalidated through the catalog", catalog.calls[0].CurrentTarget)
	}

	if !chatSessions.setTargetCalled {
		t.Fatal("SetTarget was not called, want exactly one target change")
	}
	wantTarget := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"}
	if chatSessions.setTargetReq.Target != wantTarget {
		t.Fatalf("SetTarget Target = %+v, want %+v", chatSessions.setTargetReq.Target, wantTarget)
	}
	if chatSessions.setTargetReq.SessionID != "session-1" {
		t.Fatalf("SetTarget SessionID = %q, want session-1", chatSessions.setTargetReq.SessionID)
	}
	if chatSessions.setTargetReq.ExpectedVersion != 3 {
		t.Fatalf("SetTarget ExpectedVersion = %d, want 3 (the version observed from GetSession)", chatSessions.setTargetReq.ExpectedVersion)
	}
}

func TestHandleSessionSetConfigOptionMalformedParamsRejectsBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption, `{not json`)

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for malformed params")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for malformed params")
	}
}

func TestHandleSessionSetConfigOptionIdentityFailureReturnsNoEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := mintedIdentityEnvelope(t, acpsdk.AgentMethodSessionSetConfigOption, setConfigOptionParams("session-1", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for a non-correlated identity")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a rejected identity")
	}
}

func TestChangeTargetBlankWorkingRootRejectsWithNoCatalogResolution(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for an unknown working root")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for a blank session working root", len(catalog.calls))
	}
}

func TestChangeTargetResolveHomeDirFailureReturnsNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := New(nil, chatSessions, catalog, nil, nil, func() (string, error) { return "", errors.New("resolve home dir boom") }, nil, nil)

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection when resolveHomeDir fails")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 when resolveHomeDir fails", len(catalog.calls))
	}
}

func TestChangeTargetBlankHomeDirFailureReturnsNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := New(nil, chatSessions, catalog, nil, nil, func() (string, error) { return "", nil }, nil, nil)

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for a blank home directory")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for a blank home directory", len(catalog.calls))
	}
}

func TestChangeTargetConfigProjectionFailureReturnsNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/review",
		Choices:       nil,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection when the catalog projects no picker choices")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want zero mutation calls when picker projection fails")
	}
}

func TestClassifyTargetSelectionFailureMapsContextCauseThroughClassifyDependencyFailure(t *testing.T) {
	got := classifyTargetSelectionFailure(context.Canceled)
	want := classifyDependencyFailure(context.Canceled)
	if got.Code != want.Code {
		t.Fatalf("classifyTargetSelectionFailure(context.Canceled) code = %d, want %d (delegated to classifyDependencyFailure)", got.Code, want.Code)
	}
}

func TestClassifyTargetSelectionFailureMapsUnclassifiedCatalogErrorToInternalError(t *testing.T) {
	cause := &chatsessions.FactoryTargetCatalogError{Err: chatsessions.ErrFactoryTargetCatalogUnavailable}
	got := classifyTargetSelectionFailure(cause)
	want := classifyDependencyFailure(cause)
	if got.Code != want.Code {
		t.Fatalf("classifyTargetSelectionFailure(unclassified catalog error) code = %d, want %d (internal error, not invalid-params)", got.Code, want.Code)
	}
}

func TestClassifyTargetSelectionFailureMapsValidationErrorToSafeReject(t *testing.T) {
	cause := &chatsessions.ValidationError{Value: "Session", Field: "WorkingRoot", Err: chatsessions.ErrRequiredValue}
	got := classifyTargetSelectionFailure(cause)
	internal := classifyDependencyFailure(cause)
	if got.Code == internal.Code {
		t.Fatalf("classifyTargetSelectionFailure(*ValidationError) code = %d, want a caller-attributable rejection distinct from the generic internal error code %d", got.Code, internal.Code)
	}
}

func TestClassifyTargetSelectionFailureMapsUnknownCauseToInternalError(t *testing.T) {
	got := classifyTargetSelectionFailure(errors.New("boom"))
	want := classifyDependencyFailure(errors.New("boom"))
	if got.Code != want.Code {
		t.Fatalf("classifyTargetSelectionFailure(plain error) code = %d, want %d (generic internal error fallback)", got.Code, want.Code)
	}
}

func TestHandleSessionSetConfigOptionRejectsUnsupportedConfigIdBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		`{"sessionId":"session-1","configId":"model","value":"some-model"}`)

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for an unsupported configId")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for an unsupported configId")
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for an unsupported configId")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0", len(catalog.calls))
	}
}

func TestHandleSessionSetConfigOptionRejectsBooleanShapeForTargetOption(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		`{"sessionId":"session-1","configId":"target","type":"boolean","value":true}`)

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for a boolean payload against the target option")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a malformed option shape")
	}
}

func TestHandleSessionSetConfigOptionUnknownSessionRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionErr: &chatsessions.NotFoundError{Value: "Session", ID: "session-missing"}}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-missing", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for an unknown session")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for an unknown session")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for an unknown session", len(catalog.calls))
	}
}

func TestHandleSessionSetConfigOptionStaleVersionRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
		setTargetErr:     &chatsessions.ConflictError{Value: "Session", ID: "session-1", Expected: 3, Actual: 4},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for a stale expected version")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
}

func TestHandleSessionSetConfigOptionDisallowedTargetRejectsBeforeMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: "factory:@you/not-allowed",
		Err:    chatsessions.ErrFactoryTargetNotAllowed,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", "factory:@you/not-allowed"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for a disallowed target")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for a disallowed target")
	}
}

func TestHandleSessionSetConfigOptionWorkingRootIncompatibleTargetRejects(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: "factory:@you/pinned",
		Err:    chatsessions.ErrFactoryTargetWorkingRootIncompatible,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", "factory:@you/pinned"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for a working-root-incompatible target")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for a working-root-incompatible target")
	}
}

func TestHandleSessionSetConfigOptionFailureNeverLeaksRawValueOrRoot(t *testing.T) {
	sensitiveTarget := "factory:@you/sk_live_should_never_leak"
	sensitiveRoot := "/home/operator/should-never-leak"
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, sensitiveRoot)}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: sensitiveTarget,
		Err:    chatsessions.ErrFactoryTargetNotInstalled,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
		setConfigOptionParams("session-1", sensitiveTarget))

	_, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection")
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

func TestServeDispatchesSessionSetConfigOptionOverRealJSONRPCFraming(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	input := `{"jsonrpc":"2.0","id":1,"method":"session/set_config_option","params":` +
		setConfigOptionParams("session-1", "factory:@you/review") + `}` + "\n"
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

// TestServeRespondsMethodNotFoundForEveryUnimplementedMethodStillExcludesSessionSetConfigOption
// guards against a regression that would put "session/set_config_option"
// back on the unimplemented-methods list in
// TestServeRespondsMethodNotFoundForEveryUnimplementedMethod (server_test.go):
// a server with no session/set_config_option collaborators configured must
// still dispatch the method (and report a bounded internal failure), never
// method-not-found.
func TestServeRespondsMethodNotFoundForEveryUnimplementedMethodStillExcludesSessionSetConfigOption(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":9,"method":"session/set_config_option","params":{"sessionId":"s","configId":"target","value":"factory:@you/factory-builder"}}` + "\n"
	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	resp := assertSingleResponseLine(t, out)
	if resp.Error == nil {
		t.Fatal("error = nil, want a bounded failure for an unconfigured session/set_config_option server")
	}
	if resp.Error.Code == -32601 {
		t.Fatal("error code = method-not-found (-32601), want session/set_config_option to be dispatched, not rejected as unsupported")
	}
}
