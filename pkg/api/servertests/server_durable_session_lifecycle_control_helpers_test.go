package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

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
	return factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
}

type apiLifecycleFailingChildProvider struct{}

func (apiLifecycleFailingChildProvider) Infer(
	_ context.Context,
	_ interfaces.ProviderInferenceRequest,
) (interfaces.InferenceResponse, error) {
	return interfaces.InferenceResponse{}, workerprovider.NewProviderError(
		interfaces.WorkFailureTypePermanentBadRequest,
		"simulated live child error",
		nil,
	)
}

func newAPILifecycleFailingChildRuntimeService(t *testing.T) factorysessionexecution.Service {
	t.Helper()
	projectRoot := setupAPILifecycleWorkflowFixture(t, "agent-run-live-child-failure.workflow.js", "agent-run-live-child-failure")
	return factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		Provider:          apiLifecycleFailingChildProvider{},
	})
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
	raw, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "javascript", "runtime", "testdata", fixtureName))
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
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(
		filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json"),
	)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
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
