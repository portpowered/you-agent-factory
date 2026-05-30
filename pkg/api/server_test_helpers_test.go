package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

func newTestServer(f *testutil.MockFactory) *Server {
	logger, _ := zap.NewDevelopment()
	return NewServer(f, 8080, logger)
}

func newTestServerWithCodexRoot(root string) *Server {
	logger, _ := zap.NewDevelopment()
	return NewServerWithOptions(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}, 8080, logger, ServerOptions{CodexSessionsRoot: root})
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

func makeListWorkTokens(prefix string, count int, now time.Time) map[string]*interfaces.Token {
	tokens := make(map[string]*interfaces.Token, count)
	for i := 1; i <= count; i++ {
		suffix := string(rune('0' + i))
		id := "tok-" + prefix + "-" + suffix
		tokens[id] = listWorkToken(id, "work-"+prefix+"-"+suffix, prefix+":init", prefix, now)
	}
	return tokens
}

func listWorkToken(id, workID, placeID, workTypeID string, now time.Time) *interfaces.Token {
	return listWorkTokenWithTraces(id, workID, "", placeID, workTypeID, "", "", now)
}

func listWorkTokenWithTraces(id, workID, name, placeID, workTypeID, traceID, currentChainingTraceID string, now time.Time) *interfaces.Token {
	color := interfaces.TokenColor{
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

func listWorkTokenWithColor(id, workID, placeID, workTypeID string, now time.Time, color interfaces.TokenColor) *interfaces.Token {
	if color.WorkID == "" {
		color.WorkID = workID
	}
	if color.WorkTypeID == "" {
		color.WorkTypeID = workTypeID
	}
	return &interfaces.Token{
		ID:        id,
		PlaceID:   placeID,
		Color:     color,
		CreatedAt: now,
		EnteredAt: now,
		History: interfaces.TokenHistory{
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

func assertGeneratedWorkContentParts(t *testing.T, content *factoryapi.WorkContent, want []interfaces.WorkContentPart) {
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

func assertGeneratedWorkContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want interfaces.WorkContentPart) {
	t.Helper()
	switch want.Type {
	case interfaces.WorkContentPartTypeText:
		assertGeneratedTextContentPart(t, got, index, want)
	case interfaces.WorkContentPartTypeImage:
		assertGeneratedImageContentPart(t, got, index, want)
	case interfaces.WorkContentPartTypeAudio:
		assertGeneratedAudioContentPart(t, got, index, want)
	case interfaces.WorkContentPartTypeJSON:
		assertGeneratedJSONContentPart(t, got, index, want)
	case interfaces.WorkContentPartTypeBinary:
		assertGeneratedBinaryContentPart(t, got, index, want)
	default:
		t.Fatalf("unsupported expected content type %q", want.Type)
	}
}

func assertGeneratedTextContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want interfaces.WorkContentPart) {
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

func assertGeneratedImageContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want interfaces.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkImageContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode image: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeImage || part.File != want.File {
		t.Fatalf("content[%d] = %#v, want image %q", index, part, want.File)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedAudioContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want interfaces.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode audio: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeAudio || part.File != want.File {
		t.Fatalf("content[%d] = %#v, want audio %q", index, part, want.File)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedJSONContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want interfaces.WorkContentPart) {
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

func assertGeneratedBinaryContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want interfaces.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkBinaryContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode binary: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeBinary || part.File != want.File {
		t.Fatalf("content[%d] = %#v, want binary %q", index, part, want.File)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedPartSharedFields(t *testing.T, index int, slot *string, label *string, role *string, contentType *string, artifactID *string, metadata *factoryapi.WorkContentMetadata, want interfaces.WorkContentPart) {
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

func submittedRequestNamed(t *testing.T, requests []interfaces.SubmitRequest, name string) interfaces.SubmitRequest {
	t.Helper()
	for _, request := range requests {
		if request.Name == name {
			return request
		}
	}
	t.Fatalf("submit request %q not found in %#v", name, requests)
	return interfaces.SubmitRequest{}
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

func assertSubmittedChildRelations(t *testing.T, relations []interfaces.Relation) {
	t.Helper()

	var foundParentChild bool
	var foundDependsOn bool
	for _, relation := range relations {
		switch relation.Type {
		case interfaces.RelationParentChild:
			foundParentChild = true
			if relation.TargetWorkID != "batch-request-api-parent-child-parent" {
				t.Fatalf("parent-child target = %q, want batch-request-api-parent-child-parent", relation.TargetWorkID)
			}
		case interfaces.RelationDependsOn:
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

	req := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(body))
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

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func upsertWorkRequest(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
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
func TestGetProviderSessionDetails_LoadsCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
		`{"unexpected":true}`,
		`not-json`,
		``,
	}, "\n"))

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	assertProviderSessionResponseIdentity(t, resp)
	assertProviderSessionParseCounts(t, resp.Parse)
	assertProviderSessionTranscriptSummary(t, resp)
	assertProviderSessionParseDiagnostics(t, resp.Parse)
}

func assertProviderSessionResponseIdentity(t *testing.T, resp factoryapi.ProviderSessionDetailResponse) {
	t.Helper()

	if string(resp.ProviderSession.Provider) != "codex" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != "sess_123" {
		t.Fatalf("provider session = %#v, want codex session_id sess_123", resp.ProviderSession)
	}
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted rollout path with metadata", resp.Source)
	}
}

func assertProviderSessionParseCounts(t *testing.T, parse factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if parse.LineCount != 4 || parse.EventCount != 3 || parse.MalformedLineCount != 1 || parse.UnknownEventCount != 1 {
		t.Fatalf("parse summary = %#v, want line/event/malformed/unknown counts", parse)
	}
}

func assertProviderSessionTranscriptSummary(t *testing.T, resp factoryapi.ProviderSessionDetailResponse) {
	t.Helper()

	if len(resp.Transcript) != 1 || resp.Transcript[0].Type != factoryapi.Reasoning || resp.Transcript[0].Order != 1 {
		t.Fatalf("transcript = %#v, want one reasoning transcript entry", resp.Transcript)
	}
	if len(resp.Parse.Turns) != 1 || resp.Parse.Turns[0].ReasoningCount != 1 || len(resp.Parse.Reasoning) != 1 || resp.Parse.Reasoning[0].SourceType != "reasoning" {
		t.Fatalf("parse detail = %#v, want reasoning turn summary", resp.Parse)
	}
}

func assertProviderSessionParseDiagnostics(t *testing.T, parse factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(parse.ParseErrors) != 1 || parse.ParseErrors[0].LineNumber != 4 || len(parse.UnknownEvents) != 1 || parse.UnknownEvents[0].LineNumber != 3 {
		t.Fatalf("parse diagnostics = %#v, want malformed line 4 and unknown line 3", parse)
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

func TestParseCodexSessionSummary_ExtractsDiagnosticDetails(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"reasoning","summary":["checked input"],"encrypted_content":"sealed"}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":{"cmd":"go test ./pkg/api"}}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":130}}}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call-2","name":"apply_patch","input":"patch text","status":"in_progress"}}`,
		`{"timestamp":"2026-05-18T10:00:07Z","type":"event_msg","payload":{"type":"new_future_event"}}`,
		`{"timestamp":"2026-05-18T10:00:08Z","type":"unexpected_top_level"}`,
		`{bad json`,
	}, "\n")

	summary, err := parseCodexSessionSummary(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session summary: %v", err)
	}
	assertCodexSessionSummaryCoreCounts(t, summary)
	assertCodexSessionSummaryFunctionCalls(t, summary)
	assertCodexSessionSummaryReasoning(t, summary)
	parsed, err := parseCodexSessionDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session details: %v", err)
	}
	assertParsedCodexSessionSummaryTranscript(t, parsed)
	assertCodexSessionSummaryTokenUsage(t, summary)
	assertCodexSessionSummaryUnknowns(t, summary)
}

func TestParseCodexSessionDetails_EmitsMixedTranscriptChronologically(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect the failing run."}]}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":["Checking tool output"],"encrypted_content":"sealed"}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":{"cmd":"go test ./pkg/api"}}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok","status":"completed"}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"The package tests passed."}}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"event_msg","payload":{"type":"task_started","message":"Applying follow-up patch"}}`,
		`{"timestamp":"2026-05-18T10:00:07Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"Need one more validation step."}}`,
		`{"timestamp":"2026-05-18T10:00:08Z","type":"event_msg","payload":{"type":"new_future_event"}}`,
		`{"timestamp":"2026-05-18T10:00:09Z","type":"unexpected_top_level"}`,
		`{bad json`,
	}, "\n")

	parsed, err := parseCodexSessionDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session details: %v", err)
	}

	assertMixedCodexSessionSummary(t, parsed.Summary)
	assertMixedCodexSessionTranscript(t, parsed)
}

func assertCodexSessionSummaryCoreCounts(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.LineCount != 10 || summary.EventCount != 9 || summary.MalformedLineCount != 1 || summary.UnknownEventCount != 2 || len(summary.Turns) != 2 || len(summary.FunctionCalls) != 2 {
		t.Fatalf("summary = %#v, want parsed counts and two turns/calls", summary)
	}
}

func assertCodexSessionSummaryFunctionCalls(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	firstCall := summary.FunctionCalls[0]
	if firstCall.Order != 1 || stringValue(firstCall.Name) != "exec_command" || stringValue(firstCall.Arguments) != `{"cmd":"go test ./pkg/api"}` || stringValue(firstCall.Output) != "ok" || stringValue(firstCall.Status) != "completed" {
		t.Fatalf("first function call = %#v, want completed exec_command call", firstCall)
	}
	secondCall := summary.FunctionCalls[1]
	if secondCall.Order != 2 || stringValue(secondCall.Name) != "apply_patch" || stringValue(secondCall.Status) != "in_progress" || stringValue(secondCall.Output) != "" {
		t.Fatalf("second function call = %#v, want in-progress custom tool call", secondCall)
	}
}

func assertCodexSessionSummaryReasoning(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(summary.Reasoning) != 1 || stringValue(summary.Reasoning[0].Summary) != `["checked input"]` || summary.Reasoning[0].Encrypted == nil || !*summary.Reasoning[0].Encrypted || stringValue(summary.Reasoning[0].EncryptedContent) != "sealed" {
		t.Fatalf("reasoning = %#v, want summary, encrypted marker, and encrypted content", summary.Reasoning)
	}
}

func assertParsedCodexSessionSummaryTranscript(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if len(parsed.Transcript) != 4 {
		t.Fatalf("transcript = %#v, want four ordered transcript entries", parsed.Transcript)
	}
	if parsed.Transcript[0].Type != factoryapi.Reasoning || stringValue(parsed.Transcript[0].SourceType) != "reasoning" || intValue(parsed.Transcript[0].LineNumber) != 2 {
		t.Fatalf("first transcript entry = %#v, want reasoning line 2", parsed.Transcript[0])
	}
	if parsed.Transcript[1].Type != factoryapi.ToolCall || stringValue(parsed.Transcript[1].Name) != "exec_command" || stringValue(parsed.Transcript[1].Arguments) != `{"cmd":"go test ./pkg/api"}` {
		t.Fatalf("second transcript entry = %#v, want exec_command tool call", parsed.Transcript[1])
	}
	if parsed.Transcript[2].Type != factoryapi.ToolOutput || stringValue(parsed.Transcript[2].Output) != "ok" || stringValue(parsed.Transcript[2].Status) != "completed" {
		t.Fatalf("third transcript entry = %#v, want completed tool output", parsed.Transcript[2])
	}
	if parsed.Transcript[3].Type != factoryapi.ToolCall || stringValue(parsed.Transcript[3].Name) != "apply_patch" || stringValue(parsed.Transcript[3].Status) != "in_progress" {
		t.Fatalf("fourth transcript entry = %#v, want in-progress apply_patch tool call", parsed.Transcript[3])
	}
	if parsed.Transcript[3].Order != 4 {
		t.Fatalf("final transcript entry order = %d, want 4", parsed.Transcript[3].Order)
	}
}

func assertCodexSessionSummaryTokenUsage(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.TokenUsage == nil || intValue(summary.TokenUsage.InputTokens) != 100 || intValue(summary.TokenUsage.CachedInputTokens) != 40 || intValue(summary.TokenUsage.OutputTokens) != 25 || intValue(summary.TokenUsage.ReasoningOutputTokens) != 5 || intValue(summary.TokenUsage.TotalTokens) != 130 {
		t.Fatalf("token usage = %#v, want total consumed token fields", summary.TokenUsage)
	}
}

func assertCodexSessionSummaryUnknowns(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(summary.UnknownEvents) != 2 || summary.UnknownEvents[0].LineNumber != 8 || stringValue(summary.UnknownEvents[0].Type) != "event_msg" || stringValue(summary.UnknownEvents[0].PayloadType) != "new_future_event" || summary.UnknownEvents[1].LineNumber != 9 || stringValue(summary.UnknownEvents[1].Type) != "unexpected_top_level" {
		t.Fatalf("unknown events = %#v, want compact line-level unknown records", summary.UnknownEvents)
	}
	if len(summary.ParseErrors) != 1 || summary.ParseErrors[0].LineNumber != 10 {
		t.Fatalf("parse errors = %#v, want malformed line retained", summary.ParseErrors)
	}
}

func assertMixedCodexSessionSummary(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.LineCount != 11 || summary.EventCount != 10 || summary.MalformedLineCount != 1 || summary.UnknownEventCount != 2 {
		t.Fatalf("summary = %#v, want mixed-session diagnostic counts", summary)
	}
	if len(summary.Turns) != 1 || summary.Turns[0].FunctionCallCount != 1 || summary.Turns[0].ReasoningCount != 2 {
		t.Fatalf("turn summary = %#v, want one turn with function and reasoning counts", summary.Turns)
	}
	if len(summary.UnknownEvents) != 2 || summary.UnknownEvents[0].LineNumber != 9 || summary.UnknownEvents[1].LineNumber != 10 {
		t.Fatalf("unknown events = %#v, want unknown event_msg and top-level event retained", summary.UnknownEvents)
	}
	if len(summary.ParseErrors) != 1 || summary.ParseErrors[0].LineNumber != 11 {
		t.Fatalf("parse errors = %#v, want malformed line 11 retained", summary.ParseErrors)
	}
}

func assertMixedCodexSessionTranscriptLength(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if len(parsed.Transcript) != 7 {
		t.Fatalf("transcript = %#v, want seven ordered transcript entries", parsed.Transcript)
	}
}

func assertMixedCodexSessionTranscriptEntry(t *testing.T, parsed parsedCodexSessionDetails, index int, wantType factoryapi.ProviderSessionTranscriptEntryType, wantLine int, wantText string) {
	t.Helper()

	entry := parsed.Transcript[index]
	if entry.Order != index+1 || entry.Type != wantType || intValue(entry.LineNumber) != wantLine || stringValue(entry.Text) != wantText {
		t.Fatalf("transcript[%d] = %#v, want order=%d type=%q line=%d text=%q", index, entry, index+1, wantType, wantLine, wantText)
	}
	if intValue(entry.TurnIndex) != 1 {
		t.Fatalf("transcript[%d] turn index = %#v, want 1", index, entry.TurnIndex)
	}
}

func assertMixedCodexSessionTranscript(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptLength(t, parsed)
	assertMixedCodexSessionTranscriptUserMessage(t, parsed)
	assertMixedCodexSessionTranscriptReasoning(t, parsed)
	assertMixedCodexSessionTranscriptToolEvents(t, parsed)
	assertMixedCodexSessionTranscriptAssistantAndSystemEvents(t, parsed)
}

func assertMixedCodexSessionTranscriptUserMessage(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptEntry(t, parsed, 0, factoryapi.UserMessage, 2, "Inspect the failing run.")
	if parsed.Transcript[0].SourceType == nil || *parsed.Transcript[0].SourceType != "message" {
		t.Fatalf("first transcript source type = %#v, want message", parsed.Transcript[0].SourceType)
	}
}

func assertMixedCodexSessionTranscriptReasoning(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if parsed.Transcript[1].Order != 2 || parsed.Transcript[1].Type != factoryapi.Reasoning || intValue(parsed.Transcript[1].LineNumber) != 3 || stringValue(parsed.Transcript[1].Summary) != `["Checking tool output"]` || parsed.Transcript[1].Encrypted == nil || !*parsed.Transcript[1].Encrypted || stringValue(parsed.Transcript[1].EncryptedContent) != "sealed" {
		t.Fatalf("transcript[1] = %#v, want encrypted reasoning summary and content on line 3", parsed.Transcript[1])
	}

	assertMixedCodexSessionTranscriptEntry(t, parsed, 6, factoryapi.Reasoning, 8, "Need one more validation step.")
	if parsed.Transcript[6].SourceType == nil || *parsed.Transcript[6].SourceType != "agent_reasoning" {
		t.Fatalf("final reasoning transcript source type = %#v, want agent_reasoning", parsed.Transcript[6].SourceType)
	}
}

func assertMixedCodexSessionTranscriptToolEvents(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if parsed.Transcript[2].Order != 3 || parsed.Transcript[2].Type != factoryapi.ToolCall || intValue(parsed.Transcript[2].LineNumber) != 4 || stringValue(parsed.Transcript[2].Name) != "exec_command" {
		t.Fatalf("transcript[2] = %#v, want tool call on line 4", parsed.Transcript[2])
	}
	if parsed.Transcript[3].Order != 4 || parsed.Transcript[3].Type != factoryapi.ToolOutput || intValue(parsed.Transcript[3].LineNumber) != 5 || stringValue(parsed.Transcript[3].Output) != "ok" || stringValue(parsed.Transcript[3].Status) != "completed" {
		t.Fatalf("transcript[3] = %#v, want tool output on line 5", parsed.Transcript[3])
	}
}

func assertMixedCodexSessionTranscriptAssistantAndSystemEvents(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptEntry(t, parsed, 4, factoryapi.AssistantMessage, 6, "The package tests passed.")
	if parsed.Transcript[4].SourceType == nil || *parsed.Transcript[4].SourceType != "agent_message" {
		t.Fatalf("assistant transcript source type = %#v, want agent_message", parsed.Transcript[4].SourceType)
	}

	assertMixedCodexSessionTranscriptEntry(t, parsed, 5, factoryapi.SystemEvent, 7, "Applying follow-up patch")
	if parsed.Transcript[5].SourceType == nil || *parsed.Transcript[5].SourceType != "task_started" {
		t.Fatalf("system-event transcript source type = %#v, want task_started", parsed.Transcript[5].SourceType)
	}
}

func TestParseCodexSessionSummary_AcceptsLargeJSONLRecords(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"reasoning","content":"` + strings.Repeat("x", 128*1024) + `"}}`,
	}, "\n")

	summary, err := parseCodexSessionSummary(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session summary: %v", err)
	}
	if summary.LineCount != 2 || summary.EventCount != 2 || len(summary.Reasoning) != 1 {
		t.Fatalf("summary = %#v, want large response item parsed successfully", summary)
	}
}

func TestGetProviderSessionDetails_NotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithCodexRoot(t.TempDir())
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_IgnoresUnsupportedRolloutFileNames(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-backup-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-backup-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_RejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=codex&kind=session_id&id=../secret",
		"/provider-sessions/detail?provider=codex&kind=session_id&id=/tmp/rollout-session.jsonl",
		"/provider-sessions/detail?provider=codex&kind=session_id&id=session.with.dot",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCodexRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
		})
	}
}

func TestGetProviderSessionDetails_RejectsUnsupportedProviderOrKindByContract(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=openai&kind=session_id&id=sess-123",
		"/provider-sessions/detail?provider=codex&kind=path&id=sess-123",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCodexRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request parameter")
		})
	}
}

func TestGetProviderSessionDetails_RejectsSessionSymlinkOutsideConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideSessionPath := filepath.Join(outside, "rollout-sess-outside.jsonl")
	if err := os.WriteFile(outsideSessionPath, []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write outside session fixture: %v", err)
	}
	sessionDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.Symlink(outsideSessionPath, filepath.Join(sessionDir, "rollout-sess-outside.jsonl")); err != nil {
		t.Fatalf("create provider session symlink: %v", err)
	}

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess-outside", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
}

func TestGetProviderSessionDetails_RejectsSessionSymlinkOutsideConfiguredRootEvenWhenValidMatchExists(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess-shared", `{"type":"session_meta","id":"sess-shared"}`)
	outside := t.TempDir()
	outsideSessionPath := filepath.Join(outside, "rollout-2026-05-20T17-35-24-sess-shared.jsonl")
	if err := os.WriteFile(outsideSessionPath, []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write outside session fixture: %v", err)
	}
	sessionDir := filepath.Join(root, "2026", "05", "18")
	if err := os.Symlink(outsideSessionPath, filepath.Join(sessionDir, "rollout-2026-05-20T17-35-24-sess-shared.jsonl")); err != nil {
		t.Fatalf("create provider session symlink: %v", err)
	}

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess-shared", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
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
