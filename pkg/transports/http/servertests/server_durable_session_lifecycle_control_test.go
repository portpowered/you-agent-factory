package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestCancelFactorySession_RuntimeBackedDurableSessionReturnsTypedLifecycleControl(t *testing.T) {
	const sessionID = "dur-sess-cancel-001"
	service := apiExecutionScript{
		cancel: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "CANCEL", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusCanceling), nil
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			read.Status = factorysessionexecution.LifecycleStatus("CANCELED")
			return read, nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "cancel", nil)
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
	waitForDurableSessionStatus(t, server.URL, sessionID, factoryapi.FactorySessionDurableLifecycleStatusCanceled)
}

func TestTerminateFactorySession_RuntimeBackedDurableSessionReturnsTypedLifecycleControl(t *testing.T) {
	const sessionID = "dur-sess-terminate-001"
	service := apiExecutionScript{
		terminate: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "TERMINATE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatus("TERMINATED")), nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "terminate", nil)
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
	service := apiExecutionScript{
		cancel: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{}, factorysessionexecution.ErrDurableSessionNotFound
		},
	}
	srv := newDurableAPITestServer(service)
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
	service := apiExecutionScript{}
	srv := newDurableAPITestServer(service)
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
	const sessionID = "dur-sess-pause-001"
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusPaused), nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil)
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
	const sessionID = "dur-sess-resume-001"
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusPaused), nil
		},
		resume: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "RESUME", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusRunning), nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	if _, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "resume", nil)
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
	const sessionID = "dur-sess-resume-noop-001"
	resumeCalls := 0
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusPaused), nil
		},
		resume: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			resumeCalls++
			outcome := factorysessionexecution.LifecycleControlOutcomeAccepted
			if resumeCalls == 2 {
				outcome = factorysessionexecution.LifecycleControlOutcomeNoOp
			}
			return apiControlResult(sessionID, "RESUME", outcome, factorysessionexecution.LifecycleStatusRunning), nil
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			return runningAPIReadResult(sessionID), nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	if _, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}
	if _, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "resume", nil); status != http.StatusOK {
		t.Fatalf("first resume status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "resume", nil)
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

	read, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING unchanged", read.Status)
	}
}

func TestPauseFactorySession_RuntimeBackedPausedSessionReturnsTypedNoOp(t *testing.T) {
	const sessionID = "dur-sess-pause-noop-001"
	pauseCalls := 0
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			pauseCalls++
			outcome := factorysessionexecution.LifecycleControlOutcomeAccepted
			if pauseCalls == 2 {
				outcome = factorysessionexecution.LifecycleControlOutcomeNoOp
			}
			return apiControlResult(sessionID, "PAUSE", outcome, factorysessionexecution.LifecycleStatusPaused), nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	if _, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil)
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
	const sessionID = "dur-sess-pause-replay-001"
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusPaused), nil
		},
	}
	serverURL := serverURLForLifecycle(t, service)

	requestID := "ctrl-api-runtime-lifecycle-replay-001"
	body := &factoryapi.FactorySessionLifecycleControlRequest{RequestId: &requestID}

	first, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", body)
	if status != http.StatusOK {
		t.Fatalf("first status = %d, want 200", status)
	}
	second, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", body)
	if status != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", status)
	}
	if first.Outcome != second.Outcome || first.Status != second.Status || first.Operation != second.Operation {
		t.Fatalf("replay drift: first=%#v second=%#v", first, second)
	}
}

func TestPauseFactorySession_RuntimeBackedConflictingRequestIdReturnsTypedConflict(t *testing.T) {
	const sessionID = "dur-sess-pause-conflict-001"
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusPaused), nil
		},
		resume: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("RESUME", factorysessionexecution.LifecycleControlOutcome("CONFLICT"), factorysessionexecution.LifecycleStatusPaused)
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			read.Status = factorysessionexecution.LifecycleStatusPaused
			return read, nil
		},
	}
	serverURL := serverURLForLifecycle(t, service)

	requestID := "ctrl-api-runtime-lifecycle-conflict-001"
	if _, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", &factoryapi.FactorySessionLifecycleControlRequest{
		RequestId: &requestID,
	}); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "resume", &factoryapi.FactorySessionLifecycleControlRequest{
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

	read, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("session status = %q, want PAUSED unchanged", read.Status)
	}
}

func TestCancelFactorySession_RuntimeBackedTerminalSessionReturnsTypedConflict(t *testing.T) {
	const sessionID = "dur-sess-cancel-terminal-001"
	service := apiExecutionScript{
		cancel: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("CANCEL", factorysessionexecution.LifecycleControlOutcomeTerminalSession, factorysessionexecution.LifecycleStatusSucceeded)
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "cancel", nil)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", response.Outcome)
	}
}

func TestPauseFactorySession_TerminalSessionReturnsTypedConflict(t *testing.T) {
	const sessionID = "dur-sess-pause-terminal-001"
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("PAUSE", factorysessionexecution.LifecycleControlOutcomeTerminalSession, factorysessionexecution.LifecycleStatusSucceeded)
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", response.Outcome)
	}
}

func TestPauseFactorySession_LiveSessionReturnsTypedLifecycleControl(t *testing.T) {
	const sessionID = "live-session-001"
	srv := newDurableAndLiveAPITestServer(nil, apiLiveSessionScript{
		pause: func(
			_ context.Context,
			gotSessionID string,
			_ factorysessionexecution.ControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			if gotSessionID != sessionID {
				t.Fatalf("pause session ID = %q, want %q", gotSessionID, sessionID)
			}
			return factorysessionmapping.LifecycleControlResponseToAPI(apiControlResult(
				sessionID,
				factorysessionexecution.LifecycleControlPause,
				factorysessionexecution.LifecycleControlOutcomeAccepted,
				factorysessionexecution.LifecycleStatusPaused,
			)), nil
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil)
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
	const sessionID = "live-session-001"
	srv := newDurableAndLiveAPITestServer(nil, apiLiveSessionScript{
		resume: func(
			_ context.Context,
			gotSessionID string,
			_ factorysessionexecution.ControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			if gotSessionID != sessionID {
				t.Fatalf("resume session ID = %q, want %q", gotSessionID, sessionID)
			}
			return factorysessionmapping.LifecycleControlResponseToAPI(apiControlResult(
				sessionID,
				factorysessionexecution.LifecycleControlResume,
				factorysessionexecution.LifecycleControlOutcomeAccepted,
				factorysessionexecution.LifecycleStatusRunning,
			)), nil
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionLifecycleControl(t, server.URL, sessionID, "resume", nil)
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
	srv := newDurableAndLiveAPITestServer(nil, apiLiveSessionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("%w: live-session-001", apisurface.ErrFactorySessionNotFound)
		},
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
	const sessionID = "dur-sess-approve-invalid-001"
	service := apiExecutionScript{
		approve: func(context.Context, string, factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("APPROVE", factorysessionexecution.LifecycleControlOutcomeInvalidState, factorysessionexecution.LifecycleStatusRunning)
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionApprove(t, server.URL, sessionID, nil)
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
	const sessionID = "dur-sess-js-awaiting-001"
	service := apiExecutionScript{
		approve: func(context.Context, string, factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "APPROVE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusRunning), nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionApprove(t, server.URL, sessionID, nil)
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
	const sessionID = "dur-sess-js-failed-partial-001"
	service := apiExecutionScript{
		retryDispatch: func(context.Context, string, factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
			result := apiControlResult(sessionID, "RETRY_DISPATCH", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusRunning)
			result.RetryDispatchID = "disp-js-fail-002"
			return result, nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionRetryDispatch(t, server.URL, sessionID, factoryapi.FactorySessionRetryDispatchRequest{
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
	const sessionID = "dur-sess-petri-success-001"
	service := apiExecutionScript{
		retryDispatch: func(context.Context, string, factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("RETRY_DISPATCH", factorysessionexecution.LifecycleControlOutcomeTerminalSession, factorysessionexecution.LifecycleStatusSucceeded)
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionRetryDispatch(t, server.URL, sessionID, factoryapi.FactorySessionRetryDispatchRequest{
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
	service := apiExecutionScript{}
	srv := newDurableAPITestServer(service)
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
	service := apiExecutionScript{}
	srv := newDurableAPITestServer(service)
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
	const sessionID = "dur-sess-js-run-n-001"
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusPaused), nil
		},
	}
	serverURL := serverURLForLifecycle(t, service)

	requestID := "ctrl-api-lifecycle-replay-001"
	body := &factoryapi.FactorySessionLifecycleControlRequest{RequestId: &requestID}

	first, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", body)
	if status != http.StatusOK {
		t.Fatalf("first status = %d, want 200", status)
	}
	second, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", body)
	if status != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", status)
	}
	if first.Outcome != second.Outcome || first.Status != second.Status || first.Operation != second.Operation {
		t.Fatalf("replay drift: first=%#v second=%#v", first, second)
	}
}

func TestPauseFactorySession_ConflictingRequestIdReturnsTypedConflictWithoutMutation(t *testing.T) {
	const sessionID = "dur-sess-js-run-n-001"
	service := apiExecutionScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusPaused), nil
		},
		resume: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("RESUME", factorysessionexecution.LifecycleControlOutcome("CONFLICT"), factorysessionexecution.LifecycleStatusPaused)
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			read.Status = factorysessionexecution.LifecycleStatusPaused
			return read, nil
		},
	}
	serverURL := serverURLForLifecycle(t, service)

	requestID := "ctrl-api-lifecycle-conflict-001"
	if _, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", &factoryapi.FactorySessionLifecycleControlRequest{
		RequestId: &requestID,
	}); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}

	response, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "resume", &factoryapi.FactorySessionLifecycleControlRequest{
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

	read, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("session status = %q, want PAUSED unchanged", read.Status)
	}
}

func TestRetryFactorySessionDispatch_MissingDispatchIdReturnsBadRequest(t *testing.T) {
	service := apiExecutionScript{}

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
	const sessionID = "dur-sess-retry-missing-001"
	service := apiExecutionScript{
		retryDispatch: func(context.Context, string, factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{}, factorysessionexecution.ErrDispatchNotFound
		},
	}

	status, errResp := postFactorySessionRetryDispatchExpectError(
		t,
		serverURLForLifecycle(t, service),
		sessionID,
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
	const sessionID = interruptSessionID
	const dispatchID = interruptDispatchID
	retried := false
	service := failedProviderScript()
	service.retryDispatch = func(context.Context, string, factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
		retried = true
		result := apiControlResult(sessionID, "RETRY_DISPATCH", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusRunning)
		result.RetryDispatchID = dispatchID
		return result, nil
	}
	originalGetSession := service.getSession
	service.getSession = func(ctx context.Context, id string) (factorysessionexecution.SessionReadResult, error) {
		if !retried {
			return originalGetSession(ctx, id)
		}
		return runningAPIReadResult(sessionID), nil
	}
	originalList := service.listDispatches
	service.listDispatches = func(ctx context.Context, id string) (factorysessionexecution.ListDispatchesResult, error) {
		result, err := originalList(ctx, id)
		if retried {
			result.Dispatches[0].Status = factorysessionexecution.DispatchStatus("QUEUED")
			result.Dispatches[0].Attempt = 2
		}
		return result, err
	}
	originalDetail := service.getDispatch
	service.getDispatch = func(ctx context.Context, id, target string) (factorysessionexecution.DispatchDetail, error) {
		result, err := originalDetail(ctx, id, target)
		if retried {
			result.Status = factorysessionexecution.DispatchStatus("QUEUED")
			result.Attempt = 2
		}
		return result, err
	}

	srv := newDurableAPITestServer(service)
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
	const sessionID = "dur-sess-retry-terminal-001"
	service := apiExecutionScript{
		retryDispatch: func(context.Context, string, factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("RETRY_DISPATCH", factorysessionexecution.LifecycleControlOutcomeTerminalSession, factorysessionexecution.LifecycleStatusSucceeded)
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionRetryDispatch(t, server.URL, sessionID, factoryapi.FactorySessionRetryDispatchRequest{
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
	const sessionID = "dur-sess-route-lifecycle-001"
	service := apiExecutionScript{
		startAsync: func(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.AsyncStartResult, error) {
			return factorysessionexecution.AsyncStartResult{
				SessionID: sessionID,
				Status:    string(factorysessionexecution.LifecycleStatusRunning),
			}, nil
		},
		cancel: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "CANCEL", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusCanceling), nil
		},
	}
	srv := newDurableAPITestServer(service)
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
	const sessionID = interruptSessionID
	service := completedProviderScript("fake", "fake-provider-session-1", "fake")
	service.pause = func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
		return factorysessionexecution.LifecycleControlResult{},
			apiControlError("PAUSE", factorysessionexecution.LifecycleControlOutcomeTerminalSession, factorysessionexecution.LifecycleStatusSucceeded)
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	before := captureDurableSessionInspectionSnapshot(t, server.URL, sessionID)
	assertAPIFakeChildInspectionSnapshot(t, before)

	_, pauseStatus := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil)
	if pauseStatus != http.StatusConflict {
		t.Fatalf("pause on terminal session status = %d, want 409", pauseStatus)
	}

	after := captureDurableSessionInspectionSnapshot(t, server.URL, sessionID)
	assertDurableSessionReadUnchanged(t, before.read, after.read)
	assertDurableSessionResultUnchanged(t, before.result, after.result)
	assertDispatchListUnchanged(t, before.dispatches, after.dispatches)
	assertArtifactListUnchanged(t, before.artifacts, after.artifacts)
	assertLifecycleEventsNonDecreasing(t, before.events, after.events)
	assertPostControlEventsAlignWithStatus(t, sessionID, after.events, after.read.Status)

	dispatchDetail := getDurableDispatchDetail(t, server.URL, sessionID, "dispatch-1")
	assertAPIFakeChildDispatchDetail(t, dispatchDetail)
	assertAPIInspectionResponsesExcludeLiveProviderMarkers(t, before, dispatchDetail)
}

func TestSimpleFinalDurableSessionReads_APIPreservesFinalResultWithoutChildDispatches(t *testing.T) {
	const sessionID = "dur-sess-simple-final-001"
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			return terminalAPIReadResult(sessionID), nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			return finalAPIResult(sessionID), nil
		},
		listDispatches: func(context.Context, string) (factorysessionexecution.ListDispatchesResult, error) {
			return factorysessionexecution.ListDispatchesResult{SessionID: sessionID, Dispatches: []factorysessionexecution.DispatchSummary{}}, nil
		},
		listArtifacts: func(context.Context, string) (factorysessionexecution.ListArtifactsResult, error) {
			return factorysessionexecution.ListArtifactsResult{SessionID: sessionID, Artifacts: []factorysessionexecution.ArtifactSummary{}}, nil
		},
		readEvents: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			return apiTerminalEvents(sessionID), nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	snapshot := captureDurableSessionInspectionSnapshot(t, server.URL, sessionID)
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
	assertPostControlEventsAlignWithStatus(t, sessionID, snapshot.events, snapshot.read.Status)
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
	for _, term := range []string{"DynamicWorkflowRun", "workflow run"} {
		if strings.Contains(responseText, term) {
			t.Fatalf("inspection response contained forbidden vocabulary %q:\n%s", term, responseText)
		}
	}
}
