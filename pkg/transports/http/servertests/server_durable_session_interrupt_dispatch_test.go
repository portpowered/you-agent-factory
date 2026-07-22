package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	interruptSessionID  = "dur-sess-api-interrupt-001"
	interruptDispatchID = "dispatch-1"
)

func TestInterruptFactorySessionDispatch_FixtureBackedRunningSessionReturnsTypedLifecycleControl(t *testing.T) {
	service := apiExecutionScript{
		interruptDispatch: func(_ context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
			if sessionID != interruptSessionID || request.DispatchID != interruptDispatchID {
				t.Fatalf("interrupt request = %q %#v", sessionID, request)
			}
			return apiInterruptResult("ACCEPTED", "INTERRUPTED"), nil
		},
	}
	response, status := postFactorySessionInterruptDispatch(
		t,
		durableRoleHTTPServer(t, service),
		interruptSessionID,
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: interruptDispatchID},
	)
	if status != http.StatusOK ||
		response.Operation != factoryapi.FactorySessionLifecycleControlKindInterruptDispatch ||
		response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		response.DispatchId == nil || *response.DispatchId != interruptDispatchID {
		t.Fatalf("response = %#v status=%d, want accepted interrupt", response, status)
	}
}

func TestLifecycleControls_TerminalSessionRejectedControlPreservesInspectablePartialStateAcrossReadSurfaces(t *testing.T) {
	service := terminalInspectableInterruptScript()
	serverURL := durableRoleHTTPServer(t, service)
	before := captureDurableSessionInspectionSnapshot(t, serverURL, interruptSessionID)
	response, status := postFactorySessionInterruptDispatch(
		t,
		serverURL,
		interruptSessionID,
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: interruptDispatchID},
	)
	if status != http.StatusConflict ||
		response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("response = %#v status=%d, want terminal conflict", response, status)
	}
	after := captureDurableSessionInspectionSnapshot(t, serverURL, interruptSessionID)
	assertDurableSessionReadUnchanged(t, before.read, after.read)
	assertDurableSessionResultUnchanged(t, before.result, after.result)
	assertDispatchListUnchanged(t, before.dispatches, after.dispatches)
	assertArtifactListUnchanged(t, before.artifacts, after.artifacts)
}

func TestInterruptFactorySessionDispatch_AlreadyInterruptedReturnsNoOp(t *testing.T) {
	service := apiExecutionScript{
		interruptDispatch: func(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
			return apiInterruptResult("NO_OP", "INTERRUPTED"), nil
		},
	}
	response, status := postFactorySessionInterruptDispatch(
		t, durableRoleHTTPServer(t, service), interruptSessionID,
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: interruptDispatchID},
	)
	if status != http.StatusOK || response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("response = %#v status=%d, want NO_OP", response, status)
	}
}

func TestInterruptFactorySessionDispatch_CompletedDispatchReturnsTypedConflict(t *testing.T) {
	assertInterruptConflict(t, factorysessions.LifecycleControlOutcomeTerminalSession, factorysessions.LifecycleStatusSucceeded)
}

func TestInterruptFactorySessionDispatch_QueuedDispatchReturnsTypedConflict(t *testing.T) {
	assertInterruptConflict(t, factorysessions.LifecycleControlOutcomeInvalidState, factorysessions.LifecycleStatusRunning)
}

func assertInterruptConflict(
	t *testing.T,
	outcome factorysessions.LifecycleControlOutcome,
	status factorysessions.LifecycleStatus,
) {
	t.Helper()
	service := apiExecutionScript{
		interruptDispatch: func(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
			return factorysessions.LifecycleControlResult{},
				apiControlError("INTERRUPT_DISPATCH", outcome, status)
		},
	}
	response, code := postFactorySessionInterruptDispatch(
		t, durableRoleHTTPServer(t, service), interruptSessionID,
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: interruptDispatchID},
	)
	if code != http.StatusConflict || string(response.Outcome) != string(outcome) {
		t.Fatalf("response = %#v status=%d, want conflict %q", response, code, outcome)
	}
}

func TestInterruptFactorySessionDispatch_MissingDispatchReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		interruptDispatch: func(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
			return factorysessions.LifecycleControlResult{}, factorysessions.ErrDispatchNotFound
		},
	}
	status, response := postFactorySessionInterruptDispatchExpectError(
		t, durableRoleHTTPServer(t, service), interruptSessionID,
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: "missing"},
	)
	if status != http.StatusNotFound || response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("response = %#v status=%d, want NOT_FOUND", response, status)
	}
}

func TestInterruptFactorySessionDispatch_MissingDispatchIDReturnsBadRequest(t *testing.T) {
	status, response := postFactorySessionInterruptDispatchExpectError(
		t, durableRoleHTTPServer(t, apiExecutionScript{}), interruptSessionID,
		factoryapi.FactorySessionInterruptDispatchRequest{},
	)
	if status != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("response = %#v status=%d, want BAD_REQUEST", response, status)
	}
}

func TestInterruptFactorySessionDispatch_NonDurableSessionPreservesLiveStub(t *testing.T) {
	server := httptest.NewServer(newDurableAPITestServer(nil).Handler())
	t.Cleanup(server.Close)
	status, response := postFactorySessionInterruptDispatchExpectError(
		t, server.URL, "live-session-001",
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: interruptDispatchID},
	)
	if status != http.StatusNotImplemented || response.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("response = %#v status=%d, want live-session stub", response, status)
	}
}

func TestInterruptFactorySessionDispatch_LateResultAfterInterruptSuppressedFromNormalRouting(t *testing.T) {
	service := interruptedTransportScript()
	serverURL := durableRoleHTTPServer(t, service)
	before := captureDurableSessionInspectionSnapshot(t, serverURL, interruptSessionID)
	after := captureDurableSessionInspectionSnapshot(t, serverURL, interruptSessionID)
	assertDurableSessionReadUnchanged(t, before.read, after.read)
	assertDurableSessionResultUnchanged(t, before.result, after.result)
	if after.dispatches.Dispatches[0].Status != factoryapi.FactoryDispatchStatusINTERRUPTED {
		t.Fatalf("dispatch = %#v, want INTERRUPTED after late result", after.dispatches.Dispatches)
	}
}

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsQueuedRunningCompletedPath(t *testing.T) {
	serverURL := durableRoleHTTPServer(t, completedProviderScript("mock", "live-provider-session-1", "live"))
	assertCompletedProviderProjection(t, serverURL, interruptSessionID, "mock", "live-provider-session-1")
}

func TestLiveProviderAndFakeChildSessions_APIPreserveDistinctProviderAndArtifactProjections(t *testing.T) {
	liveURL := durableRoleHTTPServer(t, completedProviderScript("mock", "live-provider-session-1", "live"))
	fakeURL := durableRoleHTTPServer(t, completedProviderScript("fake", "fake-provider-session-1", "fake"))
	live := getDurableDispatchDetail(t, liveURL, interruptSessionID, interruptDispatchID)
	fake := getDurableDispatchDetail(t, fakeURL, interruptSessionID, interruptDispatchID)
	if live.Provider == nil || fake.Provider == nil || *live.Provider == *fake.Provider {
		t.Fatalf("live provider=%#v fake provider=%#v, want distinct projections", live.Provider, fake.Provider)
	}
}

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsRunningDispatchBeforeCompletion(t *testing.T) {
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			return runningAPIReadResult(interruptSessionID), nil
		},
		listDispatches: func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
			result := providerDispatchList("mock", "live-provider-session-1", "live")
			result.Dispatches[0].Status = "RUNNING"
			return result, nil
		},
		getDispatch: func(context.Context, string, string) (factorysessions.DispatchDetail, error) {
			detail := providerDispatchDetail("mock", "live-provider-session-1", "live")
			detail.Status = "RUNNING"
			detail.StatusTransitions = []factorysessions.DispatchStatus{"QUEUED", "RUNNING"}
			return detail, nil
		},
	}
	serverURL := durableRoleHTTPServer(t, service)
	summary := getDurableDispatchList(t, serverURL, interruptSessionID).Dispatches[0]
	detail := getDurableDispatchDetail(t, serverURL, interruptSessionID, interruptDispatchID)
	if summary.Status != factoryapi.FactoryDispatchStatusRUNNING ||
		detail.Status != factoryapi.FactoryDispatchStatusRUNNING {
		t.Fatalf("summary=%#v detail=%#v, want RUNNING", summary, detail)
	}
}

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsFailedBridgedChildWithTypedFailureDetail(t *testing.T) {
	service := failedProviderScript()
	serverURL := durableRoleHTTPServer(t, service)
	snapshot := captureDurableSessionInspectionSnapshot(t, serverURL, interruptSessionID)
	if snapshot.read.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed ||
		len(snapshot.dispatches.Dispatches) != 1 ||
		snapshot.dispatches.Dispatches[0].Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("snapshot = %#v, want failed session and dispatch", snapshot)
	}
	detail := getDurableDispatchDetail(t, serverURL, interruptSessionID, interruptDispatchID)
	if detail.FailureDetail == nil ||
		detail.FailureDetail.Reason != factoryapi.WorkFailureType(workerexecution.WorkFailureTypePermanentBadRequest) {
		t.Fatalf("failureDetail = %#v, want typed permanent bad request", detail.FailureDetail)
	}
	if len(snapshot.artifacts.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for failed dispatch", snapshot.artifacts.Artifacts)
	}
}

func postFactorySessionInterruptDispatch(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, int) {
	t.Helper()
	resp := postInterruptRaw(t, serverURL, sessionID, request)
	defer resp.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode interrupt response: %v", err)
	}
	return response, resp.StatusCode
}

func postFactorySessionInterruptDispatchExpectError(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (int, factoryapi.ErrorResponse) {
	t.Helper()
	resp := postInterruptRaw(t, serverURL, sessionID, request)
	defer resp.Body.Close()
	var response factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode interrupt error: %v", err)
	}
	return resp.StatusCode, response
}

func postInterruptRaw(
	t *testing.T,
	serverURL, sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) *http.Response {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal interrupt: %v", err)
	}
	resp, err := http.Post(
		serverURL+"/factory-sessions/"+sessionID+"/interrupt-dispatch",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST interrupt: %v", err)
	}
	return resp
}

func apiInterruptResult(outcome string, status string) factorysessions.LifecycleControlResult {
	result := apiControlResult(
		interruptSessionID,
		"INTERRUPT_DISPATCH",
		factorysessions.LifecycleControlOutcome(outcome),
		factorysessions.LifecycleStatus(status),
	)
	result.DispatchID = interruptDispatchID
	return result
}

func terminalInspectableInterruptScript() apiExecutionScript {
	service := completedProviderScript("fake", "fake-provider-session-1", "fake")
	service.interruptDispatch = func(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
		return factorysessions.LifecycleControlResult{},
			apiControlError("INTERRUPT_DISPATCH", factorysessions.LifecycleControlOutcomeTerminalSession, factorysessions.LifecycleStatusSucceeded)
	}
	return service
}

func interruptedTransportScript() apiExecutionScript {
	service := completedProviderScript("mock", "live-provider-session-1", "live")
	service.getSession = func(context.Context, string) (factorysessions.SessionReadResult, error) {
		read := runningAPIReadResult(interruptSessionID)
		read.Status = factorysessions.LifecycleStatus("INTERRUPTED")
		return read, nil
	}
	service.getResult = func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
		return notReadyAPIResult(interruptSessionID, factorysessions.LifecycleStatus("INTERRUPTED")), nil
	}
	service.listDispatches = func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
		result := providerDispatchList("mock", "live-provider-session-1", "live")
		result.Dispatches[0].Status = "INTERRUPTED"
		return result, nil
	}
	service.getDispatch = func(context.Context, string, string) (factorysessions.DispatchDetail, error) {
		detail := providerDispatchDetail("mock", "live-provider-session-1", "live")
		detail.Status = "INTERRUPTED"
		detail.StatusTransitions = []factorysessions.DispatchStatus{"QUEUED", "RUNNING", "INTERRUPTED"}
		return detail, nil
	}
	return service
}

func completedProviderScript(provider, providerSessionID, mode string) apiExecutionScript {
	return apiExecutionScript{
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			read := terminalAPIReadResult(interruptSessionID)
			read.Progress = &factorysessions.ProgressCounts{TotalDispatches: 1, CompletedDispatches: 1}
			return read, nil
		},
		getResult: func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			return finalAPIResult(interruptSessionID), nil
		},
		listDispatches: func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
			return providerDispatchList(provider, providerSessionID, mode), nil
		},
		getDispatch: func(context.Context, string, string) (factorysessions.DispatchDetail, error) {
			return providerDispatchDetail(provider, providerSessionID, mode), nil
		},
		listArtifacts: func(context.Context, string) (factorysessions.ListArtifactsResult, error) {
			return factorysessions.ListArtifactsResult{
				SessionID: interruptSessionID,
				Artifacts: []factorysessions.ArtifactSummary{{
					ID:         "child-artifact-1",
					Kind:       "CHILD_RESULT",
					Visibility: "WORKFLOW_RUNTIME",
					Label:      "summarize-findings",
					DispatchID: interruptDispatchID,
				}},
			}, nil
		},
		readEvents: func(_ context.Context, _ string, request factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
			return apiTerminalEventsAfter(interruptSessionID, request), nil
		},
	}
}

func providerDispatchList(provider, providerSessionID, mode string) factorysessions.ListDispatchesResult {
	return factorysessions.ListDispatchesResult{
		SessionID: interruptSessionID,
		Dispatches: []factorysessions.DispatchSummary{{
			ID:           interruptDispatchID,
			Status:       "COMPLETED",
			DispatchKind: "JAVASCRIPT_AGENT",
			Phase:        "execute",
			Label:        "summarize-findings",
			Attempt:      1,
			Provider:     provider,
			ProviderSessionRefs: []factorysessions.ProviderSessionRef{{
				Provider: provider, Kind: "session_id", ID: providerSessionID,
			}},
			OutputArtifactIDs: []string{"child-artifact-1"},
			JavaScript: &factorysessions.DispatchJavaScriptProjection{
				TaskKind: "AGENT", TaskLabel: "summarize-findings", ExecutionMode: mode,
			},
		}},
	}
}

func providerDispatchDetail(provider, providerSessionID, mode string) factorysessions.DispatchDetail {
	projection := &factorysessions.DispatchJavaScriptProjection{
		TaskKind: "AGENT", TaskLabel: "summarize-findings", ExecutionMode: mode,
	}
	return factorysessions.DispatchDetail{
		DispatchSummary:   providerDispatchList(provider, providerSessionID, mode).Dispatches[0],
		SessionID:         interruptSessionID,
		OrchestratorKind:  "JAVASCRIPT",
		StatusTransitions: []factorysessions.DispatchStatus{"QUEUED", "RUNNING", "COMPLETED"},
		JavaScript:        projection,
	}
}

func failedProviderScript() apiExecutionScript {
	failure := &factorysessions.DispatchFailureDetail{
		Reason:  string(workerexecution.WorkFailureTypePermanentBadRequest),
		Message: "Provider rejected the request as invalid.",
	}
	return apiExecutionScript{
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			read := terminalAPIReadResult(interruptSessionID)
			read.Status = factorysessions.LifecycleStatusFailed
			read.ResultSummary.ResultStatus = "FAILED"
			read.Failure = &factorysessions.FailureSummary{Reason: failure.Reason, Message: failure.Message}
			return read, nil
		},
		getResult: func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			return factorysessions.ResultReadResult{
				SessionID: interruptSessionID, ResultStatus: "FAILED",
				SessionStatus: factorysessions.LifecycleStatusFailed,
				Failure:       &factorysessions.FailureSummary{Reason: failure.Reason, Message: failure.Message},
			}, nil
		},
		listDispatches: func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
			result := providerDispatchList("mock", "", "live")
			result.Dispatches[0].Status = "FAILED"
			result.Dispatches[0].FailureDetail = failure
			return result, nil
		},
		getDispatch: func(context.Context, string, string) (factorysessions.DispatchDetail, error) {
			detail := providerDispatchDetail("mock", "", "live")
			detail.Status = "FAILED"
			detail.FailureDetail = failure
			detail.StatusTransitions = []factorysessions.DispatchStatus{"QUEUED", "RUNNING", "FAILED"}
			return detail, nil
		},
		listArtifacts: func(context.Context, string) (factorysessions.ListArtifactsResult, error) {
			return factorysessions.ListArtifactsResult{
				SessionID: interruptSessionID,
				Artifacts: []factorysessions.ArtifactSummary{},
			}, nil
		},
		readEvents: func(_ context.Context, _ string, request factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
			return apiTerminalEventsAfter(interruptSessionID, request), nil
		},
	}
}

func assertCompletedProviderProjection(
	t *testing.T,
	serverURL, sessionID, provider, providerSessionID string,
) {
	t.Helper()
	read := getDurableFactorySession(t, serverURL, sessionID)
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("read status = %q, want SUCCEEDED", read.Status)
	}
	summary := getDurableDispatchList(t, serverURL, sessionID).Dispatches[0]
	detail := getDurableDispatchDetail(t, serverURL, sessionID, interruptDispatchID)
	if summary.Status != factoryapi.FactoryDispatchStatusCOMPLETED ||
		detail.Status != factoryapi.FactoryDispatchStatusCOMPLETED ||
		detail.Provider == nil || *detail.Provider != provider ||
		detail.ProviderSessionRefs == nil ||
		(*detail.ProviderSessionRefs)[0].Id != providerSessionID {
		t.Fatalf("summary=%#v detail=%#v, want completed provider projection", summary, detail)
	}
}
