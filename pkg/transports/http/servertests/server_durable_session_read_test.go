package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestListFactorySessions_RuntimeBackedIncludesLiveAndPersistedScopes(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{
		DurableExecutionService: service,
		FactorySessions: factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{
					Id:         "~default",
					FactoryDir: "/workspace/root",
					FolderPath: "/workspace/root",
					Project:    "root",
					IsDefault:  true,
				},
			},
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-runtime-list-live-001"))
	waitForRuntimeSessionTerminal(t, service, started.SessionId)

	liveList := getFactorySessionList(t, server.URL, "live")
	if !containsLiveSessionID(liveList.Sessions, "~default") {
		t.Fatalf("live list sessions = %#v, want workspace row ~default", liveList.Sessions)
	}
	if containsLiveSessionID(liveList.Sessions, started.SessionId) {
		t.Fatalf("live list sessions = %#v, should not include terminal durable row %q", liveList.Sessions, started.SessionId)
	}

	persistedList := getFactorySessionList(t, server.URL, "persisted")
	if !containsDurableSessionID(persistedList, started.SessionId) {
		t.Fatalf("persisted durableSessions = %#v, want %q", persistedList.DurableSessions, started.SessionId)
	}
}

func TestGetFactorySession_RuntimeBackedReturnsTerminalReadModel(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-get-001")
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
	if syncResult.SessionId == "" {
		t.Fatal("sync sessionId missing")
	}

	read := getDurableFactorySession(t, server.URL, syncResult.SessionId)
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", read.Status)
	}
	if read.ResolvedSource.SourceRef == nil || *read.ResolvedSource.SourceRef != workflowsource.ProjectClaudeWorkflowsDir+"/simple-final.js" {
		t.Fatalf("resolved source ref = %#v", read.ResolvedSource.SourceRef)
	}
	if read.ResultSummary == nil || read.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", read.ResultSummary)
	}
	if read.SourceHash == nil || *read.SourceHash == "" {
		t.Fatal("expected source hash on durable session read")
	}
	if read.EffectivePolicyHash == nil || *read.EffectivePolicyHash == "" {
		t.Fatal("expected effective policy hash on durable session read")
	}
}

func TestGetFactorySession_RuntimeBackedReturnsOrderedPhasesAndLatestCheckpoint(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "progress-primitives.workflow.js", "progress-primitives")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	server := httptest.NewServer(newAPITestServer(&testutil.MockFactory{DurableExecutionService: service}).Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-introspection-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("progress-primitives")
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&started); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	read := getDurableFactorySession(t, server.URL, started.SessionId)
	if read.PhaseSummaries == nil || len(*read.PhaseSummaries) != 2 || (*read.PhaseSummaries)[0].Phase != "setup" || (*read.PhaseSummaries)[1].Phase != "execute" {
		t.Fatalf("phaseSummaries = %#v, want ordered setup, execute", read.PhaseSummaries)
	}
	if read.LatestCheckpoint == nil || read.LatestCheckpoint.Id == "" || read.LatestCheckpoint.Label == nil || *read.LatestCheckpoint.Label != "after-artifact" {
		t.Fatalf("latestCheckpoint = %#v, want stable after-artifact checkpoint", read.LatestCheckpoint)
	}
	if read.LatestCheckpoint.Phase == nil || *read.LatestCheckpoint.Phase != "execute" {
		t.Fatalf("latestCheckpoint.phase = %#v, want execute", read.LatestCheckpoint.Phase)
	}
}

func TestGetFactorySession_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-001")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}: %v", err)
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

func TestGetFactorySession_LivePetriSessionRemainsCompatible(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySession
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode live session response: %v", err)
	}
	if response.Id != "session-beta" {
		t.Fatalf("id = %q, want session-beta", response.Id)
	}
}

func getFactorySessionList(t *testing.T, serverURL, scope string) factoryapi.ListFactorySessionsResponse {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions?scope=" + scope)
	if err != nil {
		t.Fatalf("GET /factory-sessions?scope=%s: %v", scope, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.ListFactorySessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return response
}

func getDurableFactorySession(t *testing.T, serverURL, sessionID string) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s: %v", sessionID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.FactorySessionDurableReadModel
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode durable session response: %v", err)
	}
	return response
}

func containsLiveSessionID(sessions []factoryapi.FactorySessionSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.Id == sessionID {
			return true
		}
	}
	return false
}

func containsDurableSessionID(response factoryapi.ListFactorySessionsResponse, sessionID string) bool {
	if response.DurableSessions == nil {
		return false
	}
	for _, row := range *response.DurableSessions {
		if row.SessionId == sessionID {
			return true
		}
	}
	return false
}

func waitForRuntimeSessionTerminal(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		read, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession while waiting for terminal status: %v", err)
		}
		if read.Status == factorysessionexecution.LifecycleStatusSucceeded {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("session %q did not reach SUCCEEDED within timeout", sessionID)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}

func serverURLForLifecycle(t *testing.T, service factorysessionexecution.Service) string {
	t.Helper()
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	return server.URL
}

func startAPIRunningSessionForControl(t *testing.T, service *factorysessionexecution.FakeService) struct {
	SessionID string
} {
	t.Helper()
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-run-n-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/run-n.yaml",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync running session: %v", err)
	}
	return struct{ SessionID string }{SessionID: started.SessionID}
}

func newAPILifecycleRuntimeService(t *testing.T, fixtureName, workflowName string) factorysessionexecution.Service {
	t.Helper()
	projectRoot := setupAPILifecycleWorkflowFixture(t, fixtureName, workflowName)
	return newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
}

type apiLifecycleFailingChildProvider struct{}

func (apiLifecycleFailingChildProvider) Infer(
	_ context.Context,
	_ workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, workerprovider.NewProviderError(
		workerexecution.WorkFailureTypePermanentBadRequest,
		"simulated live child error",
		nil,
	)
}

func newAPILifecycleFailingChildRuntimeService(t *testing.T) factorysessionexecution.Service {
	t.Helper()
	projectRoot := setupAPILifecycleWorkflowFixture(t, "agent-run-live-child-failure.workflow.js", "agent-run-live-child-failure")
	return newAPIJavaScriptExecutionService(t, projectRoot, factorysessionexecution.ChildExecutorModeLive,
		apiLifecycleFailingChildProvider{})
}

func startRuntimeBackedFailedSessionWithDispatch(
	t *testing.T,
	service factorysessionexecution.Service,
) (sessionID string, dispatchID string) {
	t.Helper()
	const targetDispatchID = "dispatch-1"

	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-lifecycle-retry-failed-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-live-child-failure",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &factorysessionexecution.RuntimeOptions{
			ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", read.Status)
	}

	dispatchDetail, err := service.GetDispatch(context.Background(), completed.SessionID, targetDispatchID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.Status != factorysessionexecution.DispatchStatusFailed {
		t.Fatalf("dispatch status = %q, want FAILED", dispatchDetail.Status)
	}

	return completed.SessionID, targetDispatchID
}

func startRuntimeBackedDurableSession(
	t *testing.T,
	service factorysessionexecution.Service,
) factorysessionexecution.AsyncStartResult {
	t.Helper()
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-lifecycle-start-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	return started
}

func postFactorySessionLifecycleControl(
	t *testing.T,
	serverURL, sessionID, operation string,
	request *factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, int) {
	t.Helper()
	resp, err := postFactorySessionLifecycleControlRaw(t, serverURL, sessionID, operation, request)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/%s: %v", sessionID, operation, err)
	}
	defer resp.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	return response, resp.StatusCode
}

func postFactorySessionLifecycleControlExpectError(
	t *testing.T,
	serverURL, sessionID, operation string,
	request *factoryapi.FactorySessionLifecycleControlRequest,
) (int, factoryapi.ErrorResponse) {
	t.Helper()
	var body []byte
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		body = encoded
	}
	url := serverURL + "/factory-sessions/" + sessionID + "/" + operation
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return resp.StatusCode, errResp
}

func postFactorySessionLifecycleControlRaw(
	t *testing.T,
	serverURL, sessionID, operation string,
	request *factoryapi.FactorySessionLifecycleControlRequest,
) (*http.Response, error) {
	t.Helper()
	var body []byte
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		body = encoded
	}
	url := serverURL + "/factory-sessions/" + sessionID + "/" + operation
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 500 {
		var errResp factoryapi.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, errors.New(errResp.Message)
	}
	return resp, nil
}

func setupAPILifecycleWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "orchestrators", "javascript", "runtime", "testdata", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func newAPILifecycleFakeService(t *testing.T) *factorysessionexecution.FakeService {
	t.Helper()
	return newAPIFixtureExecutionService(t,
		filepath.Join("..", "testdata", "durable-session-contract-fixtures.json"),
	)
}

func startAPIAwaitingApprovalSession(t *testing.T, service *factorysessionexecution.FakeService) {
	t.Helper()
	_, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-awaiting-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/approval-gate.yaml",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync awaiting approval: %v", err)
	}
}

func startAPIFailedPartialSession(t *testing.T, service *factorysessionexecution.FakeService) {
	t.Helper()
	_, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-failed-partial-001",
		Source:    factorysessionexecution.Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync failed partial: %v", err)
	}
}

func postFactorySessionApprove(
	t *testing.T,
	serverURL, sessionID string,
	request *factoryapi.FactorySessionApproveRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, int) {
	t.Helper()
	resp, err := postFactorySessionApproveRaw(t, serverURL, sessionID, request)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/approve: %v", sessionID, err)
	}
	defer resp.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode approve response: %v", err)
	}
	return response, resp.StatusCode
}

func postFactorySessionApproveExpectError(
	t *testing.T,
	serverURL, sessionID string,
	request *factoryapi.FactorySessionApproveRequest,
) (int, factoryapi.ErrorResponse) {
	t.Helper()
	var body []byte
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		body = encoded
	}
	url := serverURL + "/factory-sessions/" + sessionID + "/approve"
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return resp.StatusCode, errResp
}

func postFactorySessionApproveRaw(
	t *testing.T,
	serverURL, sessionID string,
	request *factoryapi.FactorySessionApproveRequest,
) (*http.Response, error) {
	t.Helper()
	var body []byte
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		body = encoded
	}
	url := serverURL + "/factory-sessions/" + sessionID + "/approve"
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 500 {
		var errResp factoryapi.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, errors.New(errResp.Message)
	}
	return resp, nil
}

func postFactorySessionRetryDispatch(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionRetryDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, int) {
	t.Helper()
	resp, err := postFactorySessionRetryDispatchRaw(t, serverURL, sessionID, request)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/retry-dispatch: %v", sessionID, err)
	}
	defer resp.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode retry-dispatch response: %v", err)
	}
	return response, resp.StatusCode
}

func postFactorySessionRetryDispatchExpectError(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionRetryDispatchRequest,
) (int, factoryapi.ErrorResponse) {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	url := serverURL + "/factory-sessions/" + sessionID + "/retry-dispatch"
	resp, err := http.Post(url, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return resp.StatusCode, errResp
}

func postFactorySessionRetryDispatchRaw(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionRetryDispatchRequest,
) (*http.Response, error) {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	url := serverURL + "/factory-sessions/" + sessionID + "/retry-dispatch"
	resp, err := http.Post(url, "application/json", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 500 {
		var errResp factoryapi.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, errors.New(errResp.Message)
	}
	return resp, nil
}

func drainAPILifecycleRuntimeSessions(t *testing.T, service factorysessionexecution.Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		list, err := service.ListSessions(context.Background(), factorysessionexecution.ListSessionsRequest{
			Scope: factorysessionexecution.SessionListScopeAll,
		})
		if err != nil {
			return
		}
		pending := false
		for _, session := range list.DurableSessions {
			if factorysessionexecution.IsTerminalLifecycleStatus(session.Status) {
				continue
			}
			pending = true
			_, _ = service.Terminate(context.Background(), session.SessionID, factorysessionexecution.ControlRequest{
				Reason: "test cleanup",
			})
		}
		if !pending {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func removeAPILifecycleProjectState(t *testing.T, projectRoot string) {
	t.Helper()
	const (
		cleanupTimeout      = 5 * time.Second
		cleanupPollInterval = 25 * time.Millisecond
		cleanupQuietPeriod  = 250 * time.Millisecond
	)
	runtimeStateRoot := filepath.Join(projectRoot, ".you-agent-factory")
	deadline := time.Now().Add(cleanupTimeout)
	var absentSince time.Time
	for time.Now().Before(deadline) {
		_, statErr := os.Stat(runtimeStateRoot)
		if statErr == nil {
			// Terminate can publish its terminal projection before the async
			// runner completes its final persistence write. Reset the quiet
			// period whenever that late write recreates runtime state.
			absentSince = time.Time{}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			absentSince = time.Time{}
		}

		if err := os.RemoveAll(runtimeStateRoot); err != nil {
			absentSince = time.Time{}
			time.Sleep(cleanupPollInterval)
			continue
		}
		if absentSince.IsZero() {
			absentSince = time.Now()
		} else if time.Since(absentSince) >= cleanupQuietPeriod {
			return
		}
		time.Sleep(cleanupPollInterval)
	}
	t.Errorf("runtime state directory %q did not remain removed during cleanup", runtimeStateRoot)
}
