package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
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
	waitForDurableSessionStatus(t, server.URL, started.SessionID, factoryapi.FactorySessionDurableLifecycleStatusCanceled)
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
	if response.Links == nil || response.Links.Session == nil || *response.Links.Session == "" {
		t.Fatalf("links = %#v, want session inspection link", response.Links)
	}
}

func TestResumeFactorySession_RuntimeBackedRunningSessionReturnsTypedNoOp(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	if _, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "pause", nil); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}
	if _, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "resume", nil); status != http.StatusOK {
		t.Fatalf("first resume status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "resume", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}

	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING unchanged", read.Status)
	}
}

func TestPauseFactorySession_RuntimeBackedPausedSessionReturnsTypedNoOp(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	if _, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "pause", nil); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "pause", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}
}

func TestPauseFactorySession_RuntimeBackedIdempotentRequestIdReplayReturnsSameOutcome(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)
	serverURL := serverURLForLifecycle(t, service)

	requestID := "ctrl-api-runtime-lifecycle-replay-001"
	body := &factoryapi.FactorySessionLifecycleControlRequest{RequestId: &requestID}

	first, status := postFactorySessionLifecycleControl(t, serverURL, started.SessionID, "pause", body)
	if status != http.StatusOK {
		t.Fatalf("first status = %d, want 200", status)
	}
	second, status := postFactorySessionLifecycleControl(t, serverURL, started.SessionID, "pause", body)
	if status != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", status)
	}
	if first.Outcome != second.Outcome || first.Status != second.Status || first.Operation != second.Operation {
		t.Fatalf("replay drift: first=%#v second=%#v", first, second)
	}
}

func TestPauseFactorySession_RuntimeBackedConflictingRequestIdReturnsTypedConflict(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)
	serverURL := serverURLForLifecycle(t, service)

	requestID := "ctrl-api-runtime-lifecycle-conflict-001"
	if _, status := postFactorySessionLifecycleControl(t, serverURL, started.SessionID, "pause", &factoryapi.FactorySessionLifecycleControlRequest{
		RequestId: &requestID,
	}); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, serverURL, started.SessionID, "resume", &factoryapi.FactorySessionLifecycleControlRequest{
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

	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("session status = %q, want PAUSED unchanged", read.Status)
	}
}

func TestCancelFactorySession_RuntimeBackedTerminalSessionReturnsTypedConflict(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "simple-final.workflow.js", "simple-final")
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-cancel-terminal-001",
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

	response, status := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "cancel", nil)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", response.Outcome)
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

func TestRetryFactorySessionDispatch_RuntimeBackedFailedSessionReturnsTypedLifecycleControl(t *testing.T) {
	service := newAPILifecycleFailingChildRuntimeService(t)
	sessionID, dispatchID := startRuntimeBackedFailedSessionWithDispatch(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	beforeRead := getDurableFactorySession(t, server.URL, sessionID)
	beforeDispatchList := getDurableDispatchList(t, server.URL, sessionID)
	beforeDispatchDetail := getDurableDispatchDetail(t, server.URL, sessionID, dispatchID)

	response, status := postFactorySessionRetryDispatch(t, server.URL, sessionID, factoryapi.FactorySessionRetryDispatchRequest{
		DispatchId: dispatchID,
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
	if response.RetryDispatchId == nil || *response.RetryDispatchId != dispatchID {
		t.Fatalf("retryDispatchId = %#v, want %q", response.RetryDispatchId, dispatchID)
	}
	assertLifecycleControlPreservesInspectionLinks(t, sessionID, response.Links)

	afterRead := getDurableFactorySession(t, server.URL, sessionID)
	if afterRead.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session status after retry = %q, want RUNNING", afterRead.Status)
	}
	assertDurableSessionInspectionLinks(t, sessionID, afterRead.Links)

	afterDispatchList := getDurableDispatchList(t, server.URL, sessionID)
	if len(afterDispatchList.Dispatches) < len(beforeDispatchList.Dispatches) {
		t.Fatalf("dispatch history lost: before=%d after=%d", len(beforeDispatchList.Dispatches), len(afterDispatchList.Dispatches))
	}
	afterDispatchDetail := getDurableDispatchDetail(t, server.URL, sessionID, dispatchID)
	if afterDispatchDetail.Status != factoryapi.FactoryDispatchStatusQUEUED {
		t.Fatalf("dispatch status after retry = %q, want QUEUED", afterDispatchDetail.Status)
	}
	beforeAttempt := int32(0)
	if beforeDispatchDetail.Attempt != nil {
		beforeAttempt = *beforeDispatchDetail.Attempt
	}
	if afterDispatchDetail.Attempt == nil || *afterDispatchDetail.Attempt <= beforeAttempt {
		t.Fatalf("dispatch attempt after retry = %#v, want > %d", afterDispatchDetail.Attempt, beforeAttempt)
	}
	getDurableFactorySessionResult(t, server.URL, sessionID, "")
	getDurableFactorySessionEvents(t, server.URL, sessionID, "")

	if beforeRead.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("pre-retry session status = %q, want FAILED", beforeRead.Status)
	}
}

func TestRetryFactorySessionDispatch_RuntimeBackedTerminalSessionReturnsTypedConflict(t *testing.T) {
	projectRoot := setupAPILifecycleWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-lifecycle-retry-terminal-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "retry-terminal",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", read.Status)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionRetryDispatch(t, server.URL, completed.SessionID, factoryapi.FactorySessionRetryDispatchRequest{
		DispatchId: "dispatch-1",
	})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", response.Outcome)
	}
}

func TestRealBackendFactorySessionRoutes_LifecycleControlsAreImplemented(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "busy-loop.workflow.js", "busy-loop")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
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

func TestFakeChildDurableSessionReads_APIPreservesShippedTransportSemantics(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-fake-child-transport-regression-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	before := captureDurableSessionInspectionSnapshot(t, server.URL, completed.SessionID)
	assertAPIFakeChildInspectionSnapshot(t, before)

	_, pauseStatus := postFactorySessionLifecycleControl(t, server.URL, completed.SessionID, "pause", nil)
	if pauseStatus != http.StatusConflict {
		t.Fatalf("pause on terminal session status = %d, want 409", pauseStatus)
	}

	after := captureDurableSessionInspectionSnapshot(t, server.URL, completed.SessionID)
	assertDurableSessionReadUnchanged(t, before.read, after.read)
	assertDurableSessionResultUnchanged(t, before.result, after.result)
	assertDispatchListUnchanged(t, before.dispatches, after.dispatches)
	assertArtifactListUnchanged(t, before.artifacts, after.artifacts)
	assertLifecycleEventsNonDecreasing(t, before.events, after.events)
	assertPostControlEventsAlignWithStatus(t, completed.SessionID, after.events, after.read.Status)

	dispatchDetail := getDurableDispatchDetail(t, server.URL, completed.SessionID, "dispatch-1")
	assertAPIFakeChildDispatchDetail(t, dispatchDetail)
	assertAPIInspectionResponsesExcludeLiveProviderMarkers(t, before, dispatchDetail)
}

func TestSimpleFinalDurableSessionReads_APIPreservesFinalResultWithoutChildDispatches(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-simple-final-transport-regression-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "api",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	snapshot := captureDurableSessionInspectionSnapshot(t, server.URL, completed.SessionID)
	if snapshot.read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", snapshot.read.Status)
	}
	if snapshot.result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", snapshot.result.ResultStatus)
	}
	if len(snapshot.dispatches.Dispatches) != 0 {
		t.Fatalf("dispatch list = %#v, want empty for simple-final", snapshot.dispatches.Dispatches)
	}
	if len(snapshot.artifacts.Artifacts) != 0 {
		t.Fatalf("artifact list = %#v, want empty for simple-final", snapshot.artifacts.Artifacts)
	}
	assertPostControlEventsAlignWithStatus(t, completed.SessionID, snapshot.events, snapshot.read.Status)
	assertAPIInspectionResponsesExcludeLiveProviderMarkers(t, snapshot, factoryapi.FactoryDispatch{})
}

func assertAPIFakeChildSessionSucceeded(t *testing.T, snapshot durableSessionInspectionSnapshot) {
	t.Helper()
	if snapshot.read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", snapshot.read.Status)
	}
	if snapshot.read.Progress == nil ||
		snapshot.read.Progress.TotalDispatches == nil || *snapshot.read.Progress.TotalDispatches != 1 ||
		snapshot.read.Progress.CompletedDispatches == nil || *snapshot.read.Progress.CompletedDispatches != 1 {
		t.Fatalf("session progress = %#v, want one completed dispatch", snapshot.read.Progress)
	}
	if snapshot.result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", snapshot.result.ResultStatus)
	}
}

func assertAPIFakeChildDispatchArtifacts(t *testing.T, snapshot durableSessionInspectionSnapshot) {
	t.Helper()
	if len(snapshot.dispatches.Dispatches) != 1 {
		t.Fatalf("dispatch list = %#v, want one dispatch", snapshot.dispatches.Dispatches)
	}
	dispatchSummary := snapshot.dispatches.Dispatches[0]
	if dispatchSummary.Javascript == nil || dispatchSummary.Javascript.ExecutionMode == nil ||
		*dispatchSummary.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeFake {
		t.Fatalf("dispatch executionMode = %#v, want fake", dispatchSummary.Javascript)
	}
	if dispatchSummary.ProviderSessionRefs == nil || len(*dispatchSummary.ProviderSessionRefs) != 1 ||
		(*dispatchSummary.ProviderSessionRefs)[0].Id != "fake-provider-session-1" {
		t.Fatalf("providerSessionRefs = %#v, want fake-provider-session-1", dispatchSummary.ProviderSessionRefs)
	}
	if len(snapshot.artifacts.Artifacts) != 1 {
		t.Fatalf("artifact list = %#v, want one artifact", snapshot.artifacts.Artifacts)
	}
	if snapshot.artifacts.Artifacts[0].DispatchId == nil || *snapshot.artifacts.Artifacts[0].DispatchId != "dispatch-1" {
		t.Fatalf("artifact dispatchId = %#v, want dispatch-1", snapshot.artifacts.Artifacts[0].DispatchId)
	}
}

func assertAPIFakeChildInspectionSnapshot(
	t *testing.T,
	snapshot durableSessionInspectionSnapshot,
) {
	t.Helper()
	assertAPIFakeChildSessionSucceeded(t, snapshot)
	assertAPIFakeChildDispatchArtifacts(t, snapshot)
}

func assertAPIFakeChildDispatchDetail(t *testing.T, dispatchDetail factoryapi.FactoryDispatch) {
	t.Helper()
	if dispatchDetail.Javascript == nil || dispatchDetail.Javascript.ExecutionMode == nil ||
		*dispatchDetail.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeFake {
		t.Fatalf("dispatch detail executionMode = %#v, want fake", dispatchDetail.Javascript)
	}
	if dispatchDetail.Provider == nil || *dispatchDetail.Provider != "fake" {
		t.Fatalf("dispatch provider = %#v, want fake", dispatchDetail.Provider)
	}
}

func assertAPIInspectionResponsesExcludeLiveProviderMarkers(
	t *testing.T,
	snapshot durableSessionInspectionSnapshot,
	dispatchDetail factoryapi.FactoryDispatch,
) {
	t.Helper()
	encoded, err := json.Marshal(struct {
		Read       factoryapi.FactorySessionDurableReadModel
		Result     factoryapi.FactorySessionResult
		Dispatches factoryapi.ListFactorySessionDispatchesResponse
		Artifacts  factoryapi.ListFactorySessionArtifactsResponse
		Events     []factoryapi.FactoryEvent
		Dispatch   factoryapi.FactoryDispatch
	}{
		Read:       snapshot.read,
		Result:     snapshot.result,
		Dispatches: snapshot.dispatches,
		Artifacts:  snapshot.artifacts,
		Events:     snapshot.events,
		Dispatch:   dispatchDetail,
	})
	if err != nil {
		t.Fatalf("marshal inspection snapshot: %v", err)
	}
	responseText := string(encoded)
	if strings.Contains(responseText, "live-provider-session-1") {
		t.Fatalf("fake-child inspection leaked live-provider session ref:\n%s", responseText)
	}
	for _, term := range fixtures.ForbiddenFixtureVocabularyTerms() {
		if strings.Contains(responseText, term) {
			t.Fatalf("inspection response contained forbidden vocabulary %q:\n%s", term, responseText)
		}
	}
}
