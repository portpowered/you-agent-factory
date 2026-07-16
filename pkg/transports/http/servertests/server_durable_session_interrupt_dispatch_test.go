package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

func TestLifecycleControls_TerminalSessionRejectedControlPreservesInspectablePartialStateAcrossReadSurfaces(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-lifecycle-all-surfaces-terminal-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{"subject": "workflows"},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	before := captureDurableSessionInspectionSnapshot(t, server.URL, completed.SessionID)
	if len(before.dispatches.Dispatches) == 0 {
		t.Fatal("expected dispatch history on completed agent-run-fake-child session")
	}

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
	getDurableDispatchDetail(t, server.URL, completed.SessionID, "dispatch-1")
	assertPostControlEventsAlignWithStatus(t, completed.SessionID, after.events, after.read.Status)
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
	service := newAPIFakeExecutionService(t, factorysessionexecution.WithFakeScenarios(factorysessionexecution.FakeScenario{
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
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
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
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
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
	if errResp.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
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
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
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

func apiLiveProviderSyncCompletedSession(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	service := newAPILiveProviderRuntimeService(t)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-live-provider-dispatch-sync-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
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
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	return server, completed.SessionID
}

func assertAPILiveProviderOneCompletedDispatchProgress(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	sessionRead := getDurableFactorySession(t, serverURL, sessionID)
	if sessionRead.Progress == nil ||
		sessionRead.Progress.TotalDispatches == nil || *sessionRead.Progress.TotalDispatches != 1 ||
		sessionRead.Progress.CompletedDispatches == nil || *sessionRead.Progress.CompletedDispatches != 1 {
		t.Fatalf("session progress = %#v, want one completed dispatch", sessionRead.Progress)
	}
}

func assertAPILiveProviderCompletedDispatchSummary(
	t *testing.T,
	dispatchSummary factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()
	if dispatchSummary.Id != "dispatch-1" {
		t.Fatalf("dispatch id = %q, want dispatch-1", dispatchSummary.Id)
	}
	if dispatchSummary.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch status = %q, want COMPLETED", dispatchSummary.Status)
	}
	if dispatchSummary.Provider == nil || *dispatchSummary.Provider != "mock" {
		t.Fatalf("dispatch provider = %#v, want mock", dispatchSummary.Provider)
	}
	if dispatchSummary.Javascript == nil || dispatchSummary.Javascript.ExecutionMode == nil ||
		*dispatchSummary.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("dispatch executionMode = %#v, want live-provider", dispatchSummary.Javascript)
	}
	if dispatchSummary.ProviderSessionRefs == nil || len(*dispatchSummary.ProviderSessionRefs) != 1 ||
		(*dispatchSummary.ProviderSessionRefs)[0].Id != "live-provider-session-1" {
		t.Fatalf("providerSessionRefs = %#v", dispatchSummary.ProviderSessionRefs)
	}
	if dispatchSummary.OutputArtifactIds == nil || len(*dispatchSummary.OutputArtifactIds) != 1 ||
		(*dispatchSummary.OutputArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("outputArtifactIds = %#v, want [child-artifact-1]", dispatchSummary.OutputArtifactIds)
	}
}

func assertAPILiveProviderCompletedDispatchDetail(
	t *testing.T,
	dispatchDetail factoryapi.FactoryDispatch,
) {
	t.Helper()
	if dispatchDetail.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch detail status = %q, want COMPLETED", dispatchDetail.Status)
	}
	if dispatchDetail.Javascript == nil || dispatchDetail.Javascript.ExecutionMode == nil ||
		*dispatchDetail.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("dispatch detail executionMode = %#v, want live-provider", dispatchDetail.Javascript)
	}
	assertAPIDispatchStatusTransitions(t, dispatchDetail.StatusTransitions, []factoryapi.FactoryDispatchStatus{
		factoryapi.FactoryDispatchStatusQUEUED,
		factoryapi.FactoryDispatchStatusRUNNING,
		factoryapi.FactoryDispatchStatusCOMPLETED,
	})
	if dispatchDetail.ArtifactIds == nil || len(*dispatchDetail.ArtifactIds) != 1 ||
		(*dispatchDetail.ArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("dispatch artifactIds = %#v, want [child-artifact-1]", dispatchDetail.ArtifactIds)
	}
}

func assertAPILiveProviderCompletedDispatchReads(
	t *testing.T,
	serverURL, sessionID string,
) (factoryapi.FactorySessionDispatchSummary, factoryapi.FactoryDispatch) {
	t.Helper()
	dispatchList := getDurableDispatchList(t, serverURL, sessionID)
	if len(dispatchList.Dispatches) != 1 {
		t.Fatalf("dispatch list = %#v, want one dispatch", dispatchList.Dispatches)
	}
	dispatchSummary := dispatchList.Dispatches[0]
	assertAPILiveProviderCompletedDispatchSummary(t, dispatchSummary)
	dispatchDetail := getDurableDispatchDetail(t, serverURL, sessionID, "dispatch-1")
	assertAPILiveProviderCompletedDispatchDetail(t, dispatchDetail)
	return dispatchSummary, dispatchDetail
}

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsQueuedRunningCompletedPath(t *testing.T) {
	server, sessionID := apiLiveProviderSyncCompletedSession(t)
	defer server.Close()

	assertAPILiveProviderOneCompletedDispatchProgress(t, server.URL, sessionID)
	dispatchSummary, dispatchDetail := assertAPILiveProviderCompletedDispatchReads(t, server.URL, sessionID)
	assertAPILiveProviderProviderSessionRef(t, dispatchSummary.ProviderSessionRefs)
	assertAPILiveProviderArtifactLineage(t, server.URL, sessionID, dispatchSummary, dispatchDetail)

	events := getDurableFactorySessionEvents(t, server.URL, sessionID, "")
	assertAPILiveProviderDispatchLifecycleEventsAlignWithReads(
		t,
		events,
		"dispatch-1",
		dispatchSummary,
		dispatchDetail,
	)
}

func TestLiveProviderAndFakeChildSessions_APIPreserveDistinctProviderAndArtifactProjections(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")

	fakeService := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	fakeCompleted, err := fakeService.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-live-provider-fake-child-coexist-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("fake StartSync: %v", err)
	}

	liveService := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeLive,
		factorysessionexecution.SmokeLiveChildProvider())
	liveCompleted, err := liveService.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-live-provider-live-child-coexist-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &factorysessionexecution.RuntimeOptions{
			ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("live StartSync: %v", err)
	}

	fakeServer := httptest.NewServer(newAPITestServer(&testutil.MockFactory{DurableExecutionService: fakeService}).Handler())
	defer fakeServer.Close()
	liveServer := httptest.NewServer(newAPITestServer(&testutil.MockFactory{DurableExecutionService: liveService}).Handler())
	defer liveServer.Close()

	fakeDispatch := getDurableDispatchList(t, fakeServer.URL, fakeCompleted.SessionID).Dispatches[0]
	if fakeDispatch.Javascript == nil || fakeDispatch.Javascript.ExecutionMode == nil ||
		*fakeDispatch.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeFake {
		t.Fatalf("fake dispatch executionMode = %#v, want fake", fakeDispatch.Javascript)
	}
	if fakeDispatch.ProviderSessionRefs == nil || len(*fakeDispatch.ProviderSessionRefs) != 1 ||
		(*fakeDispatch.ProviderSessionRefs)[0].Id != "fake-provider-session-1" {
		t.Fatalf("fake providerSessionRefs = %#v", fakeDispatch.ProviderSessionRefs)
	}
	fakeArtifact := getDurableArtifactList(t, fakeServer.URL, fakeCompleted.SessionID).Artifacts[0]
	if fakeArtifact.DispatchId == nil || *fakeArtifact.DispatchId != "dispatch-1" {
		t.Fatalf("fake artifact dispatchId = %#v, want dispatch-1", fakeArtifact.DispatchId)
	}

	liveDispatch := getDurableDispatchList(t, liveServer.URL, liveCompleted.SessionID).Dispatches[0]
	if liveDispatch.Javascript == nil || liveDispatch.Javascript.ExecutionMode == nil ||
		*liveDispatch.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("live dispatch executionMode = %#v, want live-provider", liveDispatch.Javascript)
	}
	assertAPILiveProviderProviderSessionRef(t, liveDispatch.ProviderSessionRefs)
	liveDispatchDetail := getDurableDispatchDetail(t, liveServer.URL, liveCompleted.SessionID, "dispatch-1")
	assertAPILiveProviderArtifactLineage(t, liveServer.URL, liveCompleted.SessionID, liveDispatch, liveDispatchDetail)

	fakeSnapshot := captureDurableSessionInspectionSnapshot(t, fakeServer.URL, fakeCompleted.SessionID)
	assertAPIFakeChildInspectionSnapshot(t, fakeSnapshot)
	liveSnapshot := captureDurableSessionInspectionSnapshot(t, liveServer.URL, liveCompleted.SessionID)
	if liveSnapshot.read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("live session status = %q, want SUCCEEDED", liveSnapshot.read.Status)
	}
}

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsRunningDispatchBeforeCompletion(t *testing.T) {
	service, provider := newAPILiveProviderBlockingRuntimeService(t)
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-live-provider-dispatch-async-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &factorysessionexecution.RuntimeOptions{
			ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	provider.waitForInferStart(t)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	runningDispatch := waitForAPIDispatchStatus(
		t,
		server.URL,
		started.SessionID,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusRUNNING,
		2*time.Second,
	)
	if runningDispatch.Javascript == nil || runningDispatch.Javascript.ExecutionMode == nil ||
		*runningDispatch.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("running dispatch executionMode = %#v, want live-provider", runningDispatch.Javascript)
	}

	sessionRead := getDurableFactorySession(t, server.URL, started.SessionID)
	if sessionRead.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session status while child running = %q, want RUNNING", sessionRead.Status)
	}
	if sessionRead.Progress == nil || sessionRead.Progress.TotalDispatches == nil ||
		*sessionRead.Progress.TotalDispatches != 1 {
		t.Fatalf("session progress while child running = %#v, want one total dispatch", sessionRead.Progress)
	}

	provider.releaseInfer()
	waitForRuntimeSessionTerminal(t, service, started.SessionID)

	completedDetail := getDurableDispatchDetail(t, server.URL, started.SessionID, "dispatch-1")
	if completedDetail.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch detail after completion = %q, want COMPLETED", completedDetail.Status)
	}
	assertAPIDispatchStatusTransitions(t, completedDetail.StatusTransitions, []factoryapi.FactoryDispatchStatus{
		factoryapi.FactoryDispatchStatusQUEUED,
		factoryapi.FactoryDispatchStatusRUNNING,
		factoryapi.FactoryDispatchStatusCOMPLETED,
	})
}

func assertAPILiveProviderFailedSessionRead(t *testing.T, snapshot durableSessionInspectionSnapshot) {
	t.Helper()
	if snapshot.read.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", snapshot.read.Status)
	}
	if snapshot.read.Progress == nil ||
		snapshot.read.Progress.TotalDispatches == nil || *snapshot.read.Progress.TotalDispatches != 1 ||
		snapshot.read.Progress.FailedDispatches == nil || *snapshot.read.Progress.FailedDispatches != 1 {
		t.Fatalf("session progress = %#v, want one failed dispatch", snapshot.read.Progress)
	}
	if snapshot.read.FailureDetail == nil || snapshot.read.FailureDetail.Reason == "" {
		t.Fatalf("session failure = %#v, want typed workflow failure", snapshot.read.FailureDetail)
	}
}

func assertAPILiveProviderFailedSessionSnapshot(
	t *testing.T,
	snapshot durableSessionInspectionSnapshot,
	dispatchID string,
) (factoryapi.FactorySessionDispatchSummary, factoryapi.FactoryDispatch) {
	t.Helper()
	assertAPILiveProviderFailedSessionRead(t, snapshot)
	if len(snapshot.dispatches.Dispatches) != 1 {
		t.Fatalf("dispatch list = %#v, want one dispatch", snapshot.dispatches.Dispatches)
	}
	dispatchSummary := snapshot.dispatches.Dispatches[0]
	if dispatchSummary.Id != dispatchID {
		t.Fatalf("dispatch id = %q, want %q", dispatchSummary.Id, dispatchID)
	}
	if dispatchSummary.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch status = %q, want FAILED", dispatchSummary.Status)
	}
	if dispatchSummary.Javascript == nil || dispatchSummary.Javascript.ExecutionMode == nil ||
		*dispatchSummary.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("dispatch executionMode = %#v, want live-provider", dispatchSummary.Javascript)
	}
	assertAPILiveProviderDispatchFailureDetail(t, dispatchSummary.FailureDetail)
	if dispatchSummary.Attempt == nil || *dispatchSummary.Attempt != 1 || dispatchSummary.Retryable == nil || *dispatchSummary.Retryable ||
		dispatchSummary.FailureClassification == nil || *dispatchSummary.FailureClassification != factoryapi.WorkFailureTypePermanentBadRequest {
		t.Fatalf("dispatch retry diagnostics = %#v", dispatchSummary)
	}
	return dispatchSummary, factoryapi.FactoryDispatch{}
}

func assertAPILiveProviderFailedDispatchDetail(
	t *testing.T,
	serverURL, sessionID, dispatchID string,
	dispatchSummary factoryapi.FactorySessionDispatchSummary,
) factoryapi.FactoryDispatch {
	t.Helper()
	dispatchDetail := getDurableDispatchDetail(t, serverURL, sessionID, dispatchID)
	if dispatchDetail.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch detail status = %q, want FAILED", dispatchDetail.Status)
	}
	if dispatchDetail.Javascript == nil || dispatchDetail.Javascript.ExecutionMode == nil ||
		*dispatchDetail.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("dispatch detail executionMode = %#v, want live-provider", dispatchDetail.Javascript)
	}
	assertAPILiveProviderDispatchFailureDetail(t, dispatchDetail.FailureDetail)
	if dispatchDetail.Attempt == nil || *dispatchDetail.Attempt != 1 || dispatchDetail.Retryable == nil || *dispatchDetail.Retryable ||
		dispatchDetail.FailureClassification == nil || *dispatchDetail.FailureClassification != factoryapi.WorkFailureTypePermanentBadRequest {
		t.Fatalf("dispatch detail retry diagnostics = %#v", dispatchDetail)
	}
	assertAPIDispatchStatusTransitions(t, dispatchDetail.StatusTransitions, []factoryapi.FactoryDispatchStatus{
		factoryapi.FactoryDispatchStatusQUEUED,
		factoryapi.FactoryDispatchStatusRUNNING,
		factoryapi.FactoryDispatchStatusFAILED,
	})
	if dispatchDetail.ArtifactIds != nil && len(*dispatchDetail.ArtifactIds) != 0 {
		t.Fatalf("dispatch artifactIds = %#v, want none for failed child", dispatchDetail.ArtifactIds)
	}
	if dispatchSummary.OutputArtifactIds != nil && len(*dispatchSummary.OutputArtifactIds) != 0 {
		t.Fatalf("dispatch outputArtifactIds = %#v, want none for failed child", dispatchSummary.OutputArtifactIds)
	}
	return dispatchDetail
}

func assertAPILiveProviderFailedDispatchHasNoArtifacts(
	t *testing.T,
	serverURL, sessionID string,
	snapshot durableSessionInspectionSnapshot,
) {
	t.Helper()
	artifactList := getDurableArtifactList(t, serverURL, sessionID)
	if len(artifactList.Artifacts) != 0 {
		t.Fatalf("artifact list = %#v, want none for failed child", artifactList.Artifacts)
	}
	if snapshot.read.ArtifactRefs != nil && len(*snapshot.read.ArtifactRefs) != 0 {
		t.Fatalf("session artifactRefs = %#v, want none for failed child", snapshot.read.ArtifactRefs)
	}
}

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsFailedBridgedChildWithTypedFailureDetail(t *testing.T) {
	service := newAPILifecycleFailingChildRuntimeService(t)
	sessionID, dispatchID := startRuntimeBackedFailedSessionWithDispatch(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	snapshot := captureDurableSessionInspectionSnapshot(t, server.URL, sessionID)
	dispatchSummary, _ := assertAPILiveProviderFailedSessionSnapshot(t, snapshot, dispatchID)
	dispatchDetail := assertAPILiveProviderFailedDispatchDetail(t, server.URL, sessionID, dispatchID, dispatchSummary)
	assertAPILiveProviderFailedDispatchHasNoArtifacts(t, server.URL, sessionID, snapshot)

	assertPostControlEventsAlignWithStatus(t, sessionID, snapshot.events, snapshot.read.Status)
	assertAPILiveProviderDispatchLifecycleEventsAlignWithReads(
		t,
		snapshot.events,
		dispatchID,
		dispatchSummary,
		dispatchDetail,
	)
	if snapshot.result.SessionStatus == nil ||
		*snapshot.result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("result sessionStatus = %#v, want FAILED", snapshot.result.SessionStatus)
	}
}
