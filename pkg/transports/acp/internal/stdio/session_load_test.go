package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

const validSessionLoadParams = `{"sessionId":"session-1","cwd":"/work/project","mcpServers":[]}`
const validSessionResumeParams = `{"sessionId":"session-1","cwd":"/work/project"}`

func sessionResumeParamsWithAttachmentID(t *testing.T, attachmentID string) string {
	t.Helper()
	params, err := json.Marshal(acpsdk.ResumeSessionRequest{
		SessionId: "session-1",
		Cwd:       "/work/project",
		Meta:      session.AttachmentResumeMetadata(attachmentID),
	})
	if err != nil {
		t.Fatalf("marshal session/resume params: %v", err)
	}
	return string(params)
}

func attachmentIDFromMeta(t *testing.T, meta map[string]any) string {
	t.Helper()
	attachmentID, ok := meta[session.AttachmentResumeMetaKey].(string)
	if !ok || attachmentID == "" {
		t.Fatalf("_meta[%q] = %#v, want a non-empty attachment identity", session.AttachmentResumeMetaKey, meta[session.AttachmentResumeMetaKey])
	}
	return attachmentID
}

// TestServeSessionLoadReplaysRetainedUpdatesBeforeItsResponse proves the ACP
// load ordering: a fresh connection receives the retained Chat aggregate as
// session/update notifications before the load response, with the exact item
// identity assigned at sequencing time. It exercises the real JSON-RPC Serve
// loop rather than only the handler so notification framing and response
// ordering cannot drift apart.
func TestServeSessionLoadReplaysRetainedUpdatesBeforeItsResponse(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.seedItem(t, streamingTestSessionID, "item-user", "", workers.KindMessage, workers.PhaseCompleted, workers.MessagePayload{
		Role:          "user",
		ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "retained question"}},
	})
	eventsSvc.seedItem(t, streamingTestSessionID, "item-assistant", "", workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("retained answer"))

	input := `{"jsonrpc":"2.0","id":1,"method":"session/load","params":{"sessionId":"` + streamingTestSessionID + `","cwd":"/work/project","mcpServers":[]}}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("Serve() emitted %d frames, want two retained updates then load response: %s", len(lines), out.String())
	}
	var userNotification notificationMessage
	if err := json.Unmarshal([]byte(lines[0]), &userNotification); err != nil {
		t.Fatalf("unmarshal retained user notification: %v", err)
	}
	if userNotification.Method != acpsdk.ClientMethodSessionUpdate {
		t.Fatalf("first frame method = %q, want %q", userNotification.Method, acpsdk.ClientMethodSessionUpdate)
	}
	if userNotification.Params.Update.UserMessageChunk == nil || userNotification.Params.Update.UserMessageChunk.MessageId == nil {
		t.Fatalf("first frame update = %+v, want an identified user message", userNotification.Params.Update)
	}
	if got := *userNotification.Params.Update.UserMessageChunk.MessageId; got != "item-user" {
		t.Fatalf("replayed user MessageId = %q, want original sequencer identity %q", got, "item-user")
	}
	var assistantNotification notificationMessage
	if err := json.Unmarshal([]byte(lines[1]), &assistantNotification); err != nil {
		t.Fatalf("unmarshal retained assistant notification: %v", err)
	}
	if assistantNotification.Params.Update.AgentMessageChunk == nil || assistantNotification.Params.Update.AgentMessageChunk.MessageId == nil {
		t.Fatalf("second frame update = %+v, want an identified agent message", assistantNotification.Params.Update)
	}
	if got := *assistantNotification.Params.Update.AgentMessageChunk.MessageId; got != "item-assistant" {
		t.Fatalf("replayed assistant MessageId = %q, want original sequencer identity %q", got, "item-assistant")
	}

	response := assertSingleResponseLine(t, bytes.NewBufferString(lines[2]+"\n"))
	if string(response.ID) != "1" || response.Error != nil {
		t.Fatalf("load response = %+v, want successful response id 1", response)
	}
}

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
	server := New(nil, nil, nil, nil, nil, nil, nil, nil)

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
	if got := attachmentIDFromMeta(t, resp.Meta); got != cached.ID {
		t.Fatalf("session/load response attachment identity = %q, want cached attachment %q", got, cached.ID)
	}
}

// TestHandleSessionLoadRetryKeepsItsClosedSessionAttachment proves that a
// completed load remains idempotent after close: the load-specific attachment
// is still the cursor that has acknowledged the retained history, so retrying
// must not create a new cursor that replays it a second time.
func TestHandleSessionLoadRetryKeepsItsClosedSessionAttachment(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project")}
	server := newTestServer(chatSessions, nil, "/home/operator")
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionLoad, validSessionLoadParams)

	result, rpcErr := server.handleSessionLoad(ctx, env)
	if rpcErr != nil {
		t.Fatalf("first handleSessionLoad() error = %+v, want success", rpcErr)
	}
	var first acpsdk.LoadSessionResponse
	if err := json.Unmarshal(result, &first); err != nil {
		t.Fatalf("unmarshal first LoadSessionResponse: %v", err)
	}
	firstID := attachmentIDFromMeta(t, first.Meta)

	chatSessions.getSessionResult.Session.State = chatsessions.SessionStateClosed
	result, rpcErr = server.handleSessionLoad(ctx, env)
	if rpcErr != nil {
		t.Fatalf("retried handleSessionLoad() error = %+v, want success", rpcErr)
	}
	var retried acpsdk.LoadSessionResponse
	if err := json.Unmarshal(result, &retried); err != nil {
		t.Fatalf("unmarshal retried LoadSessionResponse: %v", err)
	}
	if got := attachmentIDFromMeta(t, retried.Meta); got != firstID {
		t.Fatalf("retried session/load attachment identity = %q, want original %q", got, firstID)
	}
	if got := len(chatSessions.attachments); got != 1 {
		t.Fatalf("Attach created %d attachments, want one retained-history cursor", got)
	}
}

// TestHandleSessionLoadSuppressesSuccessMetadataWhenReplayCannotStart proves
// a client never receives a successful load response with an attachment
// identity when either its attachment or the retained-history read fails.
// Returning that identity would falsely imply that the requested history had
// been delivered and could cause the client to acknowledge records it never
// observed.
func TestHandleSessionLoadSuppressesSuccessMetadataWhenReplayCannotStart(t *testing.T) {
	tests := []struct {
		name   string
		server func(t *testing.T) *Server
	}{
		{
			name: "attachment failure",
			server: func(t *testing.T) *Server {
				t.Helper()
				return newTestServer(&fakeChatSessionsService{
					getSessionResult: sessionAt("session-1", "factory:@you/review", 3, "/work/project"),
					attachErr:        errors.New("attach retained-history cursor"),
				}, nil, "/home/operator")
			},
		},
		{
			name: "retained history read failure",
			server: func(t *testing.T) *Server {
				t.Helper()
				server, eventsSvc := newStreamingTestServer(t, &fakeFactoryTargetService{})
				eventsSvc.readErr = errors.New("read retained history")
				return server
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionLoad, validSessionLoadParams)
			result, rpcErr := tt.server(t).handleSessionLoad(context.Background(), env)
			if rpcErr == nil {
				t.Fatal("handleSessionLoad() error = nil, want a bounded failure")
			}
			if result != nil {
				t.Fatalf("handleSessionLoad() result = %s, want no successful attachment metadata", result)
			}
		})
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
	env := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionResume, sessionResumeParamsWithAttachmentID(t, original.ID))

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

// TestServeSessionResumeRestoresTwoDetachedAttachmentsByReturnedMetaIdentity
// drives two independent consumers through the real stdio -> session/load ->
// response _meta -> session/resume route. Their positions intentionally
// differ before disconnect, then each client returns only the opaque identity
// the server issued to it. This proves the customer-facing ACP path never
// guesses from detached attachments, replays an already acknowledged cursor,
// or gives one consumer the other's cursor when both reconnect.
func TestServeSessionResumeRestoresTwoDetachedAttachmentsByReturnedMetaIdentity(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	server := newTestServer(chatSessions, nil, "/home/operator")

	serveForAttachmentID := func(method, params string) string {
		t.Helper()
		input := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":` + params + `}` + "\n"
		out := &bytes.Buffer{}
		if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
			t.Fatalf("Serve(%s) error = %v", method, err)
		}
		response := assertSingleResponseLine(t, out)
		if response.Error != nil {
			t.Fatalf("Serve(%s) response error = %+v", method, response.Error)
		}
		var result struct {
			Meta map[string]any `json:"_meta"`
		}
		if err := json.Unmarshal(response.Result, &result); err != nil {
			t.Fatalf("unmarshal %s result: %v", method, err)
		}
		return attachmentIDFromMeta(t, result.Meta)
	}

	firstID := serveForAttachmentID(acpsdk.AgentMethodSessionLoad, validSessionLoadParams)
	if _, err := chatSessions.AcknowledgeAttachment(context.Background(), chatsessions.AcknowledgeAttachmentRequest{
		SessionID: "session-1", AttachmentID: firstID, AfterSequence: 3,
	}); err != nil {
		t.Fatalf("acknowledge first attachment: %v", err)
	}

	secondID := serveForAttachmentID(acpsdk.AgentMethodSessionLoad, validSessionLoadParams)
	if secondID == firstID {
		t.Fatalf("second session/load identity = %q, want an independent attachment", secondID)
	}
	if _, err := chatSessions.AcknowledgeAttachment(context.Background(), chatsessions.AcknowledgeAttachmentRequest{
		SessionID: "session-1", AttachmentID: secondID, AfterSequence: 7,
	}); err != nil {
		t.Fatalf("acknowledge second attachment: %v", err)
	}

	if got := serveForAttachmentID(acpsdk.AgentMethodSessionResume, sessionResumeParamsWithAttachmentID(t, secondID)); got != secondID {
		t.Fatalf("second reconnect identity = %q, want %q", got, secondID)
	}
	if got := serveForAttachmentID(acpsdk.AgentMethodSessionResume, sessionResumeParamsWithAttachmentID(t, firstID)); got != firstID {
		t.Fatalf("first reconnect identity = %q, want %q", got, firstID)
	}

	chatSessions.mu.Lock()
	first := chatSessions.attachments[firstID]
	second := chatSessions.attachments[secondID]
	chatSessions.mu.Unlock()
	if first.AfterSequence != 3 {
		t.Fatalf("first attachment AfterSequence = %d, want 3", first.AfterSequence)
	}
	if second.AfterSequence != 7 {
		t.Fatalf("second attachment AfterSequence = %d, want 7", second.AfterSequence)
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
