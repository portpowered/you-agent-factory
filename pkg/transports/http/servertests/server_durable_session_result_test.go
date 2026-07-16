package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestGetFactorySessionResults_RuntimeBackedCompletedReturnsFinalResult(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-result-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("simple-final")
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	result := getDurableFactorySessionResult(t, server.URL, syncResult.SessionId, "")
	if result.SessionId != syncResult.SessionId {
		t.Fatalf("sessionId = %q, want %q", result.SessionId, syncResult.SessionId)
	}
	if result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one work content part", result.PrimaryResult)
	}
	jsonPart, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("primary result json part: %v", err)
	}
	payload, ok := jsonPart.Json.(map[string]any)
	if !ok {
		t.Fatalf("primary json payload = %#v, want object", jsonPart.Json)
	}
	if payload["echo"] != "you:api" {
		t.Fatalf("echo = %#v, want you:api", payload["echo"])
	}
}

func TestGetFactorySessionResults_RuntimeBackedAPIShapingMatchesServiceProjection(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-result-projection-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("simple-final")
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	serviceResult, err := service.GetResult(context.Background(), syncResult.SessionId, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("service GetResult: %v", err)
	}
	want := factorysession.ResultResponseToAPI(serviceResult)

	apiResult := getDurableFactorySessionResult(t, server.URL, syncResult.SessionId, "")
	assertFactorySessionResultJSONEqual(t, want, apiResult)
}

func TestGetFactorySessionResults_RuntimeBackedRunningReturnsNotReady(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "busy-loop.workflow.js", "busy-loop")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-result-running-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("busy-loop")
	started := postDurableAsyncStart(t, server.URL, request)

	result := getDurableFactorySessionResult(t, server.URL, started.SessionId, "")
	if result.SessionId != started.SessionId {
		t.Fatalf("sessionId = %q, want %q", result.SessionId, started.SessionId)
	}
	if result.ResultStatus != factoryapi.FactorySessionResultStatusNotReady {
		t.Fatalf("resultStatus = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Reason == nil || *result.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v, want RESULT_NOT_READY", result.Availability)
	}
}

func TestGetFactorySessionResults_RuntimeBackedSyncTimeoutReturnsAvailability(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "busy-loop.workflow.js", "busy-loop")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	timeoutMillis := int64(25)
	request := runtimeBackedAsyncStartRequest("req-api-runtime-result-timeout-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("busy-loop")
	request.Wait = &factoryapi.FactorySessionExecutionWaitOptions{
		TimeoutMillis: &timeoutMillis,
	}
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	result := getDurableFactorySessionResult(t, server.URL, syncResult.SessionId, "")
	if result.ResultStatus != factoryapi.FactorySessionResultStatusNotReady {
		t.Fatalf("resultStatus = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Reason == nil || *result.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, want SYNC_WAIT_TIMED_OUT", result.Availability)
	}
}

func TestGetFactorySessionResults_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-result-001/results")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/results: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestGetFactorySessionResults_LivePetriSessionReturnsNotFound(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/results", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func getDurableFactorySessionResult(t *testing.T, serverURL, sessionID, query string) factoryapi.FactorySessionResult {
	t.Helper()
	url := serverURL + "/factory-sessions/" + sessionID + "/results"
	if query != "" {
		url += "?" + query
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get result status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.FactorySessionResult
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode durable result response: %v", err)
	}
	return response
}

func assertFactorySessionResultJSONEqual(t *testing.T, want, got factoryapi.FactorySessionResult) {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want result: %v", err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got result: %v", err)
	}
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("API result JSON diverged from ResultResponseToAPI projection:\nwant %s\ngot  %s", wantJSON, gotJSON)
	}
}
