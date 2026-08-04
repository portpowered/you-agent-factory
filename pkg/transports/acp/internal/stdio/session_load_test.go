package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

const validSessionLoadParams = `{"sessionId":"session-1","cwd":"/work/project","mcpServers":[]}`
const validSessionResumeParams = `{"sessionId":"session-1","cwd":"/work/project"}`

// TestHandleSessionLoadAndResumeRejectMissingSessionIdBeforeAnyEffect proves
// both handlers validate before ever touching a collaborator: a request
// missing "sessionId" is rejected and GetSession/Attach are never called.
func TestHandleSessionLoadAndResumeRejectMissingSessionIdBeforeAnyEffect(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		params  string
		handler func(*Server, context.Context, envelope.Envelope) (json.RawMessage, *acpsdk.RequestError)
	}{
		{"load", acpsdk.AgentMethodSessionLoad, `{"cwd":"/work/project","mcpServers":[]}`, (*Server).handleSessionLoad},
		{"resume", acpsdk.AgentMethodSessionResume, `{"cwd":"/work/project"}`, (*Server).handleSessionResume},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatSessions := &fakeChatSessionsService{}
			server := newTestServer(chatSessions, nil, "/home/operator")
			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, tt.method, tt.params)

			result, rpcErr := tt.handler(server, context.Background(), env)
			if rpcErr == nil {
				t.Fatal("got nil error, want a rejection for a missing sessionId")
			}
			if result != nil {
				t.Fatalf("result = %q, want nil on rejection", result)
			}
			if chatSessions.getSessionCalled {
				t.Fatal("GetSession was called, want no effect for a rejected request")
			}
		})
	}
}

// TestHandleSessionLoadAndResumeUnknownSessionReturnsBoundedRejection proves
// an unknown sessionId (*chatsessions.NotFoundError from GetSession)
// classifies as a bounded rejection, never a raw internal error leaking the
// cause, and never reaches Attach.
func TestHandleSessionLoadAndResumeUnknownSessionReturnsBoundedRejection(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		params  string
		handler func(*Server, context.Context, envelope.Envelope) (json.RawMessage, *acpsdk.RequestError)
	}{
		{"load", acpsdk.AgentMethodSessionLoad, validSessionLoadParams, (*Server).handleSessionLoad},
		{"resume", acpsdk.AgentMethodSessionResume, validSessionResumeParams, (*Server).handleSessionResume},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatSessions := &fakeChatSessionsService{getSessionErr: &chatsessions.NotFoundError{Value: "Session", ID: "session-1"}}
			server := newTestServer(chatSessions, nil, "/home/operator")
			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, tt.method, tt.params)

			result, rpcErr := tt.handler(server, context.Background(), env)
			if rpcErr == nil {
				t.Fatal("got nil error, want a rejection for an unknown session")
			}
			if result != nil {
				t.Fatalf("result = %q, want nil on rejection", result)
			}
			if rpcErr.Code == -32603 {
				t.Fatalf("error code = %d (internal error), want a bounded invalid-params rejection for the caller's own unknown session", rpcErr.Code)
			}
		})
	}
}

// TestHandleSessionLoadAndResumeWithoutCollaboratorsReportsBoundedFailure
// proves a Server constructed without the Chat Sessions collaborator reports
// a bounded failure rather than panicking.
func TestHandleSessionLoadAndResumeWithoutCollaboratorsReportsBoundedFailure(t *testing.T) {
	server := New(nil, nil, nil, nil, nil, nil, nil)

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionLoad, validSessionLoadParams)
	if _, rpcErr := server.handleSessionLoad(context.Background(), env); rpcErr == nil {
		t.Fatal("handleSessionLoad() error = nil, want a bounded failure when collaborators are unset")
	}

	env = numberIdentityEnvelope(t, identity.NewConnectionID(), 2, acpsdk.AgentMethodSessionResume, validSessionResumeParams)
	if _, rpcErr := server.handleSessionResume(context.Background(), env); rpcErr == nil {
		t.Fatal("handleSessionResume() error = nil, want a bounded failure when collaborators are unset")
	}
}

// TestHandleSessionLoadAndResumeRejectNonCorrelatedIdentity proves a
// transport-minted (non-connection-correlated) identity is rejected rather
// than silently attaching with a blank connection id.
func TestHandleSessionLoadAndResumeRejectNonCorrelatedIdentity(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	server := newTestServer(chatSessions, nil, "/home/operator")

	env := mintedIdentityEnvelope(t, acpsdk.AgentMethodSessionLoad, validSessionLoadParams)
	if _, rpcErr := server.handleSessionLoad(context.Background(), env); rpcErr == nil {
		t.Fatal("handleSessionLoad() error = nil, want a rejection for a non-correlated identity")
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a rejected identity")
	}
}

// TestHandleSessionLoadAttachesAndCachesAResumableAttachment proves a
// successful "session/load" eagerly attaches (Resume: true, Interactive:
// true) for this connection and records the result in this connection's
// attachmentCache, so a later "session/prompt" on the same connection reuses
// it through ensureAttachment's cache-hit path instead of attaching again.
func TestHandleSessionLoadAttachesAndCachesAResumableAttachment(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	server := newTestServer(chatSessions, nil, "/home/operator")

	connID := identity.NewConnectionID()
	env := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionLoad, validSessionLoadParams)
	cache := &attachmentCache{}
	ctx := contextWithAttachmentCache(context.Background(), cache)

	result, rpcErr := server.handleSessionLoad(ctx, env)
	if rpcErr != nil {
		t.Fatalf("handleSessionLoad() error = %+v, want success", rpcErr)
	}
	var resp acpsdk.LoadSessionResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal LoadSessionResponse: %v", err)
	}

	if len(chatSessions.detachCalls) != 0 {
		t.Fatalf("Detach was called %d times, want 0", len(chatSessions.detachCalls))
	}
	cached, ok := cache.get("session-1")
	if !ok {
		t.Fatal("session/load did not populate this connection's attachmentCache")
	}
	if !cached.Interactive {
		t.Fatal("cached attachment Interactive = false, want true")
	}
	if cached.ConnectionID != string(connID) {
		t.Fatalf("cached attachment ConnectionID = %q, want %q", cached.ConnectionID, connID)
	}
}

// TestHandleSessionResumeReactivatesAttachmentDetachedByAnEarlierConnection
// is the end-to-end proof this story's AC2 requires: a first connection
// attaches (via ensureAttachment, the same path "session/prompt" streaming
// uses) and detaches (simulating disconnect), then a second, independent
// connection's "session/resume" call reactivates that exact same attachment
// -- same ID, same AfterSequence -- instead of minting a fresh one at
// position zero.
func TestHandleSessionResumeReactivatesAttachmentDetachedByAnEarlierConnection(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	server := newTestServer(chatSessions, nil, "/home/operator")

	firstCache := &attachmentCache{}
	firstCtx := contextWithAttachmentCache(context.Background(), firstCache)
	original, ok, err := server.ensureAttachment(firstCtx, "conn-first", "session-1")
	if err != nil || !ok {
		t.Fatalf("ensureAttachment (first connection): ok=%v err=%v", ok, err)
	}
	if _, err := chatSessions.AcknowledgeAttachment(firstCtx, chatsessions.AcknowledgeAttachmentRequest{
		SessionID: "session-1", AttachmentID: original.ID, ExpectedVersion: 0, AfterSequence: 5,
	}); err != nil {
		t.Fatalf("AcknowledgeAttachment: %v", err)
	}
	server.detachAttachments(context.Background(), firstCache)

	secondCache := &attachmentCache{}
	secondCtx := contextWithAttachmentCache(context.Background(), secondCache)
	connID := identity.NewConnectionID()
	env := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionResume, validSessionResumeParams)

	if _, rpcErr := server.handleSessionResume(secondCtx, env); rpcErr != nil {
		t.Fatalf("handleSessionResume() error = %+v, want success", rpcErr)
	}

	resumed, ok := secondCache.get("session-1")
	if !ok {
		t.Fatal("session/resume did not populate the second connection's attachmentCache")
	}
	if resumed.ID != original.ID {
		t.Fatalf("resumed attachment ID = %q, want the original attachment %q reactivated", resumed.ID, original.ID)
	}
	if resumed.AfterSequence != 5 {
		t.Fatalf("resumed AfterSequence = %d, want the preserved position 5", resumed.AfterSequence)
	}
	if resumed.ConnectionID != string(connID) {
		t.Fatalf("resumed ConnectionID = %q, want the new connection %q", resumed.ConnectionID, connID)
	}
}

// TestServeDispatchesSessionLoadAndResumeOverRealJSONRPCFraming proves both
// methods are reachable through the full Serve/JSON-RPC path, not just the
// handler called directly.
func TestServeDispatchesSessionLoadAndResumeOverRealJSONRPCFraming(t *testing.T) {
	tests := []struct {
		name   string
		method string
		params string
	}{
		{"load", "session/load", validSessionLoadParams},
		{"resume", "session/resume", validSessionResumeParams},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatSessions := &fakeChatSessionsService{}
			server := newTestServer(chatSessions, nil, "/home/operator")

			input := `{"jsonrpc":"2.0","id":1,"method":"` + tt.method + `","params":` + tt.params + `}` + "\n"
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
			if !chatSessions.getSessionCalled {
				t.Fatal("GetSession was not called over the real Serve path")
			}
		})
	}
}
