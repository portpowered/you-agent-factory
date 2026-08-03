package stdio

import (
	"context"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

func TestHandleSessionSetConfigOptionRejectsMalformedJSONBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption, `{not valid json`)

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for malformed JSON params")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for malformed JSON params")
	}
}

func TestHandleSessionSetConfigOptionRejectsMintedIdentityBeforeAnyMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := mintedIdentityEnvelope(t, acpsdk.AgentMethodSessionSetConfigOption, setConfigOptionParams("session-1", "factory:@you/review"))

	result, rpcErr := server.handleSessionSetConfigOption(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection for a non-connection-correlated identity")
	}
	if result != nil {
		t.Fatalf("handleSessionSetConfigOption() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a rejected identity")
	}
}

func TestChangeTargetResolveHomeDirFailureReturnsNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := New(nil, chatSessions, catalog, func() (string, error) { return "", errors.New("home dir unavailable") })

	_, rpcErr := server.changeTarget(context.Background(), "session-1", "factory:@you/review", chatsessions.RequestIdentity{
		Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-1", JSONRPCNumberID: "1",
	})
	if rpcErr == nil {
		t.Fatal("changeTarget() error = nil, want a rejection when resolveHomeDir fails")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 when resolveHomeDir fails", len(catalog.calls))
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation when resolveHomeDir fails")
	}
}

func TestChangeTargetResolveNamedFactoryRootsFailureReturnsNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	// A blank homeDir makes factorydefinitions.ResolveNamedFactoryRoots fail
	// while the session's own WorkingRoot stays valid.
	server := New(nil, chatSessions, catalog, func() (string, error) { return "   ", nil })

	_, rpcErr := server.changeTarget(context.Background(), "session-1", "factory:@you/review", chatsessions.RequestIdentity{
		Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-1", JSONRPCNumberID: "1",
	})
	if rpcErr == nil {
		t.Fatal("changeTarget() error = nil, want a rejection when ResolveNamedFactoryRoots fails")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 when ResolveNamedFactoryRoots fails", len(catalog.calls))
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation when ResolveNamedFactoryRoots fails")
	}
}

func TestChangeTargetWorkingRootUnknownReturnsNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	_, rpcErr := server.changeTarget(context.Background(), "session-1", "factory:@you/review", chatsessions.RequestIdentity{
		Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-1", JSONRPCNumberID: "1",
	})
	if rpcErr == nil {
		t.Fatal("changeTarget() error = nil, want a rejection when the addressed session has an unknown working root")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 when the session's working root is unknown", len(catalog.calls))
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation when the session's working root is unknown")
	}
}

func TestChangeTargetProjectionFailureReturnsNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	// An empty catalog result (no choices) makes ToSessionConfigOption fail
	// after SetTarget has already been called and succeeded upstream.
	catalog := &fakeFactoryTargetCatalogService{result: chatsessions.ResolveFactoryTargetCatalogResult{}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	_, rpcErr := server.changeTarget(context.Background(), "session-1", "factory:@you/review", chatsessions.RequestIdentity{
		Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "conn-1", JSONRPCNumberID: "1",
	})
	if rpcErr == nil {
		t.Fatal("changeTarget() error = nil, want a rejection when picker projection fails")
	}
}

func TestHandleSessionPromptRejectsMalformedJSONBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt, `{not valid json`)

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for malformed JSON params")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for malformed JSON params")
	}
}

func TestHandleSessionPromptRejectsInvalidPromptBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	// A blank sessionId fails session.ValidatePrompt before this transport
	// ever inspects the prompt content for a "/factory" command.
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt, promptTextParams("", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for an invalid prompt request")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for an invalid prompt request")
	}
}

func TestHandleSessionPromptRejectsMintedIdentityBeforeAnyMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := mintedIdentityEnvelope(t, acpsdk.AgentMethodSessionPrompt, promptTextParams("session-1", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a non-connection-correlated identity")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a rejected identity")
	}
}
