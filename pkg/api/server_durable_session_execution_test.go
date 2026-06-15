package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestStartDurableFactorySessionAsync_RuntimeBackedSimpleFinalReturnsStableSession(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-async-001")
	first := postDurableAsyncStart(t, server.URL, request)
	if first.SessionId == "" {
		t.Fatal("sessionId missing from async start response")
	}
	if !strings.HasPrefix(first.SessionId, "dur-sess-") {
		t.Fatalf("sessionId = %q, want dur-sess- prefix", first.SessionId)
	}
	if first.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", first.Status)
	}
	if first.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", first.OrchestratorKind)
	}
	if first.ResolvedSource.SourceRef == nil || *first.ResolvedSource.SourceRef != workflowsource.ProjectClaudeWorkflowsDir+"/simple-final.js" {
		t.Fatalf("resolved source ref = %#v", first.ResolvedSource.SourceRef)
	}
	if first.Links == nil || first.Links.Session == nil || *first.Links.Session != "/factory-sessions/"+first.SessionId {
		t.Fatalf("links.session = %#v", first.Links)
	}

	replay := postDurableAsyncStart(t, server.URL, request)
	if replay.SessionId != first.SessionId {
		t.Fatalf("replay sessionId = %q, want %q", replay.SessionId, first.SessionId)
	}
	if replay.Status != first.Status {
		t.Fatalf("replay status = %q, want %q", replay.Status, first.Status)
	}
}

func TestStartDurableFactorySessionAsync_RequestIDConflictReturnsTypedError(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	first := runtimeBackedAsyncStartRequest("req-api-runtime-conflict-001")
	if _, err := postDurableAsyncStartRaw(t, server.URL, first); err != nil {
		t.Fatalf("first start: %v", err)
	}

	conflict := first
	conflict.Args = &map[string]any{
		"subject": "different",
		"count":   2,
		"prefix":  "you",
	}
	status, errResp := postDurableAsyncStartExpectError(t, server.URL, conflict)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if errResp.Code != factoryapi.EXECUTIONREQUESTIDCONFLICT {
		t.Fatalf("code = %q, want EXECUTION_REQUEST_ID_CONFLICT", errResp.Code)
	}
}

func TestStartDurableFactorySessionAsync_InvalidSourceDoesNotCreateSession(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	invalid := runtimeBackedAsyncStartRequest("req-api-runtime-invalid-001")
	invalid.Source.WorkflowName = strPtr("missing-workflow")
	status, errResp := postDurableAsyncStartExpectError(t, server.URL, invalid)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if errResp.Code != factoryapi.BADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}

	valid := runtimeBackedAsyncStartRequest("req-api-runtime-invalid-002")
	started := postDurableAsyncStart(t, server.URL, valid)
	if started.SessionId == "" {
		t.Fatal("expected valid start to create a session")
	}
}

func TestStartDurableFactorySessionAsync_MissingRequestIDReturnsValidationError(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-invalid-001")
	request.RequestId = ""
	status, errResp := postDurableAsyncStartExpectError(t, server.URL, request)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if errResp.Code != factoryapi.BADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}
	if !strings.Contains(errResp.Message, "requestId") {
		t.Fatalf("message = %q, want requestId validation detail", errResp.Message)
	}
}

func runtimeBackedAsyncStartRequest(requestID string) factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("simple-final"),
		},
		Args: &map[string]any{
			"subject": "api",
			"count":   2,
			"prefix":  "you",
		},
		RequestedPolicy: &factoryapi.FactorySessionRequestedPolicy{
			AdditionalProperties: map[string]any{"mode": "READ_ONLY"},
		},
	}
}

func postDurableAsyncStart(t *testing.T, serverURL string, request factoryapi.FactorySessionExecutionRequest) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	response, err := postDurableAsyncStartRaw(t, serverURL, request)
	if err != nil {
		t.Fatalf("POST /factory-sessions/async: %v", err)
	}
	return response
}

func postDurableAsyncStartRaw(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(serverURL+"/factory-sessions/async", "application/json", bytes.NewReader(body))
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var errResp factoryapi.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return factoryapi.FactorySessionExecutionResponse{}, errors.New(errResp.Message)
	}
	var response factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	return response, nil
}

func postDurableAsyncStartExpectError(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) (int, factoryapi.ErrorResponse) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(serverURL+"/factory-sessions/async", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /factory-sessions/async: %v", err)
	}
	defer resp.Body.Close()
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return resp.StatusCode, errResp
}

func setupAPIRuntimeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "orchestrators", "javascript", "runtime", "testdata", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func strPtr(value string) *string {
	return &value
}
