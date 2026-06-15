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
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestCancelFactorySession_RuntimeBackedDurableSessionReturnsTypedLifecycleControl(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "cancel", nil)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("operation = %q, want CANCEL", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling {
		t.Fatalf("status = %q, want CANCELING", response.Status)
	}
	if response.Links == nil || response.Links.Results == nil || *response.Links.Results == "" {
		t.Fatalf("links = %#v, want results inspection link", response.Links)
	}
}

func TestTerminateFactorySession_RuntimeBackedDurableSessionReturnsTypedLifecycleControl(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "terminate", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindTerminate {
		t.Fatalf("operation = %q, want TERMINATE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusTerminated {
		t.Fatalf("status = %q, want TERMINATED", response.Status)
	}
}

func TestCancelFactorySession_MissingDurableSessionReturnsNotFound(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "simple-final.workflow.js", "simple-final")
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	status, errResp := postFactorySessionLifecycleControlExpectError(
		t,
		server.URL,
		"dur-sess-missing-999",
		"cancel",
		nil,
	)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if errResp.Code != factoryapi.NOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestCancelFactorySession_NonDurableSessionPreservesLiveStub(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "simple-final.workflow.js", "simple-final")
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	status, errResp := postFactorySessionLifecycleControlExpectError(
		t,
		server.URL,
		"live-session-001",
		"cancel",
		nil,
	)
	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", status)
	}
	if errResp.Code != factoryapi.INTERNALERROR {
		t.Fatalf("code = %q, want INTERNAL_ERROR", errResp.Code)
	}
}

func TestPauseFactorySession_RuntimeBackedDurableSessionReturnsTypedLifecycleControl(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "pause", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}
}

func TestResumeFactorySession_RuntimeBackedDurableSessionReturnsTypedLifecycleControl(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	if _, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "pause", nil); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "resume", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestPauseFactorySession_TerminalSessionReturnsTypedConflict(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "simple-final.workflow.js", "simple-final")
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-pause-terminal-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		read, readErr := service.GetSession(context.Background(), started.SessionID)
		if readErr != nil {
			t.Fatalf("GetSession: %v", readErr)
		}
		if read.Status == factorysessionexecution.LifecycleStatusSucceeded {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "pause", nil)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", response.Outcome)
	}
}

func TestPauseFactorySession_NonDurableSessionPreservesLiveStub(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "simple-final.workflow.js", "simple-final")
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	status, errResp := postFactorySessionLifecycleControlExpectError(
		t,
		server.URL,
		"live-session-001",
		"pause",
		nil,
	)
	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", status)
	}
	if errResp.Code != factoryapi.INTERNALERROR {
		t.Fatalf("code = %q, want INTERNAL_ERROR", errResp.Code)
	}
}

func newAPILifecycleRuntimeService(t *testing.T, fixtureName, workflowName string) *factorysessionexecution.RuntimeService {
	t.Helper()
	projectRoot := setupAPILifecycleWorkflowFixture(t, fixtureName, workflowName)
	return factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
}

func startRuntimeBackedDurableSession(
	t *testing.T,
	service *factorysessionexecution.RuntimeService,
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
