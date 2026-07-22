// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestStartDurableFactorySessionAsync_RuntimeBackedSimpleFinalReturnsStableSession(t *testing.T) {
	const sessionID = "dur-sess-api-runtime-async-001"
	service := apiExecutionScript{
		startAsync: func(_ context.Context, request factorysessionexecution.StartRequest) (factorysessionexecution.AsyncStartResult, error) {
			if request.RequestID != "req-api-runtime-async-001" {
				t.Fatalf("requestId = %q", request.RequestID)
			}
			return factorysessionexecution.AsyncStartResult{
				SessionID:        sessionID,
				Status:           string(factorysessionexecution.LifecycleStatusRunning),
				OrchestratorKind: "JAVASCRIPT",
				ResolvedSource: factorysessionexecution.ResolvedSource{
					SourceRef: factory.WorkflowSourceProjectClaudeWorkflowsDir + "/simple-final.js",
				},
				Links: apiInspectionLinks(sessionID),
			}, nil
		},
	}
	srv := newDurableAPITestServer(service)
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
	if first.ResolvedSource.SourceRef == nil || *first.ResolvedSource.SourceRef != factory.WorkflowSourceProjectClaudeWorkflowsDir+"/simple-final.js" {
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
	const sessionID = "dur-sess-api-runtime-conflict-001"
	call := 0
	service := apiExecutionScript{
		startAsync: func(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.AsyncStartResult, error) {
			call++
			if call == 1 {
				return factorysessionexecution.AsyncStartResult{
					SessionID: sessionID,
					Status:    string(factorysessionexecution.LifecycleStatusRunning),
				}, nil
			}
			return factorysessionexecution.AsyncStartResult{}, factorysessionexecution.ErrExecutionRequestIDConflict
		},
	}
	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	first := runtimeBackedAsyncStartRequest("req-api-runtime-conflict-001")
	_, err := postDurableAsyncStartRaw(t, server.URL, first)
	if err != nil {
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
	if errResp.Code != factoryapi.ErrorResponseCodeEXECUTIONREQUESTIDCONFLICT {
		t.Fatalf("code = %q, want EXECUTION_REQUEST_ID_CONFLICT", errResp.Code)
	}
}

func TestStartDurableFactorySessionAsync_InvalidSourceDoesNotCreateSession(t *testing.T) {
	const sessionID = "dur-sess-api-runtime-valid-001"
	call := 0
	service := apiExecutionScript{
		startAsync: func(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.AsyncStartResult, error) {
			call++
			if call == 1 {
				return factorysessionexecution.AsyncStartResult{},
					&factorysessionexecution.ExecutionValidationError{Field: "source", Message: "missing workflow"}
			}
			return factorysessionexecution.AsyncStartResult{
				SessionID: sessionID,
				Status:    string(factorysessionexecution.LifecycleStatusRunning),
			}, nil
		},
	}
	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	invalid := runtimeBackedAsyncStartRequest("req-api-runtime-invalid-001")
	invalid.Source.WorkflowName = strPtr("missing-workflow")
	status, errResp := postDurableAsyncStartExpectError(t, server.URL, invalid)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}

	valid := runtimeBackedAsyncStartRequest("req-api-runtime-invalid-002")
	started := postDurableAsyncStart(t, server.URL, valid)
	if started.SessionId == "" {
		t.Fatal("expected valid start to create a session")
	}
}

func TestStartDurableFactorySessionAsync_MissingRequestIDReturnsValidationError(t *testing.T) {
	service := apiExecutionScript{}
	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-invalid-001")
	request.RequestId = ""
	status, errResp := postDurableAsyncStartExpectError(t, server.URL, request)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
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

func strPtr(value string) *string {
	return &value
}

// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestLifecycleControls_PreserveRunningSessionReadParity(t *testing.T) {
	const sessionID = "dur-sess-running-parity-001"
	status := factorysessionexecution.LifecycleStatusRunning
	events := apiTerminalEvents(sessionID)
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			read.Status = status
			return read, nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			return notReadyAPIResult(sessionID, factorysessionexecution.LifecycleStatusRunning), nil
		},
		listSessions: func(context.Context, factorysessionexecution.ListSessionsRequest) (factorysessionexecution.ListSessionsResult, error) {
			return factorysessionexecution.ListSessionsResult{
				Scope: factorysessionexecution.SessionListScopeAll,
				DurableSessions: []factorysessionexecution.DurableSessionListSummary{{
					SessionID: sessionID, Status: status,
				}},
			}, nil
		},
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			status = factorysessionexecution.LifecycleStatusPaused
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, status), nil
		},
		resume: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			status = factorysessionexecution.LifecycleStatusRunning
			return apiControlResult(sessionID, "RESUME", factorysessionexecution.LifecycleControlOutcomeAccepted, status), nil
		},
		listDispatches: func(context.Context, string) (factorysessionexecution.ListDispatchesResult, error) {
			return factorysessionexecution.ListDispatchesResult{SessionID: sessionID, Dispatches: []factorysessionexecution.DispatchSummary{}}, nil
		},
		listArtifacts: func(context.Context, string) (factorysessionexecution.ListArtifactsResult, error) {
			return factorysessionexecution.ListArtifactsResult{SessionID: sessionID, Artifacts: []factorysessionexecution.ArtifactSummary{}}, nil
		},
		readEvents: func(_ context.Context, _ string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			if request.AfterEventID != "" {
				result := events
				result.Events = result.Events[1:]
				return result, nil
			}
			return events, nil
		},
	}

	srv := newDurableAndLiveAPITestServer(service, apiLiveSessionScript{
		list: func(context.Context) (factoryapi.ListFactorySessionsResponse, error) {
			return factoryapi.ListFactorySessionsResponse{}, nil
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	beforeRead := getDurableFactorySession(t, server.URL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, beforeRead.Links)
	beforeResult := getDurableFactorySessionResult(t, server.URL, sessionID, "")
	beforeEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")
	beforeList := getFactorySessionList(t, server.URL, "all")

	pauseResp, pauseStatus := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil)
	if pauseStatus != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", pauseStatus)
	}
	assertLifecycleControlPreservesInspectionLinks(t, sessionID, pauseResp.Links)
	assertReadSurfacesReachableAfterLifecycle(t, server.URL, sessionID)
	currentEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")
	assertEventReconnectStillWorks(t, server.URL, sessionID, currentEvents)

	pausedRead := getDurableFactorySession(t, server.URL, sessionID)
	if pausedRead.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status after pause = %q, want PAUSED", pausedRead.Status)
	}
	assertDurableSessionInspectionLinks(t, sessionID, pausedRead.Links)

	resumeResp, resumeStatus := postFactorySessionLifecycleControl(t, server.URL, sessionID, "resume", nil)
	if resumeStatus != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", resumeStatus)
	}
	assertLifecycleControlPreservesInspectionLinks(t, sessionID, resumeResp.Links)

	afterRead := getDurableFactorySession(t, server.URL, sessionID)
	if afterRead.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status after resume = %q, want RUNNING", afterRead.Status)
	}
	assertDurableSessionInspectionLinks(t, sessionID, afterRead.Links)

	afterResult := getDurableFactorySessionResult(t, server.URL, sessionID, "")
	assertDurableSessionResultUnchanged(t, beforeResult, afterResult)

	afterEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")
	if len(afterEvents) < len(beforeEvents) {
		t.Fatalf("event count after lifecycle = %d, want at least %d", len(afterEvents), len(beforeEvents))
	}

	afterList := getFactorySessionList(t, server.URL, "all")
	if !containsSessionInListResponse(afterList, sessionID) {
		t.Fatalf("all-scope list = %#v, want session %q", afterList, sessionID)
	}
	if len(afterList.Sessions) < len(beforeList.Sessions) {
		t.Fatalf("all-scope session count = %d, want at least %d",
			len(afterList.Sessions), len(beforeList.Sessions))
	}
}

func containsSessionInListResponse(response factoryapi.ListFactorySessionsResponse, sessionID string) bool {
	for _, row := range response.Sessions {
		if row.Id == sessionID {
			return true
		}
	}
	if response.DurableSessions != nil {
		for _, row := range *response.DurableSessions {
			if row.SessionId == sessionID {
				return true
			}
		}
	}
	return false
}

func TestLifecycleControls_PreserveCompletedSessionReadParity(t *testing.T) {
	const sessionID = "dur-sess-completed-parity-001"
	events := apiTerminalEvents(sessionID)
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			return terminalAPIReadResult(sessionID), nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			return finalAPIResult(sessionID), nil
		},
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("PAUSE", factorysessionexecution.LifecycleControlOutcomeTerminalSession, factorysessionexecution.LifecycleStatusSucceeded)
		},
		readEvents: func(_ context.Context, _ string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			if request.AfterEventID != "" {
				result := events
				result.Events = result.Events[1:]
				return result, nil
			}
			return events, nil
		},
	}
	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	beforeRead := getDurableFactorySession(t, server.URL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, beforeRead.Links)
	beforeResult := getDurableFactorySessionResult(t, server.URL, sessionID, "")
	beforeEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")

	_, pauseStatus := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil)
	if pauseStatus != http.StatusConflict {
		t.Fatalf("pause on terminal session status = %d, want 409", pauseStatus)
	}

	afterRead := getDurableFactorySession(t, server.URL, sessionID)
	assertDurableSessionReadUnchanged(t, beforeRead, afterRead)
	assertDurableSessionInspectionLinks(t, sessionID, afterRead.Links)

	afterResult := getDurableFactorySessionResult(t, server.URL, sessionID, "")
	assertDurableSessionResultUnchanged(t, beforeResult, afterResult)

	afterEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("event count = %d, want %d after rejected lifecycle control", len(afterEvents), len(beforeEvents))
	}
	assertEventReconnectStillWorks(t, server.URL, sessionID, beforeEvents)
}

func TestLifecycleControls_PreserveDispatchArtifactReadParity(t *testing.T) {
	const sessionID = interruptSessionID
	service := completedProviderScript("fake", "fake-provider-session-1", "fake")
	service.pause = func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
		return factorysessionexecution.LifecycleControlResult{},
			apiControlError("PAUSE", factorysessionexecution.LifecycleControlOutcomeTerminalSession, factorysessionexecution.LifecycleStatusSucceeded)
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	beforeRead := getDurableFactorySession(t, server.URL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, beforeRead.Links)
	beforeDispatchList := getDurableDispatchList(t, server.URL, sessionID)
	beforeDispatchDetail := getDurableDispatchDetail(t, server.URL, sessionID, "dispatch-1")
	beforeArtifactList := getDurableArtifactList(t, server.URL, sessionID)

	_, pauseStatus := postFactorySessionLifecycleControl(t, server.URL, sessionID, "pause", nil)
	if pauseStatus != http.StatusConflict {
		t.Fatalf("pause on terminal session status = %d, want 409", pauseStatus)
	}

	afterRead := getDurableFactorySession(t, server.URL, sessionID)
	assertDurableSessionReadUnchanged(t, beforeRead, afterRead)
	assertDurableSessionInspectionLinks(t, sessionID, afterRead.Links)

	afterDispatchList := getDurableDispatchList(t, server.URL, sessionID)
	assertDispatchListUnchanged(t, beforeDispatchList, afterDispatchList)
	afterDispatchDetail := getDurableDispatchDetail(t, server.URL, sessionID, "dispatch-1")
	assertDispatchDetailUnchanged(t, beforeDispatchDetail, afterDispatchDetail)
	afterArtifactList := getDurableArtifactList(t, server.URL, sessionID)
	assertArtifactListUnchanged(t, beforeArtifactList, afterArtifactList)
}

func TestLifecycleControls_CancelPreservesReadSurfaces(t *testing.T) {
	const sessionID = "dur-sess-cancel-surfaces-001"
	events := apiTerminalEvents(sessionID)
	service := apiExecutionScript{
		cancel: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "CANCEL", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusCanceling), nil
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			read.Status = factorysessionexecution.LifecycleStatus("CANCELED")
			return read, nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			return notReadyAPIResult(sessionID, factorysessionexecution.LifecycleStatus("CANCELED")), nil
		},
		listSessions: func(context.Context, factorysessionexecution.ListSessionsRequest) (factorysessionexecution.ListSessionsResult, error) {
			return factorysessionexecution.ListSessionsResult{
				Scope:           factorysessionexecution.SessionListScopeAll,
				DurableSessions: []factorysessionexecution.DurableSessionListSummary{{SessionID: sessionID, Status: factorysessionexecution.LifecycleStatus("CANCELED")}},
			}, nil
		},
		readEvents: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			return events, nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	beforeEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")

	_, cancelStatus := postFactorySessionLifecycleControl(t, server.URL, sessionID, "cancel", nil)
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("cancel status = %d, want 202", cancelStatus)
	}

	read := getDurableFactorySession(t, server.URL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, read.Links)
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling &&
		read.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("status after cancel = %q, want CANCELING or CANCELED", read.Status)
	}

	getDurableFactorySessionResult(t, server.URL, sessionID, "")
	getFactorySessionList(t, server.URL, "persisted")

	afterEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")
	if len(afterEvents) < len(beforeEvents) {
		t.Fatalf("event count after cancel = %d, want at least %d", len(afterEvents), len(beforeEvents))
	}
}

func TestLifecycleControls_TerminatePreservesReadSurfaces(t *testing.T) {
	const sessionID = "dur-sess-terminate-surfaces-001"
	events := apiTerminalEvents(sessionID)
	service := apiExecutionScript{
		terminate: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return apiControlResult(sessionID, "TERMINATE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatus("TERMINATED")), nil
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			read.Status = factorysessionexecution.LifecycleStatus("TERMINATED")
			return read, nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			return notReadyAPIResult(sessionID, factorysessionexecution.LifecycleStatus("TERMINATED")), nil
		},
		listSessions: func(context.Context, factorysessionexecution.ListSessionsRequest) (factorysessionexecution.ListSessionsResult, error) {
			return factorysessionexecution.ListSessionsResult{
				Scope:           factorysessionexecution.SessionListScopeAll,
				DurableSessions: []factorysessionexecution.DurableSessionListSummary{{SessionID: sessionID, Status: "TERMINATED"}},
			}, nil
		},
		listDispatches: func(context.Context, string) (factorysessionexecution.ListDispatchesResult, error) {
			return factorysessionexecution.ListDispatchesResult{SessionID: sessionID, Dispatches: []factorysessionexecution.DispatchSummary{}}, nil
		},
		listArtifacts: func(context.Context, string) (factorysessionexecution.ListArtifactsResult, error) {
			return factorysessionexecution.ListArtifactsResult{SessionID: sessionID, Artifacts: []factorysessionexecution.ArtifactSummary{}}, nil
		},
		readEvents: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			return events, nil
		},
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	beforeRead := getDurableFactorySession(t, server.URL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, beforeRead.Links)
	beforeEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")

	terminateResp, terminateStatus := postFactorySessionLifecycleControl(t, server.URL, sessionID, "terminate", nil)
	if terminateStatus != http.StatusOK {
		t.Fatalf("terminate status = %d, want 200", terminateStatus)
	}
	if terminateResp.Status != factoryapi.FactorySessionDurableLifecycleStatusTerminated {
		t.Fatalf("control status = %q, want TERMINATED", terminateResp.Status)
	}
	assertLifecycleControlPreservesInspectionLinks(t, sessionID, terminateResp.Links)
	assertReadSurfacesReachableAfterLifecycle(t, server.URL, sessionID)

	read := getDurableFactorySession(t, server.URL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, read.Links)
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusTerminated &&
		read.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("status after terminate = %q, want TERMINATED or CANCELED", read.Status)
	}
	if read.SessionId != sessionID {
		t.Fatalf("session id after terminate = %q, want %q", read.SessionId, sessionID)
	}

	getDurableFactorySessionResult(t, server.URL, sessionID, "")
	getFactorySessionList(t, server.URL, "persisted")

	afterEvents := getDurableFactorySessionEvents(t, server.URL, sessionID, "")
	if len(afterEvents) < len(beforeEvents) {
		t.Fatalf("event count after terminate = %d, want at least %d", len(afterEvents), len(beforeEvents))
	}
}

func assertDurableSessionInspectionLinks(
	t *testing.T,
	sessionID string,
	links *factoryapi.FactorySessionExecutionLinks,
) {
	t.Helper()
	if links == nil {
		t.Fatal("session read links = nil, want inspection links")
	}
	base := "/factory-sessions/" + sessionID
	assertLinkPresent(t, "session", links.Session, base)
	assertLinkPresent(t, "results", links.Results, base+"/results")
	assertLinkPresent(t, "events", links.Events, base+"/events")
}

func assertLifecycleControlPreservesInspectionLinks(
	t *testing.T,
	sessionID string,
	links *factoryapi.FactorySessionLifecycleControlLinks,
) {
	t.Helper()
	if links == nil {
		t.Fatal("lifecycle control links = nil, want inspection links")
	}
	base := "/factory-sessions/" + sessionID
	assertLinkPresent(t, "results", links.Results, base+"/results")
	assertLinkPresent(t, "events", links.Events, base+"/events")
	assertLinkPresent(t, "dispatches", links.Dispatches, base+"/dispatches")
	assertLinkPresent(t, "artifacts", links.Artifacts, base+"/artifacts")
}

func assertLinkPresent(t *testing.T, label string, link *string, want string) {
	t.Helper()
	if link == nil || *link != want {
		t.Fatalf("%s link = %#v, want %q", label, link, want)
	}
}

func assertReadSurfacesReachableAfterLifecycle(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	getDurableFactorySession(t, serverURL, sessionID)
	getDurableFactorySessionResult(t, serverURL, sessionID, "")
	getDurableFactorySessionEvents(t, serverURL, sessionID, "")
	getDurableDispatchList(t, serverURL, sessionID)
	getDurableArtifactList(t, serverURL, sessionID)
}

func assertEventReconnectStillWorks(
	t *testing.T,
	serverURL, sessionID string,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("expected at least one event for reconnect assertion")
	}
	firstID := events[0].Id
	afterStart := getDurableFactorySessionEvents(
		t,
		serverURL,
		sessionID,
		"after_event_id="+url.QueryEscape(firstID),
	)
	if len(afterStart) != len(events)-1 {
		t.Fatalf("reconnect event count = %d, want %d", len(afterStart), len(events)-1)
	}
}

func waitForDurableSessionStatus(
	t *testing.T,
	serverURL, sessionID string,
	want ...factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		read := getDurableFactorySession(t, serverURL, sessionID)
		for _, status := range want {
			if read.Status == status {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	last := getDurableFactorySession(t, serverURL, sessionID)
	t.Fatalf("session %q did not reach status %#v before timeout (last=%q)", sessionID, want, last.Status)
}

func getDurableDispatchList(t *testing.T, serverURL, sessionID string) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID + "/dispatches")
	if err != nil {
		t.Fatalf("GET dispatches: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dispatch list status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.ListFactorySessionDispatchesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode dispatch list: %v", err)
	}
	return response
}

func getDurableDispatchDetail(t *testing.T, serverURL, sessionID, dispatchID string) factoryapi.FactoryDispatch {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID + "/dispatches/" + dispatchID)
	if err != nil {
		t.Fatalf("GET dispatch detail: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dispatch detail status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.FactoryDispatch
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode dispatch detail: %v", err)
	}
	return response
}

func getDurableArtifactList(t *testing.T, serverURL, sessionID string) factoryapi.ListFactorySessionArtifactsResponse {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID + "/artifacts")
	if err != nil {
		t.Fatalf("GET artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("artifact list status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.ListFactorySessionArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact list: %v", err)
	}
	return response
}

func assertDispatchListUnchanged(
	t *testing.T,
	before, after factoryapi.ListFactorySessionDispatchesResponse,
) {
	t.Helper()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("marshal before dispatch list: %v", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("marshal after dispatch list: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("dispatch list changed: before=%s after=%s", beforeJSON, afterJSON)
	}
}

func assertDispatchDetailUnchanged(t *testing.T, before, after factoryapi.FactoryDispatch) {
	t.Helper()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("marshal before dispatch detail: %v", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("marshal after dispatch detail: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("dispatch detail changed: before=%s after=%s", beforeJSON, afterJSON)
	}
}

func assertArtifactListUnchanged(
	t *testing.T,
	before, after factoryapi.ListFactorySessionArtifactsResponse,
) {
	t.Helper()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("marshal before artifact list: %v", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("marshal after artifact list: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("artifact list changed: before=%s after=%s", beforeJSON, afterJSON)
	}
}

type durableSessionInspectionSnapshot struct {
	read       factoryapi.FactorySessionDurableReadModel
	result     factoryapi.FactorySessionResult
	dispatches factoryapi.ListFactorySessionDispatchesResponse
	artifacts  factoryapi.ListFactorySessionArtifactsResponse
	events     []factoryapi.FactoryEvent
}

func captureDurableSessionInspectionSnapshot(t *testing.T, serverURL, sessionID string) durableSessionInspectionSnapshot {
	t.Helper()
	return durableSessionInspectionSnapshot{
		read:       getDurableFactorySession(t, serverURL, sessionID),
		result:     getDurableFactorySessionResult(t, serverURL, sessionID, ""),
		dispatches: getDurableDispatchList(t, serverURL, sessionID),
		artifacts:  getDurableArtifactList(t, serverURL, sessionID),
		events:     getDurableFactorySessionEvents(t, serverURL, sessionID, ""),
	}
}

func assertLifecycleEventsNonDecreasing(
	t *testing.T,
	before, after []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(after) < len(before) {
		t.Fatalf("event count after lifecycle control = %d, want at least %d", len(after), len(before))
	}
}

func assertFactoryEventTypePresent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantType, sessionID string,
) {
	t.Helper()
	for _, event := range events {
		if string(event.Type) != wantType {
			continue
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("%s event sessionId = %#v, want %q", wantType, event.Context.SessionId, sessionID)
		}
		return
	}
	t.Fatalf("events = %#v, want %s", events, wantType)
}

func assertPostControlEventsAlignWithStatus(
	t *testing.T,
	sessionID string,
	events []factoryapi.FactoryEvent,
	status factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("expected canonical events after lifecycle control")
	}
	switch status {
	case factoryapi.FactorySessionDurableLifecycleStatusPaused:
		if !containsFactoryEventType(events, "SESSION_PAUSED", sessionID) &&
			!containsLifecycleControlEvent(events, sessionID, "PAUSE") {
			t.Fatalf("events = %#v, want SESSION_PAUSED or PAUSE lifecycle control", events)
		}
	case factoryapi.FactorySessionDurableLifecycleStatusRunning:
		if !containsFactoryEventType(events, "SESSION_RESUMED", sessionID) &&
			!containsLifecycleControlEvent(events, sessionID, "RESUME") {
			t.Fatalf("events = %#v, want SESSION_RESUMED or RESUME lifecycle control", events)
		}
	case factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded:
		assertFactoryEventTypePresent(t, events, "SESSION_COMPLETED", sessionID)
	}
	last := events[len(events)-1]
	if last.SchemaVersion != factoryapi.AgentFactoryEventV1 {
		t.Fatalf("schemaVersion = %q, want agent-factory.event.v1", last.SchemaVersion)
	}
	if last.Context.SessionId == nil || *last.Context.SessionId != sessionID {
		t.Fatalf("latest event sessionId = %#v, want %q", last.Context.SessionId, sessionID)
	}
}

func containsFactoryEventType(events []factoryapi.FactoryEvent, wantType, sessionID string) bool {
	for _, event := range events {
		if string(event.Type) != wantType {
			continue
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			continue
		}
		return true
	}
	return false
}

func containsLifecycleControlEvent(events []factoryapi.FactoryEvent, sessionID, wantOperation string) bool {
	for _, event := range events {
		if string(event.Type) != "SESSION_LIFECYCLE_CONTROL" {
			continue
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			continue
		}
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			continue
		}
		var body struct {
			Operation string `json:"operation"`
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			continue
		}
		if body.Operation == wantOperation {
			return true
		}
	}
	return false
}

func assertDurableSessionPartialResultStillInspectable(
	t *testing.T,
	before, after factoryapi.FactorySessionResult,
	expectSessionStatus factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	if after.ResultStatus != before.ResultStatus {
		t.Fatalf("resultStatus changed: before=%q after=%q", before.ResultStatus, after.ResultStatus)
	}
	if after.SessionStatus == nil || *after.SessionStatus != expectSessionStatus {
		t.Fatalf("result sessionStatus = %#v, want %q", after.SessionStatus, expectSessionStatus)
	}
	if after.Availability == nil {
		t.Fatal("result availability = nil, want inspectable partial availability")
	}
}

func TestLifecycleControls_PauseResumePreservesInspectablePartialStateAcrossReadSurfaces(t *testing.T) {
	const sessionID = "dur-sess-partial-pause-resume-001"
	status := factorysessionexecution.LifecycleStatusRunning
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			read.Status = status
			return read, nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			return notReadyAPIResult(sessionID, status), nil
		},
		listDispatches: func(context.Context, string) (factorysessionexecution.ListDispatchesResult, error) {
			return factorysessionexecution.ListDispatchesResult{SessionID: sessionID, Dispatches: []factorysessionexecution.DispatchSummary{}}, nil
		},
		listArtifacts: func(context.Context, string) (factorysessionexecution.ListArtifactsResult, error) {
			return factorysessionexecution.ListArtifactsResult{SessionID: sessionID, Artifacts: []factorysessionexecution.ArtifactSummary{}}, nil
		},
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			status = factorysessionexecution.LifecycleStatusPaused
			return apiControlResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, status), nil
		},
		resume: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			status = factorysessionexecution.LifecycleStatusRunning
			return apiControlResult(sessionID, "RESUME", factorysessionexecution.LifecycleControlOutcomeAccepted, status), nil
		},
		readEvents: func(_ context.Context, _ string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			eventType := "SESSION_RESUMED"
			if status == factorysessionexecution.LifecycleStatusPaused {
				eventType = "SESSION_PAUSED"
			}
			result := factorysessionexecution.EventReadResult{
				SessionID: sessionID,
				Events: []json.RawMessage{
					apiCanonicalEvent("session-started/"+sessionID, "SESSION_STARTED", sessionID, 1),
					apiCanonicalEvent("session-control/"+sessionID, eventType, sessionID, 2),
				},
			}
			if request.AfterEventID != "" {
				result.Events = result.Events[1:]
			}
			return result, nil
		},
	}
	serverURL := durableRoleHTTPServer(t, service)

	before := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, before.read.Links)

	pauseResp, pauseStatus := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", nil)
	if pauseStatus != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", pauseStatus)
	}
	assertLifecycleControlPreservesInspectionLinks(t, sessionID, pauseResp.Links)

	afterPause := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)
	assertDurableSessionPartialResultStillInspectable(
		t,
		before.result,
		afterPause.result,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)
	assertDispatchListUnchanged(t, before.dispatches, afterPause.dispatches)
	assertArtifactListUnchanged(t, before.artifacts, afterPause.artifacts)
	assertLifecycleEventsNonDecreasing(t, before.events, afterPause.events)
	if afterPause.read.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status after pause = %q, want PAUSED", afterPause.read.Status)
	}
	assertPostControlEventsAlignWithStatus(t, sessionID, afterPause.events, afterPause.read.Status)
	assertEventReconnectStillWorks(t, serverURL, sessionID, afterPause.events)

	resumeResp, resumeStatus := postFactorySessionLifecycleControl(t, serverURL, sessionID, "resume", nil)
	if resumeStatus != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", resumeStatus)
	}
	assertLifecycleControlPreservesInspectionLinks(t, sessionID, resumeResp.Links)

	afterResume := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)
	assertDurableSessionPartialResultStillInspectable(
		t,
		before.result,
		afterResume.result,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)
	assertDispatchListUnchanged(t, before.dispatches, afterResume.dispatches)
	assertArtifactListUnchanged(t, before.artifacts, afterResume.artifacts)
	assertLifecycleEventsNonDecreasing(t, afterPause.events, afterResume.events)
	if afterResume.read.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status after resume = %q, want RUNNING", afterResume.read.Status)
	}
	assertPostControlEventsAlignWithStatus(t, sessionID, afterResume.events, afterResume.read.Status)
	assertEventReconnectStillWorks(t, serverURL, sessionID, afterResume.events)
}

func TestLifecycleControls_RetryDispatchPreservesInspectablePartialStateAcrossReadSurfaces(t *testing.T) {
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
	originalRead := service.getSession
	service.getSession = func(ctx context.Context, id string) (factorysessionexecution.SessionReadResult, error) {
		if retried {
			return runningAPIReadResult(sessionID), nil
		}
		return originalRead(ctx, id)
	}

	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	before := captureDurableSessionInspectionSnapshot(t, server.URL, sessionID)
	if before.read.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("pre-retry status = %q, want FAILED", before.read.Status)
	}
	if len(before.dispatches.Dispatches) == 0 {
		t.Fatal("expected dispatch history before retry-dispatch")
	}

	response, status := postFactorySessionRetryDispatch(t, server.URL, sessionID, factoryapi.FactorySessionRetryDispatchRequest{
		DispatchId: dispatchID,
	})
	if status != http.StatusOK {
		t.Fatalf("retry-dispatch status = %d, want 200", status)
	}
	assertLifecycleControlPreservesInspectionLinks(t, sessionID, response.Links)

	after := captureDurableSessionInspectionSnapshot(t, server.URL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, after.read.Links)
	if len(after.events) == 0 {
		t.Fatal("expected canonical events after retry-dispatch")
	}
	if len(after.dispatches.Dispatches) < len(before.dispatches.Dispatches) {
		t.Fatalf("dispatch history lost: before=%d after=%d", len(before.dispatches.Dispatches), len(after.dispatches.Dispatches))
	}
	getDurableDispatchDetail(t, server.URL, sessionID, dispatchID)
	assertEventReconnectStillWorks(t, server.URL, sessionID, after.events)
}

func TestLifecycleControls_ApproveInvalidStatePreservesInspectablePartialStateAcrossReadSurfaces(t *testing.T) {
	const sessionID = "dur-sess-approve-partial-001"
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			return runningAPIReadResult(sessionID), nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			return notReadyAPIResult(sessionID, factorysessionexecution.LifecycleStatusRunning), nil
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
		approve: func(context.Context, string, factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{},
				apiControlError("APPROVE", factorysessionexecution.LifecycleControlOutcomeInvalidState, factorysessionexecution.LifecycleStatusRunning)
		},
	}
	serverURL := durableRoleHTTPServer(t, service)

	before := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)

	_, approveStatus := postFactorySessionApprove(t, serverURL, sessionID, nil)
	if approveStatus != http.StatusConflict {
		t.Fatalf("approve status = %d, want 409", approveStatus)
	}

	after := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)
	assertDurableSessionReadUnchanged(t, before.read, after.read)
	assertDurableSessionResultUnchanged(t, before.result, after.result)
	assertDispatchListUnchanged(t, before.dispatches, after.dispatches)
	assertArtifactListUnchanged(t, before.artifacts, after.artifacts)
	assertLifecycleEventsNonDecreasing(t, before.events, after.events)
}

func TestLifecycleControls_CancelPreservesInspectablePartialStateAcrossReadSurfaces(t *testing.T) {
	const sessionID = "dur-sess-cancel-partial-001"
	canceled := false
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			if canceled {
				read.Status = factorysessionexecution.LifecycleStatus("CANCELED")
			}
			return read, nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			status := factorysessionexecution.LifecycleStatusRunning
			if canceled {
				status = factorysessionexecution.LifecycleStatus("CANCELED")
			}
			return notReadyAPIResult(sessionID, status), nil
		},
		listDispatches: func(context.Context, string) (factorysessionexecution.ListDispatchesResult, error) {
			return factorysessionexecution.ListDispatchesResult{SessionID: sessionID, Dispatches: []factorysessionexecution.DispatchSummary{}}, nil
		},
		listArtifacts: func(context.Context, string) (factorysessionexecution.ListArtifactsResult, error) {
			return factorysessionexecution.ListArtifactsResult{SessionID: sessionID, Artifacts: []factorysessionexecution.ArtifactSummary{}}, nil
		},
		readEvents: func(_ context.Context, _ string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			result := apiTerminalEvents(sessionID)
			if request.AfterEventID != "" {
				result.Events = result.Events[1:]
			}
			return result, nil
		},
		cancel: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			canceled = true
			return apiControlResult(sessionID, "CANCEL", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusCanceling), nil
		},
	}
	serverURL := durableRoleHTTPServer(t, service)

	before := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)

	_, cancelStatus := postFactorySessionLifecycleControl(t, serverURL, sessionID, "cancel", nil)
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("cancel status = %d, want 202", cancelStatus)
	}
	waitForDurableSessionStatus(t, serverURL, sessionID, factoryapi.FactorySessionDurableLifecycleStatusCanceled)

	after := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, after.read.Links)
	assertDispatchListUnchanged(t, before.dispatches, after.dispatches)
	assertArtifactListUnchanged(t, before.artifacts, after.artifacts)
	assertLifecycleEventsNonDecreasing(t, before.events, after.events)
	assertEventReconnectStillWorks(t, serverURL, sessionID, after.events)
}

func TestLifecycleControls_TerminatePreservesInspectablePartialStateAcrossReadSurfaces(t *testing.T) {
	const sessionID = "dur-sess-terminate-partial-001"
	terminated := false
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			read := runningAPIReadResult(sessionID)
			if terminated {
				read.Status = "TERMINATED"
			}
			return read, nil
		},
		getResult: func(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
			status := factorysessionexecution.LifecycleStatusRunning
			if terminated {
				status = "TERMINATED"
			}
			return notReadyAPIResult(sessionID, status), nil
		},
		listDispatches: func(context.Context, string) (factorysessionexecution.ListDispatchesResult, error) {
			return factorysessionexecution.ListDispatchesResult{SessionID: sessionID, Dispatches: []factorysessionexecution.DispatchSummary{}}, nil
		},
		listArtifacts: func(context.Context, string) (factorysessionexecution.ListArtifactsResult, error) {
			return factorysessionexecution.ListArtifactsResult{SessionID: sessionID, Artifacts: []factorysessionexecution.ArtifactSummary{}}, nil
		},
		readEvents: func(_ context.Context, _ string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			result := apiTerminalEvents(sessionID)
			if request.AfterEventID != "" {
				result.Events = result.Events[1:]
			}
			return result, nil
		},
		terminate: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			terminated = true
			return apiControlResult(sessionID, "TERMINATE", factorysessionexecution.LifecycleControlOutcomeAccepted, "TERMINATED"), nil
		},
	}
	serverURL := durableRoleHTTPServer(t, service)

	before := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)

	terminateResp, terminateStatus := postFactorySessionLifecycleControl(t, serverURL, sessionID, "terminate", nil)
	if terminateStatus != http.StatusOK {
		t.Fatalf("terminate status = %d, want 200", terminateStatus)
	}
	assertLifecycleControlPreservesInspectionLinks(t, sessionID, terminateResp.Links)

	after := captureDurableSessionInspectionSnapshot(t, serverURL, sessionID)
	assertDurableSessionInspectionLinks(t, sessionID, after.read.Links)
	assertDispatchListUnchanged(t, before.dispatches, after.dispatches)
	assertArtifactListUnchanged(t, before.artifacts, after.artifacts)
	assertLifecycleEventsNonDecreasing(t, before.events, after.events)
	assertEventReconnectStillWorks(t, serverURL, sessionID, after.events)
}
