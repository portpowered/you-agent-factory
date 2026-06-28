package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
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
	if errResp.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
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

func TestPauseFactorySession_LiveSessionReturnsTypedLifecycleControl(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		State:          interfaces.FactoryStateRunning,
		FactorySession: factoryapi.FactorySession{Id: "live-session-001"},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, "live-session-001", "pause", nil)
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

func TestResumeFactorySession_LiveSessionReturnsTypedLifecycleControl(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		State:          interfaces.FactoryStatePaused,
		FactorySession: factoryapi.FactorySession{Id: "live-session-001"},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, "live-session-001", "resume", nil)
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

func TestPauseFactorySession_MissingLiveSessionReturnsNotFound(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		GetFactorySessionErr: fmt.Errorf("%w: live-session-001", apisurface.ErrFactorySessionNotFound),
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	status, errResp := postFactorySessionLifecycleControlExpectError(
		t,
		server.URL,
		"live-session-001",
		"pause",
		nil,
	)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestApproveFactorySession_RuntimeBackedRunningSessionReturnsTypedInvalidState(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionApprove(t, server.URL, started.SessionID, nil)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindApprove {
		t.Fatalf("operation = %q, want APPROVE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
		t.Fatalf("outcome = %q, want INVALID_STATE", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestApproveFactorySession_FixtureBackedAwaitingApprovalReturnsTypedLifecycleControl(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	startAPIAwaitingApprovalSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionApprove(t, server.URL, "dur-sess-js-awaiting-001", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindApprove {
		t.Fatalf("operation = %q, want APPROVE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
	if response.Links == nil || response.Links.Results == nil || *response.Links.Results == "" {
		t.Fatalf("links = %#v, want results inspection link", response.Links)
	}
}

func TestRetryFactorySessionDispatch_FixtureBackedFailedSessionReturnsTypedLifecycleControl(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	startAPIFailedPartialSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionRetryDispatch(t, server.URL, "dur-sess-js-failed-partial-001", factoryapi.FactorySessionRetryDispatchRequest{
		DispatchId: "disp-js-fail-002",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindRetryDispatch {
		t.Fatalf("operation = %q, want RETRY_DISPATCH", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
	if response.RetryDispatchId == nil || *response.RetryDispatchId != "disp-js-fail-002" {
		t.Fatalf("retryDispatchId = %#v, want disp-js-fail-002", response.RetryDispatchId)
	}
}

func TestRetryFactorySessionDispatch_TerminalSessionReturnsTypedConflict(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	_, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-petri-success-001",
		Source:    factorysessionexecution.Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartSync terminal session: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionRetryDispatch(t, server.URL, "dur-sess-petri-success-001", factoryapi.FactorySessionRetryDispatchRequest{
		DispatchId: "disp-petri-success-001",
	})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", response.Outcome)
	}
}

func TestApproveFactorySession_NonDurableSessionPreservesLiveStub(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	status, errResp := postFactorySessionApproveExpectError(t, server.URL, "live-session-001", nil)
	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", status)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("code = %q, want INTERNAL_ERROR", errResp.Code)
	}
}

func TestRetryFactorySessionDispatch_NonDurableSessionPreservesLiveStub(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	status, errResp := postFactorySessionRetryDispatchExpectError(
		t,
		server.URL,
		"live-session-001",
		factoryapi.FactorySessionRetryDispatchRequest{DispatchId: "disp-live-001"},
	)
	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", status)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("code = %q, want INTERNAL_ERROR", errResp.Code)
	}
}

func TestPauseFactorySession_IdempotentRequestIdReplayReturnsSameOutcome(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	row := startAPIRunningSessionForControl(t, service)
	serverURL := serverURLForLifecycle(t, service)

	requestID := "ctrl-api-lifecycle-replay-001"
	body := &factoryapi.FactorySessionLifecycleControlRequest{RequestId: &requestID}

	first, status := postFactorySessionLifecycleControl(t, serverURL, row.SessionID, "pause", body)
	if status != http.StatusOK {
		t.Fatalf("first status = %d, want 200", status)
	}
	second, status := postFactorySessionLifecycleControl(t, serverURL, row.SessionID, "pause", body)
	if status != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", status)
	}
	if first.Outcome != second.Outcome || first.Status != second.Status || first.Operation != second.Operation {
		t.Fatalf("replay drift: first=%#v second=%#v", first, second)
	}
}

func TestPauseFactorySession_ConflictingRequestIdReturnsTypedConflictWithoutMutation(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	row := startAPIRunningSessionForControl(t, service)
	serverURL := serverURLForLifecycle(t, service)

	requestID := "ctrl-api-lifecycle-conflict-001"
	if _, status := postFactorySessionLifecycleControl(t, serverURL, row.SessionID, "pause", &factoryapi.FactorySessionLifecycleControlRequest{
		RequestId: &requestID,
	}); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, serverURL, row.SessionID, "resume", &factoryapi.FactorySessionLifecycleControlRequest{
		RequestId: &requestID,
	})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeConflict {
		t.Fatalf("outcome = %q, want CONFLICT", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED unchanged", response.Status)
	}

	read, err := service.GetSession(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("session status = %q, want PAUSED unchanged", read.Status)
	}
}

func TestRetryFactorySessionDispatch_MissingDispatchIdReturnsBadRequest(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	startAPIRunningSessionForControl(t, service)

	status, errResp := postFactorySessionRetryDispatchExpectError(
		t,
		serverURLForLifecycle(t, service),
		"dur-sess-js-run-n-001",
		factoryapi.FactorySessionRetryDispatchRequest{},
	)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}
}

func TestRetryFactorySessionDispatch_MissingDispatchReturnsNotFound(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	status, errResp := postFactorySessionRetryDispatchExpectError(
		t,
		serverURLForLifecycle(t, service),
		started.SessionID,
		factoryapi.FactorySessionRetryDispatchRequest{DispatchId: "disp-missing-001"},
	)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
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
	return factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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

func TestRealBackendFactorySessionRoutes_LifecycleControlsAreImplemented(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "busy-loop.workflow.js", "busy-loop")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-api-runtime-lifecycle-slice-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	sessionPath := "/factory-sessions/" + started.SessionId

	resp, err := http.Post(server.URL+sessionPath+"/cancel", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST %s/cancel: %v", sessionPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST %s/cancel status = %d, want 202: %s", sessionPath, resp.StatusCode, readBody(t, resp))
	}
	var control factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&control); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	if control.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("operation = %q, want CANCEL", control.Operation)
	}
	if control.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", control.Outcome)
	}
}
