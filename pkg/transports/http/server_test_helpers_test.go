package http

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	cursorstorage "github.com/portpowered/infinite-you/pkg/platform/cursors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

const defaultSessionWorkAPIPrefix = "/factory-sessions/" + factorysessions.DefaultSessionID

func scopeDefaultSessionWorkPath(path string) string {
	if path == "/work" || strings.HasPrefix(path, "/work?") || strings.HasPrefix(path, "/work/") {
		return defaultSessionWorkAPIPrefix + path
	}
	if strings.HasPrefix(path, "/work-requests/") {
		return defaultSessionWorkAPIPrefix + path
	}
	return path
}

func newTestServer(f *testutil.MockFactory) *Server {
	if f != nil {
		ensureMockFactoryCurrentFactory(f)
	}
	logger, _ := zap.NewDevelopment()
	return NewServer(f, 8080, logger)
}

func ensureMockFactoryCurrentFactory(f *testutil.MockFactory) {
	if f.CurrentFactoryErr != nil || f.CurrentFactory != nil {
		return
	}
	f.CurrentFactory = &factoryapi.Factory{Name: "test-factory"}
}

func newTestServerWithCodexRoot(root string) *Server {
	logger, _ := zap.NewDevelopment()
	return NewServerWithOptions(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*factorytoken.Token),
		},
	}, 8080, logger, ServerOptions{CodexSessionsRoot: root})
}

func newTestServerWithUnavailableCursorRoot(t *testing.T) *Server {
	t.Helper()
	missingRoot := filepath.Join(t.TempDir(), "cursor-root-unavailable")
	return newTestServerWithCursorRoot(missingRoot)
}

func newTestServerWithCursorRoot(root string) *Server {
	logger, _ := zap.NewDevelopment()
	return NewServerWithOptions(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*factorytoken.Token),
		},
	}, 8080, logger, ServerOptions{CursorSessionsRoot: root})
}

func newTestServerWithProviderSessionRoots(codexRoot, cursorRoot string) *Server {
	logger, _ := zap.NewDevelopment()
	return NewServerWithOptions(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*factorytoken.Token),
		},
	}, 8080, logger, ServerOptions{
		CodexSessionsRoot:  codexRoot,
		CursorSessionsRoot: cursorRoot,
	})
}

func writeProviderSessionFixture(t *testing.T, root, id, contents string) {
	t.Helper()

	dir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create provider session fixture directory: %v", err)
	}
	path := filepath.Join(dir, "rollout-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write provider session fixture: %v", err)
	}
}

func writeNamedProviderSessionFixture(t *testing.T, root, fileName, contents string) {
	t.Helper()

	dir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create provider session fixture directory: %v", err)
	}
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write named provider session fixture: %v", err)
	}
}

// customerCursorProviderSessionID mirrors the UUID-shaped session_id from the
// reported Windows provider-session detail failure mode.
const customerCursorProviderSessionID = "ed332681-38eb-485f-b3d3-d8b6df3a450b"

// customerCursorWorkspaceHash mirrors the workspace-hash directory layout under ~/.cursor/chats.
const customerCursorWorkspaceHash = "d2191e81bfe68d31807c1e354ea83571"

func writeCursorProviderSessionFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()

	root = t.TempDir()
	sessionID = "cursor-api-readable"
	return writeCursorProviderSessionFixtureAt(t, root, "workspace-hash", sessionID)
}

func writeCursorProviderSessionUUIDFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()

	root = t.TempDir()
	sessionID = customerCursorProviderSessionID
	return writeCursorProviderSessionFixtureAt(t, root, customerCursorWorkspaceHash, sessionID)
}

func writeCursorProviderSessionFixtureAt(t *testing.T, root, workspaceHash, sessionID string) (string, string) {
	t.Helper()

	dbPath := filepath.Join(root, workspaceHash, sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir cursor provider fixture: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open cursor provider fixture sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	schema := `
CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create cursor provider fixture tables: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO blobs (key, value) VALUES (?, ?)`,
		"bubble1",
		`{"bubbleId":"bubble1","chatId":"chat1","text":"Hello from API fixture","timestamp":1000,"type":1}`,
	); err != nil {
		t.Fatalf("insert cursor provider bubble: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"0",
		`{"createdAt":1000,"agentId":"`+sessionID+`","name":"API fixture session"}`,
	); err != nil {
		t.Fatalf("insert cursor provider session meta: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"1",
		`{"usage":{"inputTokens":100,"outputTokens":25,"cacheReadTokens":40,"cacheWriteTokens":10}}`,
	); err != nil {
		t.Fatalf("insert cursor provider usage meta: %v", err)
	}

	normalizedRoot, err := cursorstorage.NormalizeAgentStorageRoot(root)
	if err != nil {
		t.Fatalf("normalize cursor provider fixture root: %v", err)
	}
	if resolved, err := cursorstorage.ResolveStoreDB(normalizedRoot, sessionID); err != nil {
		t.Fatalf("resolve cursor provider fixture: %v", err)
	} else if resolved.RelativePath != filepath.ToSlash(filepath.Join(workspaceHash, sessionID, "store.db")) {
		t.Fatalf("resolved relative path = %q, want %s/%s/store.db", resolved.RelativePath, workspaceHash, sessionID)
	}

	return root, sessionID
}

// providerSessionDetailURLFromEventRef builds the GET path the dashboard uses to
// load a provider session from an event-emitted LoadableProviderSessionRef.
func providerSessionDetailURLFromEventRef(ref factoryapi.LoadableProviderSessionRef) string {
	query := url.Values{}
	query.Set("provider", string(ref.Provider))
	query.Set("kind", string(ref.Kind))
	query.Set("id", ref.Id)
	return "/provider-sessions/detail?" + query.Encode()
}

// loadableProviderSessionRefFromEventMetadata mirrors dispatch/event projection of
// canonical provider-session metadata onto the loadable detail contract.
func loadableProviderSessionRefFromEventMetadata(session workerexecution.ProviderSessionMetadata) factoryapi.LoadableProviderSessionRef {
	return factoryapi.LoadableProviderSessionRef{
		Provider: factoryapi.LoadableProviderSessionProvider(workerexecution.CanonicalProviderSessionProvider(session.Provider)),
		Kind:     factoryapi.LoadableProviderSessionKind(session.Kind),
		Id:       session.ID,
	}
}

func assertProviderSessionDetailLoadsFromEventRef(
	t *testing.T,
	srv *Server,
	ref factoryapi.LoadableProviderSessionRef,
	wantProvider factoryapi.LoadableProviderSessionProvider,
) factoryapi.ProviderSessionDetailResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, providerSessionDetailURLFromEventRef(ref), nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for event ref %#v: %s", rec.Code, ref, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != string(wantProvider) ||
		string(resp.ProviderSession.Kind) != string(ref.Kind) ||
		resp.ProviderSession.Id != ref.Id {
		t.Fatalf("provider session = %#v, want provider=%s kind=%s id=%s", resp.ProviderSession, wantProvider, ref.Kind, ref.Id)
	}
	if resp.Source.RelativePath == "" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted source metadata for event ref %#v", resp.Source, ref)
	}
	return resp
}

func readSSEFactoryEvent(t *testing.T, reader *bufio.Reader) factoryapi.FactoryEvent {
	t.Helper()

	var dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE line: %v", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "event:") {
			t.Fatalf("factory event stream should use default SSE message event, got line %q", line)
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}

	if dataLine == "" {
		t.Fatal("expected SSE data payload")
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		t.Fatalf("decode SSE factory event: %v", err)
	}
	return event
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string, wantMessage string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var resp factoryapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if string(resp.Code) != wantCode {
		t.Fatalf("error code = %q, want %q", resp.Code, wantCode)
	}
	if resp.Message != wantMessage {
		t.Fatalf("error message = %q, want %q", resp.Message, wantMessage)
	}
}

func makeListWorkTokens(prefix string, count int, now time.Time) map[string]*factorytoken.Token {
	tokens := make(map[string]*factorytoken.Token, count)
	for i := 1; i <= count; i++ {
		suffix := string(rune('0' + i))
		id := "tok-" + prefix + "-" + suffix
		tokens[id] = listWorkToken(id, "work-"+prefix+"-"+suffix, prefix+":init", prefix, now)
	}
	return tokens
}

func listWorkToken(id, workID, placeID, workTypeID string, now time.Time) *factorytoken.Token {
	return listWorkTokenWithTraces(id, workID, "", placeID, workTypeID, "", "", now)
}

func listWorkTokenWithTraces(id, workID, name, placeID, workTypeID, traceID, currentChainingTraceID string, now time.Time) *factorytoken.Token {
	color := factorytoken.Color{
		WorkID:                 workID,
		WorkTypeID:             workTypeID,
		TraceID:                traceID,
		CurrentChainingTraceID: currentChainingTraceID,
	}
	if name != "" {
		color.Name = name
	}
	return listWorkTokenWithColor(id, workID, placeID, workTypeID, now, color)
}

func listWorkTokenWithColor(id, workID, placeID, workTypeID string, now time.Time, color factorytoken.Color) *factorytoken.Token {
	if color.WorkID == "" {
		color.WorkID = workID
	}
	if color.WorkTypeID == "" {
		color.WorkTypeID = workTypeID
	}
	return &factorytoken.Token{
		ID:        id,
		PlaceID:   placeID,
		Color:     color,
		CreatedAt: now,
		EnteredAt: now,
		History: factorytoken.History{
			TotalVisits:         make(map[string]int),
			ConsecutiveFailures: make(map[string]int),
			PlaceVisits:         make(map[string]int),
		},
	}
}

func listWorkFilterTopology() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":     {ID: "task:init", TypeID: "task", State: "init"},
			"task:review":   {ID: "task:review", TypeID: "task", State: "review"},
			"task:failed":   {ID: "task:failed", TypeID: "task", State: "failed"},
			"task:complete": {ID: "task:complete", TypeID: "task", State: "complete"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}

func assertGeneratedWorkContentParts(t *testing.T, content *factoryapi.WorkContent, want []work.WorkContentPart) {
	t.Helper()
	if content == nil {
		t.Fatalf("content = nil, want %#v", want)
	}
	if len(*content) != len(want) {
		t.Fatalf("content count = %d, want %d", len(*content), len(want))
	}
	for i, wantPart := range want {
		assertGeneratedWorkContentPart(t, (*content)[i], i, wantPart)
	}
}

func extractInvocationRequestText(t *testing.T, request *factoryapi.InvocationRequest) string {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	if request.Content == nil {
		t.Fatal("content = nil, want one text part")
	}
	if len(*request.Content) != 1 {
		t.Fatalf("content parts = %d, want 1", len(*request.Content))
	}
	part, err := (*request.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("AsWorkTextContentPart: %v", err)
	}
	return part.Text
}

func assertFactorySessionInvocation(
	t *testing.T,
	mock *testutil.MockFactory,
	body string,
	wantResult apisurface.FactoryInvocationResult,
	wantSubmitText string,
) {
	t.Helper()

	srv := newTestServer(mock)
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/invocations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.InvocationResponse](t, rec)
	if response.RequestId != wantResult.RequestID || response.TraceId != wantResult.TraceID || response.Status != factoryapi.InvocationTerminalStatus(wantResult.Status) {
		t.Fatalf("invocation response = %#v, want completed invocation identifiers", response)
	}
	assertGeneratedWorkContentParts(t, response.PrimaryResult, wantResult.PrimaryResult)
	if wantSubmitText != "" {
		if len(mock.InvokedFactorySessions) != 1 {
			t.Fatalf("invoked factory sessions = %d, want 1", len(mock.InvokedFactorySessions))
		}
		if got := extractInvocationRequestText(t, &mock.InvokedFactorySessions[0]); got != wantSubmitText {
			t.Fatalf("invocation text = %q, want %q", got, wantSubmitText)
		}
	}
}

func assertGeneratedWorkContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	switch want.Type {
	case work.WorkContentPartTypeText:
		assertGeneratedTextContentPart(t, got, index, want)
	case work.WorkContentPartTypeImage:
		assertGeneratedImageContentPart(t, got, index, want)
	case work.WorkContentPartTypeAudio:
		assertGeneratedAudioContentPart(t, got, index, want)
	case work.WorkContentPartTypeJSON:
		assertGeneratedJSONContentPart(t, got, index, want)
	case work.WorkContentPartTypeBinary:
		assertGeneratedBinaryContentPart(t, got, index, want)
	default:
		t.Fatalf("unsupported expected content type %q", want.Type)
	}
}

func assertGeneratedTextContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode text: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeText || part.Text != want.Text {
		t.Fatalf("content[%d] = %#v, want text %q", index, part, want.Text)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedImageContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkImageContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode image: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeImage || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want image url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedAudioContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode audio: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeAudio || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want audio url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedJSONContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode json: %v", index, err)
	}
	rawJSON, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("content[%d] encode json: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeJSON || string(rawJSON) != string(want.JSON) {
		t.Fatalf("content[%d] = %#v, want json %s", index, part, want.JSON)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedBinaryContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkBinaryContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode binary: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeBinary || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want binary url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedPartSharedFields(t *testing.T, index int, slot *string, label *string, role *string, contentType *string, artifactID *string, metadata *factoryapi.WorkContentMetadata, want work.WorkContentPart) {
	t.Helper()

	if derefString(slot) != want.Slot ||
		derefString(label) != want.Label ||
		derefString(role) != want.Role ||
		derefString(contentType) != want.ContentType ||
		derefString(artifactID) != want.ArtifactID {
		t.Fatalf("content[%d] shared fields mismatch for %#v", index, want)
	}
	gotMetadata, _ := json.Marshal(metadata)
	wantMetadata, _ := json.Marshal(want.Metadata)
	if string(gotMetadata) != string(wantMetadata) {
		t.Fatalf("content[%d] metadata = %s, want %s", index, gotMetadata, wantMetadata)
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func deprecatedGeneratedFile(file *factoryapi.WorkContentDeprecatedFileProperty) string {
	if file == nil {
		return ""
	}
	return string(*file)
}

func testFactoryEvent(t *testing.T, eventType factoryapi.FactoryEventType, id string, context factoryapi.FactoryEventContext, payload any) factoryapi.FactoryEvent {
	t.Helper()
	var eventPayload factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		err = eventPayload.FromRunRequestEventPayload(typed)
	case factoryapi.InitialStructureRequestEventPayload:
		err = eventPayload.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = eventPayload.FromWorkRequestEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = eventPayload.FromDispatchRequestEventPayload(typed)
	default:
		t.Fatalf("unsupported test factory event payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode test factory event payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
		Id:            id,
		Context:       context,
		Payload:       eventPayload,
	}
}

func stringPointerForAPITest(value string) *string {
	return &value
}

func validNamedFactoryBody(name, workType string) string {
	return fmt.Sprintf(`{"name":%q,%s`, name, strings.TrimPrefix(namedFactoryPayloadJSON(name, workType), "{"))
}

func saveFactoryForSessionRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"factory":%s}`, factoryJSON)
}

func namedFactoryPayloadJSON(project, workType string) string {
	return fmt.Sprintf(`{
		"name": %q,
		"id": %q,
		"workTypes": [{
			"name": %q,
			"states": [
				{"name":"init","type":"INITIAL"},
				{"name":"done","type":"TERMINAL"},
				{"name":"failed","type":"FAILED"}
			]
		}],
		"workers": [{
			"name":"planner",
			"type":"MODEL_WORKER",
			"modelProvider":"CLAUDE",
			"executorProvider":"SCRIPT_WRAP",
			"model":"claude-sonnet-4-20250514"
		}],
		"workstations": [{
			"name":"plan-task",
			"behavior":"STANDARD",
			"type":"MODEL_WORKSTATION",
			"worker":"planner",
			"inputs":[{"workType":%q,"state":"init"}],
			"outputs":[{"workType":%q,"state":"done"}]
		}]
	}`, project, project, workType, workType, workType)
}

func submittedRequestNamed(t *testing.T, requests []work.SubmitRequest, name string) work.SubmitRequest {
	t.Helper()
	for _, request := range requests {
		if request.Name == name {
			return request
		}
	}
	t.Fatalf("submit request %q not found in %#v", name, requests)
	return work.SubmitRequest{}
}

func listedWorkByID(t *testing.T, works []factoryapi.Work, workID string) factoryapi.Work {
	t.Helper()
	for _, work := range works {
		if stringValue(work.WorkId) == workID {
			return work
		}
	}
	t.Fatalf("listed work %q not found in %#v", workID, works)
	return factoryapi.Work{}
}

func assertSubmittedChildRelations(t *testing.T, relations []work.Relation) {
	t.Helper()

	var foundParentChild bool
	var foundDependsOn bool
	for _, relation := range relations {
		switch relation.Type {
		case work.RelationParentChild:
			foundParentChild = true
			if relation.TargetWorkID != "batch-request-api-parent-child-parent" {
				t.Fatalf("parent-child target = %q, want batch-request-api-parent-child-parent", relation.TargetWorkID)
			}
		case work.RelationDependsOn:
			foundDependsOn = true
			if relation.TargetWorkID != "batch-request-api-parent-child-prerequisite" {
				t.Fatalf("depends_on target = %q, want batch-request-api-parent-child-prerequisite", relation.TargetWorkID)
			}
			if relation.RequiredState != "complete" {
				t.Fatalf("depends_on required_state = %q, want complete", relation.RequiredState)
			}
		default:
			t.Fatalf("unexpected normalized relation = %#v", relation)
		}
	}
	if !foundParentChild {
		t.Fatal("missing normalized parent-child relation")
	}
	if !foundDependsOn {
		t.Fatal("missing normalized depends_on relation")
	}
}

func submitWorkRequest(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, scopeDefaultSessionWorkPath("/work"), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func submitWorkStageFileRequest(
	t *testing.T,
	srv *Server,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, scopeDefaultSessionWorkPath(path), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func upsertWorkRequest(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, scopeDefaultSessionWorkPath(path), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeJSONResponse[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	return out
}

func encodeNextToken(token string) string {
	return base64.StdEncoding.EncodeToString([]byte(token))
}

func decodeListWorkPage(t *testing.T, srv *Server, path string) factoryapi.ListWorkResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, scopeDefaultSessionWorkPath(path), nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200: %s", path, rec.Code, rec.Body.String())
	}
	return decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
}

func assertListedWorkIDs(t *testing.T, works []factoryapi.Work, want []string) {
	t.Helper()
	if len(works) != len(want) {
		t.Fatalf("results = %d, want %d: %#v", len(works), len(want), works)
	}
	for i, wantWorkID := range want {
		if got := stringValue(works[i].WorkId); got != wantWorkID {
			t.Fatalf("result[%d].workId = %q, want %q: %#v", i, got, wantWorkID, works)
		}
	}
}

type upsertValidationFailureCase struct {
	name    string
	path    string
	body    string
	factory *testutil.MockFactory
	wantMsg string
}

func runUpsertValidationFailureCases(t *testing.T, cases []upsertValidationFailureCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mf := tc.factory
			if mf == nil {
				mf = &testutil.MockFactory{}
			}
			mf.Marking = &petri.MarkingSnapshot{Tokens: make(map[string]*factorytoken.Token)}
			srv := newTestServer(mf)

			rec := upsertWorkRequest(t, srv, tc.path, tc.body)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", tc.wantMsg)
			if len(mf.Submitted) != 0 {
				t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
			}
		})
	}
}

type strictJSONTestPayload struct {
	Name string `json:"name"`
}

func TestDecodeStrictJSON_ValidObject(t *testing.T) {
	got, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha"}`))
	if err != nil {
		t.Fatalf("decodeStrictJSON() error = %v, want nil", err)
	}
	if got.Name != "alpha" {
		t.Fatalf("decoded name = %q, want alpha", got.Name)
	}
}

func TestDecodeStrictJSON_UnknownField(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha","extra":1}`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), `json: unknown field "extra"`) {
		t.Fatalf("decodeStrictJSON() error = %v, want unknown field message", err)
	}
}

func TestDecodeStrictJSON_MalformedJSON(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want malformed JSON error")
	}
}

func TestDecodeStrictJSON_EmptyBody(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(""))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("decodeStrictJSON() error = %v, want io.EOF", err)
	}
}

func TestDecodeStrictJSON_MultiObjectPayload(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha"}{}`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want single-object validation error")
	}
	message, ok := requestFieldValidationMessage(err)
	if !ok {
		t.Fatalf("decodeStrictJSON() error = %T(%v), want requestFieldValidationError", err, err)
	}
	if message != "request payload must contain one JSON object" {
		t.Fatalf("validation message = %q, want single-object payload message", message)
	}
}

func TestGetProviderSessionDetails_FailsForAmbiguousTimestampPrefixedMatches(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)
	sessionDir := filepath.Join(root, "2026", "05", "19")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create provider session fixture directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "rollout-2026-05-20T17-45-24-sess_123.jsonl"), []byte(`{"type":"session_meta","id":"sess_123"}`), 0o600); err != nil {
		t.Fatalf("write second timestamp-prefixed provider session fixture: %v", err)
	}

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR", "multiple provider session files match session identifier")
}

func TestParseCodexSessionDetails_ReconcilesMirroredCodexMessages(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-06-04T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-06-04T10:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"I will inspect the factory state first.","phase":"commentary"}}`,
		`{"timestamp":"2026-06-04T10:00:01Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I will inspect the factory state first."}],"phase":"commentary"}}`,
		`{"timestamp":"2026-06-04T10:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Plan the next phase."}]}}`,
		`{"timestamp":"2026-06-04T10:00:02Z","type":"event_msg","payload":{"type":"user_message","message":"Plan the next phase."}}`,
		`{"timestamp":"2026-06-04T10:00:03Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}}}`,
		`{"timestamp":"2026-06-04T10:00:04Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Factory state looks ready."}]}}`,
	}, "\n")

	parsed, err := parseCodexSessionDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session details: %v", err)
	}

	assertMirroredCodexMessageSummary(t, parsed.Summary)
	assertMirroredCodexMessageTranscript(t, parsed)
}

func assertMirroredCodexMessageSummary(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.LineCount != 7 || summary.EventCount != 7 {
		t.Fatalf("summary = %#v, want all source records counted", summary)
	}
	if summary.TokenUsage == nil || intValue(summary.TokenUsage.TotalTokens) != 120 {
		t.Fatalf("token usage = %#v, want line-level token accounting retained", summary.TokenUsage)
	}
}

func assertMirroredCodexMessageTranscript(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if len(parsed.Transcript) != 3 {
		t.Fatalf("transcript = %#v, want mirrored messages emitted once", parsed.Transcript)
	}
	assertMirroredCodexMessageTranscriptEntry(t, parsed, 0, factoryapi.AssistantMessage, 2, "I will inspect the factory state first.", "first mirrored assistant message retained")
	assertMirroredCodexMessageTranscriptEntry(t, parsed, 1, factoryapi.UserMessage, 4, "Plan the next phase.", "first mirrored user message retained")
	assertMirroredCodexMessageTranscriptEntry(t, parsed, 2, factoryapi.AssistantMessage, 7, "Factory state looks ready.", "following distinct assistant message retained")
}

func assertMirroredCodexMessageTranscriptEntry(t *testing.T, parsed parsedCodexSessionDetails, index int, wantType factoryapi.ProviderSessionTranscriptEntryType, wantLine int, wantText string, wantDescription string) {
	t.Helper()

	entry := parsed.Transcript[index]
	if entry.Order != index+1 || entry.Type != wantType || intValue(entry.LineNumber) != wantLine || stringValue(entry.Text) != wantText {
		t.Fatalf("transcript[%d] = %#v, want %s", index, entry, wantDescription)
	}
}

func TestGetProviderSessionDetails_LoadsTimestampPrefixedCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl", strings.Join([]string{
		`{"type":"session_meta","id":"019e44f4-580e-7f32-981e-1e54ec6907d6"}`,
		`{"type":"response_item","payload":{"type":"reasoning"}}`,
	}, "\n"))

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=019e44f4-580e-7f32-981e-1e54ec6907d6", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if resp.Source.RelativePath != "2026/05/18/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl" || resp.ProviderSession.Id != "019e44f4-580e-7f32-981e-1e54ec6907d6" || resp.Parse.EventCount != 2 {
		t.Fatalf("provider session detail = %#v, want timestamp-prefixed session path", resp)
	}
}

func TestGetProviderSessionDetails_PrefersExactCodexSessionFileWhenSupportedLayoutsBothExist(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", `{"type":"session_meta","id":"sess_123"}`)
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" {
		t.Fatalf("relative path = %q, want exact rollout basename", resp.Source.RelativePath)
	}
}
