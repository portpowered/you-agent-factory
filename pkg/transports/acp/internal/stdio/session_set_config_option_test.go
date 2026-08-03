package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// setConfigOptionParams builds a raw session/set_config_option value-id
// params payload addressing the Factory target select option.
func setConfigOptionParams(sessionID, value string) string {
	return `{"sessionId":"` + sessionID + `","configId":"target","value":"` + value + `"}`
}

// sessionAt builds a GetSessionResult for a session addressed by id, with the
// given selected target, version, and WorkingRoot -- mirroring what a prior
// successful "session/new" call would have produced.
func sessionAt(id string, target string, version uint64, workingRoot string) chatsessions.GetSessionResult {
	now := time.Unix(0, 1)
	return chatsessions.GetSessionResult{Session: chatsessions.Session{
		ID:    id,
		State: chatsessions.SessionStateCreated,
		SelectedTarget: chatsessions.ChatTargetRef{
			Kind: chatsessions.ChatTargetKindFactory,
			Ref:  target,
		},
		Version:     version,
		WorkingRoot: workingRoot,
		CreatedAt:   now,
		UpdatedAt:   now,
	}}
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
	server := New(nil, chatSessions, catalog, func() (string, error) { return "", errors.New("resolve home dir boom") })

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
	server := New(nil, chatSessions, catalog, func() (string, error) { return "", nil })

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
	server := New(nil, nil, nil, nil)
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

func catalogResultWithCurrent(current string) chatsessions.ResolveFactoryTargetCatalogResult {
	return chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: current,
		Choices: []chatsessions.FactoryTargetCatalogChoice{
			{Value: "factory:@you/factory-builder", Name: "Factory Builder"},
			{Value: "factory:@you/review", Name: "Review"},
		},
	}
}
