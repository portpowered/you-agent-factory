package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// fakeChatSessionsService is a minimal chatsessions.Service test double.
// CreateSession, GetSession, and SetTarget are configurable and tracked --
// the methods this package's session/new and session/set_config_option
// handlers actually call. Every other method fails loudly: neither handler
// calls them, and a call to one would itself be a defect worth catching.
type fakeChatSessionsService struct {
	createCalled bool
	created      chatsessions.CreateSessionRequest
	createErr    error
	sessionID    string

	getSessionCalled bool
	getSessionReq    chatsessions.GetSessionRequest
	getSessionResult chatsessions.GetSessionResult
	getSessionErr    error

	setTargetCalled bool
	setTargetReq    chatsessions.SetTargetRequest
	setTargetResult chatsessions.SetTargetResult
	setTargetErr    error

	startTurnCalled bool
}

var _ chatsessions.Service = (*fakeChatSessionsService)(nil)

func (f *fakeChatSessionsService) CreateSession(_ context.Context, req chatsessions.CreateSessionRequest) (chatsessions.CreateSessionResult, error) {
	f.createCalled = true
	f.created = req
	if f.createErr != nil {
		return chatsessions.CreateSessionResult{}, f.createErr
	}
	id := f.sessionID
	if id == "" {
		id = "session-1"
	}
	now := time.Unix(0, 1)
	return chatsessions.CreateSessionResult{Session: chatsessions.Session{
		ID:             id,
		State:          chatsessions.SessionStateCreated,
		SelectedTarget: req.InitialTarget,
		WorkingRoot:    req.WorkingRoot,
		CreatedAt:      now,
		UpdatedAt:      now,
	}}, nil
}

func (f *fakeChatSessionsService) GetSession(_ context.Context, req chatsessions.GetSessionRequest) (chatsessions.GetSessionResult, error) {
	f.getSessionCalled = true
	f.getSessionReq = req
	if f.getSessionErr != nil {
		return chatsessions.GetSessionResult{}, f.getSessionErr
	}
	return f.getSessionResult, nil
}

func (f *fakeChatSessionsService) SetTarget(_ context.Context, req chatsessions.SetTargetRequest) (chatsessions.SetTargetResult, error) {
	f.setTargetCalled = true
	f.setTargetReq = req
	if f.setTargetErr != nil {
		return chatsessions.SetTargetResult{}, f.setTargetErr
	}
	return f.setTargetResult, nil
}

func (f *fakeChatSessionsService) StartTurn(context.Context, chatsessions.StartTurnRequest) (chatsessions.StartTurnResult, error) {
	f.startTurnCalled = true
	return chatsessions.StartTurnResult{}, errors.New("fakeChatSessionsService: StartTurn not implemented")
}

func (f *fakeChatSessionsService) AdvanceTurn(context.Context, chatsessions.AdvanceTurnRequest) (chatsessions.AdvanceTurnResult, error) {
	return chatsessions.AdvanceTurnResult{}, errors.New("fakeChatSessionsService: AdvanceTurn not implemented")
}

func (f *fakeChatSessionsService) Attach(context.Context, chatsessions.AttachRequest) (chatsessions.AttachResult, error) {
	return chatsessions.AttachResult{}, errors.New("fakeChatSessionsService: Attach not implemented")
}

func (f *fakeChatSessionsService) Detach(context.Context, chatsessions.DetachRequest) (chatsessions.DetachResult, error) {
	return chatsessions.DetachResult{}, errors.New("fakeChatSessionsService: Detach not implemented")
}

func (f *fakeChatSessionsService) RequestControl(context.Context, chatsessions.RequestControlRequest) (chatsessions.RequestControlResult, error) {
	return chatsessions.RequestControlResult{}, errors.New("fakeChatSessionsService: RequestControl not implemented")
}

func (f *fakeChatSessionsService) AdvanceControl(context.Context, chatsessions.AdvanceControlRequest) (chatsessions.AdvanceControlResult, error) {
	return chatsessions.AdvanceControlResult{}, errors.New("fakeChatSessionsService: AdvanceControl not implemented")
}

// fakeFactoryTargetCatalogService is a minimal
// chatsessions.FactoryTargetCatalogService test double.
type fakeFactoryTargetCatalogService struct {
	calls  []chatsessions.ResolveFactoryTargetCatalogRequest
	result chatsessions.ResolveFactoryTargetCatalogResult
	err    error
}

var _ chatsessions.FactoryTargetCatalogService = (*fakeFactoryTargetCatalogService)(nil)

func (f *fakeFactoryTargetCatalogService) ResolveFactoryTargetCatalog(
	_ context.Context,
	req chatsessions.ResolveFactoryTargetCatalogRequest,
) (chatsessions.ResolveFactoryTargetCatalogResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, f.err
	}
	return f.result, nil
}

func defaultTestCatalogResult() chatsessions.ResolveFactoryTargetCatalogResult {
	return chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/factory-builder",
		Choices: []chatsessions.FactoryTargetCatalogChoice{
			{Value: "factory:@you/factory-builder", Name: "Factory Builder"},
			{Value: "factory:@you/review", Name: "Review"},
		},
	}
}

func newTestServer(chatSessions *fakeChatSessionsService, catalog *fakeFactoryTargetCatalogService, homeDir string) *Server {
	resolveHomeDir := func() (string, error) { return homeDir, nil }
	return New(nil, chatSessions, catalog, resolveHomeDir)
}

func numberIdentityEnvelope(t *testing.T, connID identity.ConnectionID, wireID int64, method string, params string) envelope.Envelope {
	t.Helper()
	reqIdentity, err := identity.NewCorrelated(connID, identity.NewNumberJSONRPCID(wireID))
	if err != nil {
		t.Fatalf("NewCorrelated: %v", err)
	}
	return envelope.Envelope{Identity: reqIdentity, Method: method, Params: json.RawMessage(params)}
}

func stringIdentityEnvelope(t *testing.T, connID identity.ConnectionID, wireID string, method string, params string) envelope.Envelope {
	t.Helper()
	reqIdentity, err := identity.NewCorrelated(connID, identity.NewStringJSONRPCID(wireID))
	if err != nil {
		t.Fatalf("NewCorrelated: %v", err)
	}
	return envelope.Envelope{Identity: reqIdentity, Method: method, Params: json.RawMessage(params)}
}

const validSessionNewParams = `{"cwd":"/work/project","mcpServers":[]}`

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
	server := New(nil, nil, nil, nil)
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
	server := New(nil, nil, nil, nil)
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)

	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a bounded failure when collaborators are unset")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil", result)
	}
}

// mintedIdentityEnvelope builds an envelope carrying a transport-minted
// (non-connection-correlated) identity, the one RequestIdentity shape
// chatRequestIdentity always rejects: "session/new" and "session/prompt" are
// always real inbound JSON-RPC requests, so they only ever see a correlated
// identity in production, but the boundary must still fail closed rather
// than panic or fabricate a connection if it ever saw one.
func mintedIdentityEnvelope(t *testing.T, method string, params string) envelope.Envelope {
	t.Helper()
	minted, err := identity.NewMinted("transport-minted-id")
	if err != nil {
		t.Fatalf("identity.NewMinted: %v", err)
	}
	return envelope.Envelope{Identity: minted, Method: method, Params: json.RawMessage(params)}
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
	server := New(nil, chatSessions, catalog, func() (string, error) { return "", errors.New("resolve home dir boom") })

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
	server := New(nil, chatSessions, catalog, func() (string, error) { return "", nil })

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
