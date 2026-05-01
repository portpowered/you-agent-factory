package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/apisurface"
	factoryconfig "github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"go.uber.org/zap"
)

// --- Tests ---

func newTestServer(f *testutil.MockFactory) *Server {
	logger, _ := zap.NewDevelopment()
	return NewServer(f, 8080, logger)
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

func TestSubmitWork(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"workTypeName": "prd", "traceId": "test-trace-1", "payload": {"title": "Draft PRD"}}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TraceId != "test-trace-1" {
		t.Errorf("expected trace_id test-trace-1, got %s", resp.TraceId)
	}

	// Verify the factory received canonical WorkRequest data through
	// SubmitWorkRequest and returned the accepted trace metadata.
	if len(mf.WorkRequests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(mf.WorkRequests))
	}
	if mf.WorkRequests[0].Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request type = %q, want FACTORY_REQUEST_BATCH", mf.WorkRequests[0].Type)
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("expected 1 submitted request, got %d", len(mf.Submitted))
	}
	if mf.Submitted[0].WorkTypeID != "prd" {
		t.Errorf("expected work type name prd, got %s", mf.Submitted[0].WorkTypeID)
	}
	if string(mf.Submitted[0].Payload) != `{"title":"Draft PRD"}` {
		t.Errorf("payload = %s, want JSON object payload", string(mf.Submitted[0].Payload))
	}

}

func TestSubmitWork_CurrentChainingTraceIDPreservesRuntimeBoundary(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"workTypeName": "prd", "currentChainingTraceId": "chain-submit-1", "payload": {"title": "Draft PRD"}}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("work requests = %#v, want one submitted work request", mf.WorkRequests)
	}
	if mf.WorkRequests[0].CurrentChainingTraceID != "chain-submit-1" {
		t.Fatalf("work request current chaining trace ID = %q, want chain-submit-1", mf.WorkRequests[0].CurrentChainingTraceID)
	}
	if mf.WorkRequests[0].Works[0].CurrentChainingTraceID != "chain-submit-1" {
		t.Fatalf("work current chaining trace ID = %q, want chain-submit-1", mf.WorkRequests[0].Works[0].CurrentChainingTraceID)
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("normalized submissions = %d, want 1", len(mf.Submitted))
	}
	if mf.Submitted[0].CurrentChainingTraceID != "chain-submit-1" {
		t.Fatalf("normalized current chaining trace ID = %q, want chain-submit-1", mf.Submitted[0].CurrentChainingTraceID)
	}
	if mf.Submitted[0].TraceID != "chain-submit-1" {
		t.Fatalf("normalized trace_id = %q, want chain-submit-1", mf.Submitted[0].TraceID)
	}
}

func TestSubmitWork_MatchingTraceAliasesNormalizeAtBoundary(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"workTypeName": "prd", "currentChainingTraceId": "chain-submit-1", "traceId": "chain-submit-1", "payload": {"title": "Draft PRD"}}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("work requests = %#v, want one submitted work request", mf.WorkRequests)
	}
	if mf.WorkRequests[0].CurrentChainingTraceID != "chain-submit-1" {
		t.Fatalf("work request current chaining trace ID = %q, want chain-submit-1", mf.WorkRequests[0].CurrentChainingTraceID)
	}
	if mf.WorkRequests[0].Works[0].CurrentChainingTraceID != "chain-submit-1" {
		t.Fatalf("work current chaining trace ID = %q, want chain-submit-1", mf.WorkRequests[0].Works[0].CurrentChainingTraceID)
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("normalized submissions = %d, want 1", len(mf.Submitted))
	}
	if mf.Submitted[0].CurrentChainingTraceID != "chain-submit-1" {
		t.Fatalf("normalized current chaining trace ID = %q, want chain-submit-1", mf.Submitted[0].CurrentChainingTraceID)
	}
	if mf.Submitted[0].TraceID != "chain-submit-1" {
		t.Fatalf("normalized trace_id = %q, want chain-submit-1", mf.Submitted[0].TraceID)
	}
}

func TestSubmitWork_ConflictingCurrentChainingTraceIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"workTypeName": "prd", "currentChainingTraceId": "chain-submit-1", "traceId": "trace-submit-1", "payload": {"title": "Draft PRD"}}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "currentChainingTraceId and traceId must match when both are provided")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWork_WorkTypeIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"work_type_id": "legacy-task", "traceId": "test-trace-legacy", "payload": {"title": "Legacy"}}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "work_type_id is not supported; use workTypeName")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWork_PreservesRuntimeRelations(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"workTypeName":"prd","payload":{"title":"Draft PRD"},"relations":[{"type":"DEPENDS_ON","targetWorkId":"review-work","requiredState":"complete"}]}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted count = %d, want 1", len(mf.Submitted))
	}
	if len(mf.Submitted[0].Relations) != 1 {
		t.Fatalf("submitted relations = %d, want 1", len(mf.Submitted[0].Relations))
	}
	relation := mf.Submitted[0].Relations[0]
	if relation.Type != interfaces.RelationDependsOn || relation.TargetWorkID != "review-work" || relation.RequiredState != "complete" {
		t.Fatalf("submitted relation = %#v, want dependency on review-work at complete", relation)
	}
}

func TestCreateFactory_ReturnsCreatedFactoryShape(t *testing.T) {
	mf := &testutil.MockFactory{}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.CreatedFactories) != 1 {
		t.Fatalf("created factories = %d, want 1", len(mf.CreatedFactories))
	}

	var created factoryapi.Factory
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create factory response: %v", err)
	}
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	if created.WorkTypes == nil || len(*created.WorkTypes) != 1 || (*created.WorkTypes)[0].Name != "beta-task" {
		t.Fatalf("created factory work types = %#v, want beta-task", created.WorkTypes)
	}
}

func TestGetCurrentFactory_ReturnsFactoryShape(t *testing.T) {
	mf := &testutil.MockFactory{
		CurrentNamedFactory: &factoryapi.Factory{
			Name: factoryapi.FactoryName("beta"),
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "beta-task",
				States: []factoryapi.WorkState{
					{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
				},
			}},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodGet, "/factory/~current", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var current factoryapi.Factory
	if err := json.NewDecoder(rec.Body).Decode(&current); err != nil {
		t.Fatalf("decode current factory response: %v", err)
	}
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	if current.WorkTypes == nil || len(*current.WorkTypes) != 1 || (*current.WorkTypes)[0].Name != "beta-task" {
		t.Fatalf("current factory work types = %#v, want beta-task", current.WorkTypes)
	}
}

func TestCreateFactory_RejectsDuplicateFactoryName(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		CreateNamedFactoryErr: factoryconfig.ErrNamedFactoryAlreadyExists,
	})

	req := httptest.NewRequest(http.MethodPost, "/factory", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusConflict, "FACTORY_ALREADY_EXISTS", "Named factory already exists.")
}

func TestCreateFactory_RejectsInvalidFactoryName(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	req := httptest.NewRequest(http.MethodPost, "/factory", bytes.NewBufferString(validNamedFactoryBody("nested/name", "task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "INVALID_FACTORY_NAME", "Factory name must be a safe directory segment without path separators.")
}

func TestCreateFactory_RejectsInvalidFactoryPayload(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		CreateNamedFactoryErr: apisurface.ErrInvalidNamedFactory,
	})

	body := `{"name":"beta","workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"claude","executorProvider":"script_wrap","model":"claude-sonnet-4-20250514"}],"workstations":[{"name":"plan-task","kind":"STANDARD","type":"MODEL_WORKSTATION","worker":"missing-worker","inputs":[{"workType":"beta-task","state":"init"}],"outputs":[{"workType":"beta-task","state":"done"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/factory", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "INVALID_FACTORY", "Factory payload is not a valid Agent Factory definition.")
}

func TestCreateFactory_RejectsNonIdleRuntime(t *testing.T) {
	mf := &testutil.MockFactory{
		EngineState:            engineStateWithRuntimeStatus(interfaces.RuntimeStatusActive),
		CreateNamedFactoryErr:  apisurface.ErrFactoryActivationRequiresIdle,
		CurrentNamedFactoryErr: apisurface.ErrCurrentNamedFactoryNotFound,
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusConflict, "FACTORY_NOT_IDLE", "Current factory runtime must be idle before activation.")
}

func TestGetCurrentFactory_ReturnsNotFoundWithoutStoredNamedFactory(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{CurrentNamedFactoryErr: apisurface.ErrCurrentNamedFactoryNotFound})

	req := httptest.NewRequest(http.MethodGet, "/factory/~current", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "Current named factory not found.")
}

func TestSubmitWork_WorkTypeNameWithWorkTypeIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"workTypeName": "tasks", "work_type_id": "legacy-task", "payload": "fix lint"}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "work_type_id is not supported; use workTypeName")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWorkMissingWorkType(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"traceId": "test-trace-1"}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "workTypeName is required")
}

func TestSubmitWorkMarkdownPayload(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"workTypeName": "tasks", "traceId": "trace-markdown", "payload": "# Fix lint\n\nRun gofmt."}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("expected 1 submitted request, got %d", len(mf.Submitted))
	}
	if mf.Submitted[0].WorkTypeID != "tasks" {
		t.Fatalf("WorkTypeID = %q, want tasks", mf.Submitted[0].WorkTypeID)
	}
	if string(mf.Submitted[0].Payload) != `"# Fix lint\n\nRun gofmt."` {
		t.Fatalf("payload = %s, want marshaled markdown string", string(mf.Submitted[0].Payload))
	}
}

func TestSubmitWorkInvalidPayload_ReturnsDocumentedBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(`{"workTypeName":`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

func TestSubmitWorkUnknownWorkTypeReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
		SubmitErr: errors.New(`work_request: works[0] ("unknown-work") references unknown work type "unknown"`),
	}
	srv := newTestServer(mf)

	body := `{"name": "unknown-work", "workTypeName": "unknown", "payload": "fix lint"}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("unknown-work") references unknown work type name "unknown"`)
}

func TestSubmitWorkAutoTraceID(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{"workTypeName": "prd"}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var resp factoryapi.SubmitWorkResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.TraceId == "" {
		t.Error("expected auto-generated trace_id, got empty")
	}
}

func TestUpsertWorkRequest_FirstSubmitAndRepeatedRequestID(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	firstBody := `{
		"requestId": "request-api-1",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "draft", "workTypeName": "task", "traceId": "trace-original", "payload": {"title": "Draft"}}
		]
	}`
	retryBody := `{
		"requestId": "request-api-1",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "changed-draft", "workTypeName": "task", "traceId": "trace-retry", "payload": {"title": "Changed retry"}}
		]
	}`

	var firstTraceID string
	for i, body := range []string{firstBody, retryBody} {
		req := httptest.NewRequest(http.MethodPut, "/work-requests/request-api-1", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
		}

		var resp factoryapi.UpsertWorkRequestResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode upsert response: %v", err)
		}
		if resp.RequestId != "request-api-1" {
			t.Fatalf("request_id = %q, want request-api-1", resp.RequestId)
		}
		if resp.TraceId == "" {
			t.Fatal("trace_id should be returned")
		}
		if i == 0 {
			firstTraceID = resp.TraceId
		} else if resp.TraceId != firstTraceID {
			t.Fatalf("repeated trace_id = %q, want original %q", resp.TraceId, firstTraceID)
		}
	}

	if len(mf.WorkRequests) != 1 {
		t.Fatalf("work request submissions = %d, want 1", len(mf.WorkRequests))
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("normalized submissions = %d, want 1", len(mf.Submitted))
	}
	if mf.Submitted[0].RequestID != "request-api-1" || mf.Submitted[0].TraceID == "" {
		t.Fatalf("submitted request = %#v, want request and trace metadata", mf.Submitted[0])
	}
	if mf.Submitted[0].TraceID != "trace-original" {
		t.Fatalf("submitted trace_id = %q, want original trace", mf.Submitted[0].TraceID)
	}
	if mf.Submitted[0].Name != "draft" {
		t.Fatalf("submitted name = %q, want original name", mf.Submitted[0].Name)
	}
}

func TestUpsertWorkRequest_MapsWorkTypeNameAndRelationsToRuntime(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{
		"requestId": "request-api-batch",
		"currentChainingTraceId": "chain-request-batch",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "draft", "workTypeName": "task", "state": "queued", "currentChainingTraceId": "chain-draft", "traceId": "chain-draft", "payload": {"title": "Draft"}},
			{"name": "review", "workTypeName": "review", "payload": "review draft"}
		],
		"relations": [
			{"type": "DEPENDS_ON", "sourceWorkName": "review", "targetWorkName": "draft", "requiredState": "complete"}
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/work-requests/request-api-batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 {
		t.Fatalf("work request submissions = %d, want 1", len(mf.WorkRequests))
	}
	submittedRequest := mf.WorkRequests[0]
	if len(submittedRequest.Works) != 2 {
		t.Fatalf("work request works = %d, want 2", len(submittedRequest.Works))
	}
	if submittedRequest.CurrentChainingTraceID != "chain-request-batch" {
		t.Fatalf("work request current chaining trace ID = %q, want chain-request-batch", submittedRequest.CurrentChainingTraceID)
	}
	if submittedRequest.Works[0].WorkTypeID != "task" || submittedRequest.Works[1].WorkTypeID != "review" {
		t.Fatalf("domain work types = %#v, want task and review", submittedRequest.Works)
	}
	if submittedRequest.Works[0].CurrentChainingTraceID != "chain-draft" {
		t.Fatalf("draft current chaining trace ID = %q, want chain-draft", submittedRequest.Works[0].CurrentChainingTraceID)
	}
	if submittedRequest.Works[1].CurrentChainingTraceID != "chain-request-batch" {
		t.Fatalf("review current chaining trace ID = %q, want chain-request-batch", submittedRequest.Works[1].CurrentChainingTraceID)
	}
	if submittedRequest.Works[0].State != "queued" {
		t.Fatalf("domain work state = %q, want queued", submittedRequest.Works[0].State)
	}
	if len(submittedRequest.Relations) != 1 {
		t.Fatalf("work request relations = %d, want 1", len(submittedRequest.Relations))
	}
	if submittedRequest.Relations[0].SourceWorkName != "review" || submittedRequest.Relations[0].TargetWorkName != "draft" {
		t.Fatalf("domain relation = %#v, want review depends on draft", submittedRequest.Relations[0])
	}
	if len(mf.Submitted) != 2 {
		t.Fatalf("normalized submissions = %d, want 2", len(mf.Submitted))
	}
	if mf.Submitted[0].WorkTypeID != "task" || mf.Submitted[1].WorkTypeID != "review" {
		t.Fatalf("normalized work types = %#v, want task and review", mf.Submitted)
	}
	if mf.Submitted[0].CurrentChainingTraceID != "chain-draft" {
		t.Fatalf("normalized draft current chaining trace ID = %q, want chain-draft", mf.Submitted[0].CurrentChainingTraceID)
	}
	if mf.Submitted[1].CurrentChainingTraceID != "chain-request-batch" {
		t.Fatalf("normalized review current chaining trace ID = %q, want chain-request-batch", mf.Submitted[1].CurrentChainingTraceID)
	}
	if mf.Submitted[0].TargetState != "queued" {
		t.Fatalf("normalized target state = %q, want queued", mf.Submitted[0].TargetState)
	}
	if len(mf.Submitted[1].Relations) != 1 {
		t.Fatalf("review relations = %d, want 1", len(mf.Submitted[1].Relations))
	}
	relation := mf.Submitted[1].Relations[0]
	if relation.TargetWorkID != "batch-request-api-batch-draft" || relation.RequiredState != "complete" {
		t.Fatalf("normalized relation = %#v, want dependency on draft completion", relation)
	}
}

func TestUpsertWorkRequest_AcceptsParentChildRelationsByWorkName(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{
		"requestId": "request-api-parent-child",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "parent", "workTypeName": "task", "traceId": "trace-parent-child", "payload": {"title": "Parent"}},
			{"name": "prerequisite", "workTypeName": "task", "payload": {"title": "Prerequisite"}},
			{"name": "child", "workTypeName": "task", "payload": {"title": "Child"}}
		],
		"relations": [
			{"type": "PARENT_CHILD", "sourceWorkName": "child", "targetWorkName": "parent"},
			{"type": "DEPENDS_ON", "sourceWorkName": "child", "targetWorkName": "prerequisite"}
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/work-requests/request-api-parent-child", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 {
		t.Fatalf("work request submissions = %d, want 1", len(mf.WorkRequests))
	}
	if len(mf.WorkRequests[0].Relations) != 2 {
		t.Fatalf("work request relations = %d, want 2", len(mf.WorkRequests[0].Relations))
	}
	if mf.WorkRequests[0].Relations[0].Type != interfaces.WorkRelationParentChild {
		t.Fatalf("domain parent-child relation = %#v, want PARENT_CHILD", mf.WorkRequests[0].Relations[0])
	}
	if len(mf.Submitted) != 3 {
		t.Fatalf("normalized submissions = %d, want 3", len(mf.Submitted))
	}
	child := submittedRequestNamed(t, mf.Submitted, "child")
	if child.Name == "" {
		t.Fatal("normalized child submit request not found")
	}
	if child.TraceID != "trace-parent-child" {
		t.Fatalf("child trace ID = %q, want trace-parent-child", child.TraceID)
	}
	if len(child.Relations) != 2 {
		t.Fatalf("child relations = %d, want 2", len(child.Relations))
	}
	assertSubmittedChildRelations(t, child.Relations)
}

func TestUpsertWorkRequest_WorkTypeIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{
		"requestId": "request-api-legacy",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "draft", "work_type_id": "legacy-task", "payload": {"title": "Draft"}}
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/work-requests/request-api-legacy", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].work_type_id is not supported; use workTypeName")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("work request submissions = %d, want 0", len(mf.WorkRequests))
	}
	if len(mf.Submitted) != 0 {
		t.Fatalf("normalized submissions = %#v, want none", mf.Submitted)
	}
}

func TestUpsertWorkRequest_TargetStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{
		"requestId": "request-api-state-alias",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "draft", "workTypeName": "task", "target_state": "queued", "payload": {"title": "Draft"}}
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/work-requests/request-api-state-alias", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].target_state is not supported; use state")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("work request submissions = %d, want 0", len(mf.WorkRequests))
	}
	if len(mf.Submitted) != 0 {
		t.Fatalf("normalized submissions = %#v, want none", mf.Submitted)
	}
}

func TestUpsertWorkRequest_ConflictingCurrentChainingTraceIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	body := `{
		"requestId": "request-api-chaining-conflict",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "draft", "workTypeName": "task", "currentChainingTraceId": "chain-a", "traceId": "trace-b", "payload": {"title": "Draft"}}
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/work-requests/request-api-chaining-conflict", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].currentChainingTraceId and traceId must match when both are provided")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("work request submissions = %d, want 0", len(mf.WorkRequests))
	}
	if len(mf.Submitted) != 0 {
		t.Fatalf("normalized submissions = %#v, want none", mf.Submitted)
	}
}

func TestUpsertWorkRequest_InvalidExplicitStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
		Net: &state.Net{
			WorkTypes: map[string]*state.WorkType{
				"task": {
					ID: "task",
					States: []state.StateDefinition{
						{Value: "init", Category: state.StateCategoryInitial},
						{Value: "complete", Category: state.StateCategoryTerminal},
					},
				},
			},
		},
	}
	srv := newTestServer(mf)

	body := `{
		"requestId": "request-api-invalid-state",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "draft", "workTypeName": "task", "state": "queued", "payload": {"title": "Draft"}}
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/work-requests/request-api-invalid-state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("draft") references unknown state "queued" for work type name "task"`)
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("work request submissions = %d, want 0", len(mf.WorkRequests))
	}
	if len(mf.Submitted) != 0 {
		t.Fatalf("normalized submissions = %#v, want none", mf.Submitted)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-api-test-table review=2026-07-18 removal=split-test-scenarios-before-next-api-validation-expansion
func TestUpsertWorkRequestValidationFailures(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    string
		factory *testutil.MockFactory
		wantMsg string
	}{
		{
			name:    "invalid_json",
			path:    "/work-requests/request-api-1",
			body:    `{"requestId":`,
			wantMsg: "invalid request payload",
		},
		{
			name:    "missing_required_request_id",
			path:    "/work-requests/request-api-1",
			body:    `{"type": "FACTORY_REQUEST_BATCH", "works": [{"name": "draft", "workTypeName": "task"}]}`,
			wantMsg: "requestId is required",
		},
		{
			name:    "path_body_mismatch",
			path:    "/work-requests/request-api-1",
			body:    `{"requestId": "request-api-2", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "draft", "workTypeName": "task"}]}`,
			wantMsg: "request_id path and requestId body must match",
		},
		{
			name:    "cycle_error",
			path:    "/work-requests/request-api-1",
			body:    `{"requestId": "request-api-1", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "a", "workTypeName": "task"}, {"name": "b", "workTypeName": "task"}], "relations": [{"type": "DEPENDS_ON", "sourceWorkName": "a", "targetWorkName": "b"}, {"type": "DEPENDS_ON", "sourceWorkName": "b", "targetWorkName": "a"}]}`,
			wantMsg: `work_request: dependency cycle detected involving "a"`,
		},
		{
			name:    "malformed_relation",
			path:    "/work-requests/request-api-1",
			body:    `{"requestId": "request-api-1", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "a", "workTypeName": "task"}], "relations": [{"type": "DEPENDS_ON", "sourceWorkName": "a", "targetWorkName": "missing"}]}`,
			wantMsg: `work_request: relations[0] references unknown targetWorkName "missing"`,
		},
		{
			name:    "self_parenting_relation",
			path:    "/work-requests/request-api-1",
			body:    `{"requestId": "request-api-1", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "a", "workTypeName": "task"}], "relations": [{"type": "PARENT_CHILD", "sourceWorkName": "a", "targetWorkName": "a"}]}`,
			wantMsg: `work_request: relations[0] has self-parenting on "a"`,
		},
		{
			name:    "duplicate_parent_child_relation",
			path:    "/work-requests/request-api-1",
			body:    `{"requestId": "request-api-1", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "parent", "workTypeName": "task"}, {"name": "child", "workTypeName": "task"}], "relations": [{"type": "PARENT_CHILD", "sourceWorkName": "child", "targetWorkName": "parent"}, {"type": "PARENT_CHILD", "sourceWorkName": "child", "targetWorkName": "parent"}]}`,
			wantMsg: `work_request: relations[1] duplicates relations[0] ("PARENT_CHILD" "child" -> "parent")`,
		},
		{
			name:    "missing_work_type_name",
			path:    "/work-requests/request-api-1",
			body:    `{"requestId": "request-api-1", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "draft"}]}`,
			wantMsg: `work_request: works[0] ("draft") is missing workTypeName`,
		},
		{
			name:    "work_type_id_not_supported",
			path:    "/work-requests/request-api-1",
			body:    `{"requestId": "request-api-1", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "draft", "workTypeName": "task", "work_type_id": "legacy-task"}]}`,
			wantMsg: `works[0].work_type_id is not supported; use workTypeName`,
		},
		{
			name: "unknown_work_type",
			path: "/work-requests/request-api-1",
			body: `{"requestId": "request-api-1", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "draft", "workTypeName": "unknown"}]}`,
			factory: &testutil.MockFactory{
				SubmitWorkRequestErr: errors.New(`work_request: works[0] ("draft") references unknown work type "unknown"`),
			},
			wantMsg: `work_request: works[0] ("draft") references unknown work type name "unknown"`,
		},
		{
			name: "invalid_dependency_required_state",
			path: "/work-requests/request-api-1",
			body: `{"requestId": "request-api-1", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "draft", "workTypeName": "task"}, {"name": "review", "workTypeName": "task"}], "relations": [{"type": "DEPENDS_ON", "sourceWorkName": "review", "targetWorkName": "draft", "requiredState": "queued"}]}`,
			factory: &testutil.MockFactory{
				Net: &state.Net{
					WorkTypes: map[string]*state.WorkType{
						"task": {
							ID: "task",
							States: []state.StateDefinition{
								{Value: "init", Category: state.StateCategoryInitial},
								{Value: "complete", Category: state.StateCategoryTerminal},
							},
						},
					},
				},
			},
			wantMsg: `work_request: relations[0] references unknown requiredState "queued" for target work type name "task"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := tc.factory
			if mf == nil {
				mf = &testutil.MockFactory{}
			}
			mf.Marking = &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}
			srv := newTestServer(mf)

			req := httptest.NewRequest(http.MethodPut, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", tc.wantMsg)
			if len(mf.Submitted) != 0 {
				t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
			}
		})
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-api-contract-smoke review=2026-07-18 removal=split-submit-and-list-assertions-before-next-work-api-change
func TestSubmitWorkThenListWork_ConfirmsObservedJSONFields(t *testing.T) {
	now := time.Date(2026, 4, 12, 16, 30, 0, 0, time.UTC)
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	submitBody := `{
		"name": "Inventory story",
		"workTypeName": "task",
		"traceId": "trace-inventory-1",
		"payload": {"title": "Document current API"},
		"tags": {"branch": "api-standardization"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(submitBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /work status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var submitResp factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(rec.Body).Decode(&submitResp); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if submitResp.TraceId != "trace-inventory-1" {
		t.Fatalf("submit trace_id = %q, want trace-inventory-1", submitResp.TraceId)
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted count = %d, want 1", len(mf.Submitted))
	}
	submitted := mf.Submitted[0]
	if submitted.Name != "Inventory story" || submitted.WorkTypeID != "task" || submitted.TraceID != "trace-inventory-1" {
		t.Fatalf("submitted request = %#v, want name/work type/trace from JSON body", submitted)
	}

	mf.Marking.Tokens["tok-inventory-1"] = &interfaces.Token{
		ID:      "tok-inventory-1",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			Name:       submitted.Name,
			WorkID:     "work-inventory-1",
			WorkTypeID: submitted.WorkTypeID,
			TraceID:    submitted.TraceID,
			Tags:       submitted.Tags,
		},
		CreatedAt: now,
		EnteredAt: now,
	}

	req = httptest.NewRequest(http.MethodGet, "/work", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var listResp factoryapi.ListWorkResponse
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Results) != 1 {
		t.Fatalf("work result count = %d, want 1", len(listResp.Results))
	}

	token := listResp.Results[0]
	if token.Id != "tok-inventory-1" {
		t.Fatalf("token id = %q, want tok-inventory-1", token.Id)
	}
	if token.PlaceId != "task:init" {
		t.Fatalf("place_id = %q, want task:init", token.PlaceId)
	}
	if stringValue(token.Name) != "Inventory story" {
		t.Fatalf("name = %q, want Inventory story", stringValue(token.Name))
	}
	if token.WorkId != "work-inventory-1" {
		t.Fatalf("work_id = %q, want work-inventory-1", token.WorkId)
	}
	if token.WorkType != "task" {
		t.Fatalf("work_type = %q, want task", token.WorkType)
	}
	if token.TraceId != "trace-inventory-1" {
		t.Fatalf("trace_id = %q, want trace-inventory-1", token.TraceId)
	}
	if token.Tags == nil || (*token.Tags)["branch"] != "api-standardization" {
		t.Fatalf("tags = %#v, want branch api-standardization", token.Tags)
	}
	if token.CreatedAt.Format(time.RFC3339) != "2026-04-12T16:30:00Z" {
		t.Fatalf("created_at = %q, want RFC3339 timestamp", token.CreatedAt)
	}
	if token.EnteredAt.Format(time.RFC3339) != "2026-04-12T16:30:00Z" {
		t.Fatalf("entered_at = %q, want RFC3339 timestamp", token.EnteredAt)
	}
	if token.History != nil {
		t.Fatalf("list token history = %#v, want omitted history", token.History)
	}
}

func TestGetWork(t *testing.T) {
	now := time.Now()
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-prd-1": {
					ID:      "tok-prd-1",
					PlaceID: "prd:init",
					Color: interfaces.TokenColor{
						WorkID:     "work-prd-1",
						WorkTypeID: "prd",
						TraceID:    "trace-1",
					},
					CreatedAt: now,
					EnteredAt: now,
					History: interfaces.TokenHistory{
						TotalVisits:         map[string]int{"execute": 1},
						ConsecutiveFailures: make(map[string]int),
						PlaceVisits:         map[string]int{"prd:init": 1},
					},
				},
			},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest("GET", "/work/tok-prd-1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp factoryapi.TokenResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Id != "tok-prd-1" {
		t.Errorf("expected tok-prd-1, got %s", resp.Id)
	}
	if resp.PlaceId != "prd:init" {
		t.Errorf("expected prd:init, got %s", resp.PlaceId)
	}
	if resp.History == nil {
		t.Error("expected history in single token response")
	}
	if resp.History.TotalVisits == nil || (*resp.History.TotalVisits)["execute"] != 1 {
		t.Error("expected execute visit count of 1")
	}
}

func TestGetWorkNotFound(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest("GET", "/work/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "token not found")
}

// portos:func-length-exception owner=agent-factory reason=legacy-status-contract-smoke review=2026-07-18 removal=split-status-sections-before-next-status-api-change
func TestGetStatus_ReturnsAggregateSnapshotStatus(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	topology := &state.Net{
		Places: map[string]*petri.Place{
			"task:init":            {ID: "task:init", TypeID: "task", State: "init"},
			"task:review":          {ID: "task:review", TypeID: "task", State: "review"},
			"task:complete":        {ID: "task:complete", TypeID: "task", State: "complete"},
			"task:failed":          {ID: "task:failed", TypeID: "task", State: "failed"},
			"agent-slot:available": {ID: "agent-slot:available", TypeID: "agent-slot", State: "available"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "complete", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
		Resources: map[string]*state.ResourceDef{
			"agent-slot": {ID: "agent-slot", Name: "agent-slot", Capacity: 2},
		},
	}
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
		Topology:      topology,
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-init": {
				ID: "tok-init", PlaceID: "task:init",
				Color:     interfaces.TokenColor{WorkID: "work-init", WorkTypeID: "task"},
				CreatedAt: now, EnteredAt: now,
			},
			"tok-review": {
				ID: "tok-review", PlaceID: "task:review",
				Color:     interfaces.TokenColor{WorkID: "work-review", WorkTypeID: "task"},
				CreatedAt: now, EnteredAt: now,
			},
			"tok-complete": {
				ID: "tok-complete", PlaceID: "task:complete",
				Color:     interfaces.TokenColor{WorkID: "work-complete", WorkTypeID: "task"},
				CreatedAt: now, EnteredAt: now,
			},
			"tok-failed": {
				ID: "tok-failed", PlaceID: "task:failed",
				Color:     interfaces.TokenColor{WorkID: "work-failed", WorkTypeID: "task"},
				CreatedAt: now, EnteredAt: now,
			},
			"agent-slot:resource:0": {
				ID: "agent-slot:resource:0", PlaceID: "agent-slot:available",
				Color:     interfaces.TokenColor{DataType: interfaces.DataTypeResource},
				CreatedAt: now, EnteredAt: now,
			},
			"tok-time": {
				ID: "tok-time", PlaceID: interfaces.SystemTimePendingPlaceID,
				Color: interfaces.TokenColor{
					WorkID:     "time-daily-refresh",
					WorkTypeID: interfaces.SystemTimeWorkTypeID,
					TraceID:    "trace-time",
				},
				CreatedAt: now, EnteredAt: now,
			},
		}},
	}
	srv := newTestServer(&testutil.MockFactory{EngineState: snapshot})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /status status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var resp factoryapi.StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if resp.FactoryState != "RUNNING" || resp.RuntimeStatus != "ACTIVE" || resp.TotalTokens != 5 {
		t.Fatalf("status response = %#v, want RUNNING/ACTIVE with 5 tokens", resp)
	}
	if resp.Categories.Initial != 1 || resp.Categories.Processing != 1 || resp.Categories.Terminal != 1 || resp.Categories.Failed != 1 {
		t.Fatalf("categories = %#v, want one token in each category", resp.Categories)
	}
	if resp.Resources == nil || len(*resp.Resources) != 1 {
		t.Fatalf("resources = %#v, want one resource summary", resp.Resources)
	}
	resource := (*resp.Resources)[0]
	if resource.Name != "agent-slot" || resource.Available != 1 || resource.Total != 2 {
		t.Fatalf("resource = %#v, want agent-slot 1/2", resource)
	}
}

func TestListWork_HidesInternalTimeWorkTokens(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-story": {
				ID:      "tok-story",
				PlaceID: "story:init",
				Color: interfaces.TokenColor{
					WorkID:     "work-story",
					WorkTypeID: "story",
					TraceID:    "trace-story",
				},
				CreatedAt: now,
				EnteredAt: now,
			},
			"tok-time": {
				ID:      "tok-time",
				PlaceID: interfaces.SystemTimePendingPlaceID,
				Color: interfaces.TokenColor{
					WorkID:     "time-daily-refresh",
					WorkTypeID: interfaces.SystemTimeWorkTypeID,
					TraceID:    "trace-time",
					Tags: map[string]string{
						interfaces.TimeWorkTagKeyCronWorkstation: "daily-refresh",
					},
				},
				CreatedAt: now,
				EnteredAt: now,
			},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp factoryapi.ListWorkResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Id != "tok-story" {
		t.Fatalf("listed tokens = %#v, want only customer token", resp.Results)
	}
	if resp.PaginationContext != nil {
		t.Fatalf("pagination context = %#v, want none after internal token filtering", resp.PaginationContext)
	}
}

func TestGetWork_HidesInternalTimeWorkToken(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-time": {
				ID:      "tok-time",
				PlaceID: interfaces.SystemTimePendingPlaceID,
				Color: interfaces.TokenColor{
					WorkID:     "time-daily-refresh",
					WorkTypeID: interfaces.SystemTimeWorkTypeID,
					TraceID:    "trace-time",
				},
				CreatedAt: now,
				EnteredAt: now,
			},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/work/tok-time", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "token not found")
}

func TestDeprecatedFactoryApiRoutesAreNotRegistered(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	for _, path := range []string{
		"/dashboard",
		"/dashboard/stream",
		"/state",
		"/traces/trace-id",
		"/work/token-1/trace",
		"/workflows",
		"/workflows/wf-1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestGetDashboardUI_ReturnsEmbeddedShell(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{
		"<title>Agent Factory Dashboard</title>",
		"<div id=\"root\"></div>",
		"/dashboard/ui/assets/",
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected embedded dashboard shell to contain %q, got body: %s", want, rec.Body.String())
		}
	}
}

func TestGetDashboardUI_ServesEmbeddedAsset(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	shellReq := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	shellRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(shellRec, shellReq)

	assetPath := embeddedDashboardAssetPath(t, shellRec.Body.String())

	assetReq := httptest.NewRequest(http.MethodGet, assetPath, nil)
	assetRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(assetRec, assetReq)

	if assetRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", assetRec.Code, assetRec.Body.String())
	}
	if assetRec.Body.Len() == 0 {
		t.Fatal("expected embedded asset body")
	}
}

func TestGetDashboardUI_FallbacksToIndexForClientRoutes(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*interfaces.Token),
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/ui/workstations/live", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<div id=\"root\"></div>") {
		t.Fatalf("expected SPA fallback index shell, got body: %s", rec.Body.String())
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-events-stream-smoke review=2026-07-18 removal=split-history-and-live-stream-assertions-before-next-events-api-change
func TestGetEvents_ReplaysHistoryThenStreamsLiveEventsInOrder(t *testing.T) {
	eventTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	runStartedFactory := factoryapi.Factory{
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "task",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			},
		}},
	}
	historical := []factoryapi.FactoryEvent{
		testFactoryEvent(t, factoryapi.FactoryEventTypeRunRequest, "factory-event/run-started", factoryapi.FactoryEventContext{
			Tick:      0,
			EventTime: eventTime,
		},
			factoryapi.RunRequestEventPayload{RecordedAt: eventTime, Factory: runStartedFactory}),
		testFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-event/initial-structure/0", factoryapi.FactoryEventContext{
			Tick:      0,
			EventTime: eventTime,
		},
			factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{Name: "factory"}}),
		testFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "factory-event/work-request/request-1", factoryapi.FactoryEventContext{
			Tick:      1,
			EventTime: time.Date(2026, 4, 8, 12, 0, 1, 0, time.UTC),
			RequestId: stringPointerForAPITest("request-1"),
		},
			factoryapi.WorkRequestEventPayload{Type: factoryapi.WorkRequestTypeFactoryRequestBatch}),
	}
	liveEvents := make(chan factoryapi.FactoryEvent, 1)
	mf := &testutil.MockFactory{
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: historical,
			Events:  liveEvents,
		},
	}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(NewServer(mf, 8080, logger).Handler())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(resp.Body)
	first := readSSEFactoryEvent(t, reader)
	second := readSSEFactoryEvent(t, reader)
	third := readSSEFactoryEvent(t, reader)
	if first.Id != historical[0].Id || second.Id != historical[1].Id || third.Id != historical[2].Id {
		t.Fatalf("historical event order = [%s %s %s], want [%s %s %s]",
			first.Id, second.Id, third.Id, historical[0].Id, historical[1].Id, historical[2].Id)
	}
	runStartedPayload, err := first.Payload.AsRunRequestEventPayload()
	if err != nil {
		t.Fatalf("decode run-started factory payload from SSE: %v", err)
	}
	if runStartedPayload.Factory.WorkTypes == nil || len(*runStartedPayload.Factory.WorkTypes) != 1 {
		t.Fatalf("run-started factory payload = %#v, want generated factory work types", runStartedPayload.Factory)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal streamed run-started event: %v", err)
	}
	if strings.Contains(string(firstJSON), "effectiveConfig") {
		t.Fatalf("streamed run-started event contains legacy effectiveConfig: %s", firstJSON)
	}

	live := testFactoryEvent(t, factoryapi.FactoryEventTypeDispatchRequest, "factory-event/dispatch-created/dispatch-1", factoryapi.FactoryEventContext{
		Tick:       2,
		EventTime:  time.Date(2026, 4, 8, 12, 0, 2, 0, time.UTC),
		DispatchId: stringPointerForAPITest("dispatch-1"),
	},
		factoryapi.DispatchRequestEventPayload{
			TransitionId: "review",
			Inputs:       []factoryapi.DispatchConsumedWorkRef{},
		})
	liveEvents <- live

	fourth := readSSEFactoryEvent(t, reader)
	if fourth.Id != live.Id || fourth.Type != factoryapi.FactoryEventTypeDispatchRequest || fourth.Context.Tick != 2 {
		t.Fatalf("live event = %#v, want request event at tick 2", fourth)
	}
}

func TestGetEvents_ClientDisconnectCancelsSubscription(t *testing.T) {
	liveEvents := make(chan factoryapi.FactoryEvent)
	mf := &testutil.MockFactory{
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: []factoryapi.FactoryEvent{
				testFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-event/initial-structure/0",
					factoryapi.FactoryEventContext{Tick: 0, EventTime: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)},
					factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{Name: "factory"}}),
			},
			Events: liveEvents,
		},
	}

	logger, _ := zap.NewDevelopment()
	server := httptest.NewServer(NewServer(mf, 8080, logger).Handler())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}

	_ = readSSEFactoryEvent(t, bufio.NewReader(resp.Body))
	cancel()
	_ = resp.Body.Close()

	streamCtx := mf.FactoryEventStreamCtx
	if streamCtx == nil {
		t.Fatal("expected subscription context")
	}

	select {
	case <-streamCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("expected subscription context cancellation after client disconnect")
	}
}

func TestDashboardSnapshotRoutes_RemovedFromRouter(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	for _, path := range []string{"/dashboard", "/dashboard/stream"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s status = %d, want route removed", path, rec.Code)
		}
	}
}

func TestListWork(t *testing.T) {
	now := time.Now()
	tokens := make(map[string]*interfaces.Token)
	for i := 1; i <= 3; i++ {
		id := "tok-prd-" + string(rune('0'+i))
		tokens[id] = &interfaces.Token{
			ID:      id,
			PlaceID: "prd:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-prd-" + string(rune('0'+i)),
				WorkTypeID: "prd",
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

	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: tokens},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest("GET", "/work?maxResults=2", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp factoryapi.ListWorkResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.PaginationContext == nil {
		t.Fatal("expected pagination context for more results")
	}
	if stringValue(resp.PaginationContext.NextToken) == "" {
		t.Error("expected non-empty nextToken")
	}
}

func TestListWork_InvalidMaxResultsDefaultsToCurrentBehavior(t *testing.T) {
	now := time.Now()
	tokens := make(map[string]*interfaces.Token)
	for i := 1; i <= 3; i++ {
		id := "tok-legacy-" + string(rune('0'+i))
		tokens[id] = &interfaces.Token{
			ID:      id,
			PlaceID: "legacy:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-legacy-" + string(rune('0'+i)),
				WorkTypeID: "legacy",
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

	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: tokens},
	})

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "invalid", path: "/work?maxResults=abc"},
		{name: "non_positive", path: "/work?maxResults=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
			}

			var resp factoryapi.ListWorkResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(resp.Results) != len(tokens) {
				t.Fatalf("expected defaulted response with %d results, got %d", len(tokens), len(resp.Results))
			}
			if resp.PaginationContext != nil {
				t.Fatalf("expected no pagination context when maxResults defaults to %d, got %#v", defaultMaxResults, resp.PaginationContext)
			}
		})
	}
}

func TestCreateFactoryRoute_RemovedFromRouter(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /factories status = %d, want route removed", rec.Code)
	}
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
		"project": %q,
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
			"modelProvider":"claude",
			"executorProvider":"script_wrap",
			"model":"claude-sonnet-4-20250514"
		}],
		"workstations": [{
			"name":"plan-task",
			"kind":"STANDARD",
			"type":"MODEL_WORKSTATION",
			"worker":"planner",
			"inputs":[{"workType":%q,"state":"init"}],
			"outputs":[{"workType":%q,"state":"done"}]
		}]
	}`, project, workType, workType, workType)
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
