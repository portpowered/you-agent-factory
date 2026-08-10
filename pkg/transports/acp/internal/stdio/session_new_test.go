// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
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
