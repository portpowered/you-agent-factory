package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	acpsdk "github.com/coder/acp-go-sdk"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
	"strings"
	"testing"
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
