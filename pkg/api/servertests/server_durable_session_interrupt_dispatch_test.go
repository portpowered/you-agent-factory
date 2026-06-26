package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestInterruptFactorySessionDispatch_FixtureBackedRunningSessionReturnsTypedLifecycleControl(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	row := startAPIRunningSessionForControl(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	response, status := postFactorySessionInterruptDispatch(t, server.URL, row.SessionID, factoryapi.FactorySessionInterruptDispatchRequest{
		DispatchId: "disp-js-002",
		Reason:     stringPtr("stop bad run"),
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindInterruptDispatch {
		t.Fatalf("operation = %q, want INTERRUPT_DISPATCH", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.DispatchId == nil || *response.DispatchId != "disp-js-002" {
		t.Fatalf("dispatchId = %#v, want disp-js-002", response.DispatchId)
	}

	dispatch, err := service.GetDispatch(context.Background(), row.SessionID, "disp-js-002")
	if err != nil {
		t.Fatalf("GetDispatch after interrupt: %v", err)
	}
	if dispatch.Status != factorysessionexecution.DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "stop bad run" {
		t.Fatalf("failureDetail = %#v, want stop bad run", dispatch.FailureDetail)
	}

	events, err := service.ReadEvents(context.Background(), row.SessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after interrupt: %v", err)
	}
	foundInterruptedEvent := false
	for _, raw := range events.Events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type == "DISPATCH_INTERRUPTED" {
			foundInterruptedEvent = true
			break
		}
	}
	if !foundInterruptedEvent {
		t.Fatal("DISPATCH_INTERRUPTED event missing after interrupt")
	}
}

func TestInterruptFactorySessionDispatch_AlreadyInterruptedReturnsNoOp(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	row := startAPIRunningSessionForControl(t, service)
	serverURL := serverURLForLifecycle(t, service)

	first, status := postFactorySessionInterruptDispatch(t, serverURL, row.SessionID, factoryapi.FactorySessionInterruptDispatchRequest{
		DispatchId: "disp-js-002",
	})
	if status != http.StatusOK || first.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("first interrupt = status %d outcome %q", status, first.Outcome)
	}

	second, status := postFactorySessionInterruptDispatch(t, serverURL, row.SessionID, factoryapi.FactorySessionInterruptDispatchRequest{
		DispatchId: "disp-js-002",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if second.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", second.Outcome)
	}
}

func TestInterruptFactorySessionDispatch_CompletedDispatchReturnsTypedConflict(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	row := startAPIRunningSessionForControl(t, service)
	serverURL := serverURLForLifecycle(t, service)

	response, status := postFactorySessionInterruptDispatch(t, serverURL, row.SessionID, factoryapi.FactorySessionInterruptDispatchRequest{
		DispatchId: "disp-js-001",
	})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
		t.Fatalf("outcome = %q, want INVALID_STATE", response.Outcome)
	}
}

func TestInterruptFactorySessionDispatch_QueuedDispatchReturnsTypedConflict(t *testing.T) {
	service := factorysessionexecution.NewFakeService(factorysessionexecution.WithFakeScenarios(factorysessionexecution.FakeScenario{
		ID:        "queued-interrupt-invalid-state",
		RequestID: "req-js-queued-interrupt-001",
		Session: factorysessionexecution.SessionReadResult{
			SessionID: "dur-sess-js-queued-interrupt-001",
			Status:    factorysessionexecution.LifecycleStatusRunning,
		},
		Dispatches: []factorysessionexecution.DispatchSummary{{
			ID:     "disp-js-queued-001",
			Status: factorysessionexecution.DispatchStatusQueued,
		}},
		DispatchDetails: map[string]factorysessionexecution.DispatchDetail{
			"disp-js-queued-001": {
				DispatchSummary: factorysessionexecution.DispatchSummary{
					ID:     "disp-js-queued-001",
					Status: factorysessionexecution.DispatchStatusQueued,
				},
			},
		},
		Result: factorysessionexecution.ResultReadResult{
			SessionID:     "dur-sess-js-queued-interrupt-001",
			SessionStatus: factorysessionexecution.LifecycleStatusRunning,
			ResultStatus:  factorysessionexecution.ResultStatusNotReady,
		},
	}))
	_, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-queued-interrupt-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/run-n.yaml",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync queued session: %v", err)
	}
	serverURL := serverURLForLifecycle(t, service)

	response, status := postFactorySessionInterruptDispatch(t, serverURL, "dur-sess-js-queued-interrupt-001", factoryapi.FactorySessionInterruptDispatchRequest{
		DispatchId: "disp-js-queued-001",
	})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
		t.Fatalf("outcome = %q, want INVALID_STATE", response.Outcome)
	}
}

func TestInterruptFactorySessionDispatch_MissingDispatchReturnsNotFound(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	startAPIRunningSessionForControl(t, service)
	serverURL := serverURLForLifecycle(t, service)

	status, errResp := postFactorySessionInterruptDispatchExpectError(
		t,
		serverURL,
		"dur-sess-js-run-n-001",
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: "disp-missing-001"},
	)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if errResp.Code != factoryapi.NOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestInterruptFactorySessionDispatch_MissingDispatchIDReturnsBadRequest(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	startAPIRunningSessionForControl(t, service)
	serverURL := serverURLForLifecycle(t, service)

	status, errResp := postFactorySessionInterruptDispatchExpectError(
		t,
		serverURL,
		"dur-sess-js-run-n-001",
		factoryapi.FactorySessionInterruptDispatchRequest{},
	)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if errResp.Code != factoryapi.BADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}
}

func TestInterruptFactorySessionDispatch_NonDurableSessionPreservesLiveStub(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	serverURL := serverURLForLifecycle(t, service)

	status, errResp := postFactorySessionInterruptDispatchExpectError(
		t,
		serverURL,
		"live-session-001",
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: "disp-live-001"},
	)
	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", status)
	}
	if errResp.Code != factoryapi.INTERNALERROR {
		t.Fatalf("code = %q, want INTERNAL_ERROR", errResp.Code)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this transport regression keeps interrupt, late completion, and suppressed routing assertions together.
func TestInterruptFactorySessionDispatch_LateResultAfterInterruptSuppressedFromNormalRouting(t *testing.T) {
	service, serverURL, sessionID := newInterruptedLateResultTransportHarness(t)

	response, status := postFactorySessionInterruptDispatch(t, serverURL, sessionID, factoryapi.FactorySessionInterruptDispatchRequest{
		DispatchId: "dispatch-1",
		Reason:     stringPtr("stop before provider completion"),
	})
	if status != http.StatusOK {
		t.Fatalf("interrupt status = %d, want 200", status)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.DispatchId == nil || *response.DispatchId != "dispatch-1" {
		t.Fatalf("dispatchId = %#v, want dispatch-1", response.DispatchId)
	}

	applyLateInterruptTransportOutcome(t, service, sessionID)
	assertInterruptedDispatchTransportState(t, serverURL, sessionID)
	assertInterruptedSessionTransportState(t, service, sessionID)
	assertDispatchInterruptedEventRecorded(t, service, sessionID, "after transport interrupt and late completion")
}

func newInterruptedLateResultTransportHarness(t *testing.T) (*factorysessionexecution.JavaScriptRuntimeService, string, string) {
	t.Helper()
	projectRoot := setupAPILifecycleWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	sessionID := "dur-sess-interrupt-transport-late-001"
	if err := factorysessionexecution.SeedRuntimeSessionWithRunningDispatch(service, sessionID, "dispatch-1", "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	return service, server.URL, sessionID
}

func applyLateInterruptTransportOutcome(t *testing.T, service *factorysessionexecution.JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	lateRecords := []workflowruntime.RuntimeRecord{{
		Kind: workflowruntime.RecordKindChildDispatch,
		ChildDispatch: &workflowruntime.ChildDispatchRecord{
			DispatchID:         "dispatch-1",
			Status:             workflowruntime.ChildDispatchStatusCompleted,
			Label:              "summarize-findings",
			ArtifactRef:        workflowresult.FormatArtifactURI(sessionID, "child-artifact-late"),
			ProviderSessionRef: "provider-session-late",
			Provider:           "mock",
		},
	}}
	if err := factorysessionexecution.ApplyRuntimeTerminalOutcomeForTests(service, sessionID, workflowruntime.Outcome{
		OK:      true,
		Records: lateRecords,
		Value:   workflowresult.TypedValue{JSON: json.RawMessage(`{"label":"agent-run-fake-child"}`)},
	}); err != nil {
		t.Fatalf("ApplyRuntimeTerminalOutcomeForTests: %v", err)
	}
}

func assertInterruptedDispatchTransportState(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID + "/dispatches/dispatch-1")
	if err != nil {
		t.Fatalf("GET dispatch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dispatch GET status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var dispatch factoryapi.FactoryDispatch
	if err := json.NewDecoder(resp.Body).Decode(&dispatch); err != nil {
		t.Fatalf("decode dispatch: %v", err)
	}
	if dispatch.Status != factoryapi.FactoryDispatchStatusINTERRUPTED {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	if dispatch.ArtifactIds != nil && len(*dispatch.ArtifactIds) != 0 {
		t.Fatalf("artifactIds = %#v, want suppressed late child output", *dispatch.ArtifactIds)
	}
}

func assertInterruptedSessionTransportState(t *testing.T, service *factorysessionexecution.JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != factorysessionexecution.LifecycleStatusInterrupted {
		t.Fatalf("session status = %q, want INTERRUPTED after late result suppression", session.Status)
	}
	if session.Progress != nil && session.Progress.CompletedDispatches != 0 {
		t.Fatalf("completedDispatches = %d, want 0 after late result suppression", session.Progress.CompletedDispatches)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(factorysessionexecution.ResultStatusUnavailable) {
		t.Fatalf("resultSummary = %#v, want UNAVAILABLE after late result suppression", session.ResultSummary)
	}

	result, err := service.GetResult(context.Background(), sessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.SessionStatus != factorysessionexecution.LifecycleStatusInterrupted ||
		result.ResultStatus != factorysessionexecution.ResultStatusUnavailable {
		t.Fatalf("result = status %q session %q, want UNAVAILABLE/INTERRUPTED", result.ResultStatus, result.SessionStatus)
	}
}

func assertDispatchInterruptedEventRecorded(
	t *testing.T,
	service *factorysessionexecution.JavaScriptRuntimeService,
	sessionID, contextSuffix string,
) {
	t.Helper()
	events, err := service.ReadEvents(context.Background(), sessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	foundInterruptedEvent := false
	for _, raw := range events.Events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type == "DISPATCH_INTERRUPTED" {
			foundInterruptedEvent = true
			break
		}
	}
	if !foundInterruptedEvent {
		t.Fatalf("DISPATCH_INTERRUPTED event missing %s", contextSuffix)
	}
}

func postFactorySessionInterruptDispatch(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, int) {
	t.Helper()
	resp, err := postFactorySessionInterruptDispatchRaw(t, serverURL, sessionID, request)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/interrupt-dispatch: %v", sessionID, err)
	}
	defer resp.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode interrupt-dispatch response: %v", err)
	}
	return response, resp.StatusCode
}

func postFactorySessionInterruptDispatchExpectError(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (int, factoryapi.ErrorResponse) {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	url := serverURL + "/factory-sessions/" + sessionID + "/interrupt-dispatch"
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

func postFactorySessionInterruptDispatchRaw(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (*http.Response, error) {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	url := serverURL + "/factory-sessions/" + sessionID + "/interrupt-dispatch"
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
