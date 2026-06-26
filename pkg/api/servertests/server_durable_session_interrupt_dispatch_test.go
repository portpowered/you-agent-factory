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
	defer server.Close()

	response, status := postFactorySessionInterruptDispatch(t, server.URL, sessionID, factoryapi.FactorySessionInterruptDispatchRequest{
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

	resp, err := http.Get(server.URL + "/factory-sessions/" + sessionID + "/dispatches/dispatch-1")
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

	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Progress != nil && session.Progress.CompletedDispatches != 0 {
		t.Fatalf("completedDispatches = %d, want 0 after late result suppression", session.Progress.CompletedDispatches)
	}

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
		t.Fatal("DISPATCH_INTERRUPTED event missing after transport interrupt and late completion")
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
