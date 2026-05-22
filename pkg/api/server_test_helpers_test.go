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
	"regexp"
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
	return &interfaces.Token{
		ID:      id,
		PlaceID: placeID,
		Color: interfaces.TokenColor{
			WorkID:     workID,
			WorkTypeID: workTypeID,
		},
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

func embeddedDashboardAssetPath(t *testing.T, html string) string {
	t.Helper()

	pattern := regexp.MustCompile(`(?:src|href)="(/dashboard/ui/assets/[^"]+)"`)
	matches := pattern.FindStringSubmatch(html)
	if len(matches) != 2 {
		t.Fatalf("expected embedded dashboard asset path in html: %s", html)
	}

	return matches[1]
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

func engineStateWithRuntimeStatus(status interfaces.RuntimeStatus) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: status,
		Marking: petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
}

func validNamedFactoryBody(name, workType string) string {
	return fmt.Sprintf(`{"name":%q,%s`, name, strings.TrimPrefix(namedFactoryPayloadJSON(name, workType), "{"))
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
