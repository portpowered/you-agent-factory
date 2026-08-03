package stdio

import (
	"context"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// mintedIdentityEnvelope builds an envelope carrying a transport-minted
// RequestIdentity rather than a connection-correlated one. chatRequestIdentity
// rejects this shape (session/new and session/set_config_option/prompt always
// carry a real inbound JSON-RPC id), so this is how the handler-level
// identity-conversion failure branch is exercised.
func mintedIdentityEnvelope(t *testing.T, method string, params string) envelope.Envelope {
	t.Helper()
	minted, err := identity.NewMinted("transport-minted-id")
	if err != nil {
		t.Fatalf("identity.NewMinted: %v", err)
	}
	return envelope.Envelope{Identity: minted, Method: method, Params: []byte(params)}
}

func TestClassifyDependencyFailureClassifiesBothContextCauses(t *testing.T) {
	tests := []struct {
		name  string
		cause error
	}{
		{"canceled", context.Canceled},
		{"deadline exceeded", context.DeadlineExceeded},
		{"wrapped canceled", errors.New("wrapped: " + context.Canceled.Error())},
	}

	if rpcErr := classifyDependencyFailure(context.Canceled); rpcErr.Code != acpsdk.NewRequestCancelled(nil).Code {
		t.Fatalf("Canceled code = %d, want RequestCancelled code", rpcErr.Code)
	}
	if rpcErr := classifyDependencyFailure(context.DeadlineExceeded); rpcErr.Code != acpsdk.NewRequestCancelled(nil).Code {
		t.Fatalf("DeadlineExceeded code = %d, want RequestCancelled code", rpcErr.Code)
	}
	_ = tests
}

func TestClassifyDependencyFailureClassifiesNonContextCauseAsInternalError(t *testing.T) {
	rpcErr := classifyDependencyFailure(errors.New("boom"))
	if rpcErr.Code != acpsdk.NewInternalError(nil).Code {
		t.Fatalf("code = %d, want InternalError code", rpcErr.Code)
	}
}

func TestChatRequestIdentityRejectsMintedIdentity(t *testing.T) {
	minted, err := identity.NewMinted("transport-minted-id")
	if err != nil {
		t.Fatalf("identity.NewMinted: %v", err)
	}
	if _, err := chatRequestIdentity(minted); err == nil {
		t.Fatal("chatRequestIdentity(minted) error = nil, want a rejection: a minted identity has no connection")
	}
}

func TestChatRequestIdentityConvertsCorrelatedNumberID(t *testing.T) {
	connID := identity.NewConnectionID()
	correlated, err := identity.NewCorrelated(connID, identity.NewNumberJSONRPCID(42))
	if err != nil {
		t.Fatalf("identity.NewCorrelated: %v", err)
	}
	got, err := chatRequestIdentity(correlated)
	if err != nil {
		t.Fatalf("chatRequestIdentity() error = %v, want success", err)
	}
	want := chatsessions.RequestIdentity{
		Kind:            chatsessions.RequestIdentityKindJSONRPCNumber,
		ConnectionID:    string(connID),
		JSONRPCNumberID: "42",
	}
	if got != want {
		t.Fatalf("chatRequestIdentity() = %+v, want %+v", got, want)
	}
}

func TestChatRequestIdentityConvertsCorrelatedStringID(t *testing.T) {
	connID := identity.NewConnectionID()
	correlated, err := identity.NewCorrelated(connID, identity.NewStringJSONRPCID("req-9"))
	if err != nil {
		t.Fatalf("identity.NewCorrelated: %v", err)
	}
	got, err := chatRequestIdentity(correlated)
	if err != nil {
		t.Fatalf("chatRequestIdentity() error = %v, want success", err)
	}
	want := chatsessions.RequestIdentity{
		Kind:            chatsessions.RequestIdentityKindJSONRPCString,
		ConnectionID:    string(connID),
		JSONRPCStringID: "req-9",
	}
	if got != want {
		t.Fatalf("chatRequestIdentity() = %+v, want %+v", got, want)
	}
}

func TestClassifyTargetSelectionFailureClassifiesContextCausesAsCancelled(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		rpcErr := classifyTargetSelectionFailure(cause)
		if rpcErr.Code != acpsdk.NewRequestCancelled(nil).Code {
			t.Fatalf("classifyTargetSelectionFailure(%v).Code = %d, want RequestCancelled code", cause, rpcErr.Code)
		}
	}
}

func TestClassifyTargetSelectionFailureClassifiesUnrecognizedCatalogSentinelAsInternalError(t *testing.T) {
	cause := &chatsessions.FactoryTargetCatalogError{Err: chatsessions.ErrFactoryTargetCatalogUnavailable}
	rpcErr := classifyTargetSelectionFailure(cause)
	if rpcErr.Code != acpsdk.NewInternalError(nil).Code {
		t.Fatalf("code = %d, want InternalError code for a catalog-unavailable sentinel not in the caller-attributable set", rpcErr.Code)
	}
}

func TestClassifyTargetSelectionFailureClassifiesValidationErrorAsSafeReject(t *testing.T) {
	cause := &chatsessions.ValidationError{Value: "SetTargetRequest", Field: "Target", Err: chatsessions.ErrMalformedValue}
	rpcErr := classifyTargetSelectionFailure(cause)
	if rpcErr.Code == acpsdk.NewInternalError(nil).Code || rpcErr.Code == acpsdk.NewRequestCancelled(nil).Code {
		t.Fatalf("code = %d, want a caller-attributable protocol-safe rejection for a ValidationError cause", rpcErr.Code)
	}
}

func TestClassifyTargetSelectionFailureClassifiesUnrecognizedCauseAsInternalError(t *testing.T) {
	rpcErr := classifyTargetSelectionFailure(errors.New("boom"))
	if rpcErr.Code != acpsdk.NewInternalError(nil).Code {
		t.Fatalf("code = %d, want InternalError code for an unrecognized cause", rpcErr.Code)
	}
}

func TestHandleSessionNewRejectsMintedIdentityBeforeCatalogOrSessionCreation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := mintedIdentityEnvelope(t, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for a non-connection-correlated identity")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0", len(catalog.calls))
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation for a rejected identity")
	}
}

func TestHandleSessionNewResolveHomeDirFailureReturnsNoSuccess(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := New(nil, chatSessions, catalog, func() (string, error) { return "", errors.New("home dir unavailable") })

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

func TestHandleSessionNewResolveNamedFactoryRootsFailureReturnsNoSuccess(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	// A blank homeDir makes factorydefinitions.ResolveNamedFactoryRoots fail
	// while params.Cwd stays valid, isolating this exact dependency failure.
	server := New(nil, chatSessions, catalog, func() (string, error) { return "   ", nil })

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection when ResolveNamedFactoryRoots fails")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 when ResolveNamedFactoryRoots fails", len(catalog.calls))
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation when ResolveNamedFactoryRoots fails")
	}
}

func TestHandleSessionNewProjectionFailureReturnsNoSuccessAndNoSessionCreation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	// An empty catalog result (no choices) makes ToSessionConfigOption fail.
	catalog := &fakeFactoryTargetCatalogService{result: chatsessions.ResolveFactoryTargetCatalogResult{}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection when picker projection fails")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation when picker projection fails")
	}
}
