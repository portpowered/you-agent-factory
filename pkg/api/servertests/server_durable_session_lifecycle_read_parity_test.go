package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestLifecycleControls_PreserveRunningSessionReadParity(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	beforeRead := getDurableFactorySession(t, server.URL, started.SessionID)
	assertDurableSessionInspectionLinks(t, started.SessionID, beforeRead.Links)
	beforeResult := getDurableFactorySessionResult(t, server.URL, started.SessionID, "")
	beforeEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionID, "")
	beforeList := getFactorySessionList(t, server.URL, "all")

	pauseResp, pauseStatus := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "pause", nil)
	if pauseStatus != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", pauseStatus)
	}
	assertLifecycleControlPreservesInspectionLinks(t, started.SessionID, pauseResp.Links)
	assertReadSurfacesReachableAfterLifecycle(t, server.URL, started.SessionID)
	assertEventReconnectStillWorks(t, server.URL, started.SessionID, beforeEvents)

	pausedRead := getDurableFactorySession(t, server.URL, started.SessionID)
	if pausedRead.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status after pause = %q, want PAUSED", pausedRead.Status)
	}
	assertDurableSessionInspectionLinks(t, started.SessionID, pausedRead.Links)

	resumeResp, resumeStatus := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "resume", nil)
	if resumeStatus != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", resumeStatus)
	}
	assertLifecycleControlPreservesInspectionLinks(t, started.SessionID, resumeResp.Links)

	afterRead := getDurableFactorySession(t, server.URL, started.SessionID)
	if afterRead.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status after resume = %q, want RUNNING", afterRead.Status)
	}
	assertDurableSessionInspectionLinks(t, started.SessionID, afterRead.Links)

	afterResult := getDurableFactorySessionResult(t, server.URL, started.SessionID, "")
	assertDurableSessionResultUnchanged(t, beforeResult, afterResult)

	afterEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionID, "")
	if len(afterEvents) < len(beforeEvents) {
		t.Fatalf("event count after lifecycle = %d, want at least %d", len(afterEvents), len(beforeEvents))
	}

	afterList := getFactorySessionList(t, server.URL, "all")
	if !containsSessionInListResponse(afterList, started.SessionID) {
		t.Fatalf("all-scope list = %#v, want session %q", afterList, started.SessionID)
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
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-lifecycle-read-parity-001"))
	waitForRuntimeSessionTerminal(t, service, started.SessionId)

	beforeRead := getDurableFactorySession(t, server.URL, started.SessionId)
	assertDurableSessionInspectionLinks(t, started.SessionId, beforeRead.Links)
	beforeResult := getDurableFactorySessionResult(t, server.URL, started.SessionId, "")
	beforeEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionId, "")

	_, pauseStatus := postFactorySessionLifecycleControl(t, server.URL, started.SessionId, "pause", nil)
	if pauseStatus != http.StatusConflict {
		t.Fatalf("pause on terminal session status = %d, want 409", pauseStatus)
	}

	afterRead := getDurableFactorySession(t, server.URL, started.SessionId)
	assertDurableSessionReadUnchanged(t, beforeRead, afterRead)
	assertDurableSessionInspectionLinks(t, started.SessionId, afterRead.Links)

	afterResult := getDurableFactorySessionResult(t, server.URL, started.SessionId, "")
	assertDurableSessionResultUnchanged(t, beforeResult, afterResult)

	afterEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionId, "")
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("event count = %d, want %d after rejected lifecycle control", len(afterEvents), len(beforeEvents))
	}
	assertEventReconnectStillWorks(t, server.URL, started.SessionId, beforeEvents)
}

func TestLifecycleControls_PreserveDispatchArtifactReadParity(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-lifecycle-dispatch-parity-001",
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

	beforeRead := getDurableFactorySession(t, server.URL, completed.SessionID)
	assertDurableSessionInspectionLinks(t, completed.SessionID, beforeRead.Links)
	beforeDispatchList := getDurableDispatchList(t, server.URL, completed.SessionID)
	beforeDispatchDetail := getDurableDispatchDetail(t, server.URL, completed.SessionID, "dispatch-1")
	beforeArtifactList := getDurableArtifactList(t, server.URL, completed.SessionID)

	_, pauseStatus := postFactorySessionLifecycleControl(t, server.URL, completed.SessionID, "pause", nil)
	if pauseStatus != http.StatusConflict {
		t.Fatalf("pause on terminal session status = %d, want 409", pauseStatus)
	}

	afterRead := getDurableFactorySession(t, server.URL, completed.SessionID)
	assertDurableSessionReadUnchanged(t, beforeRead, afterRead)
	assertDurableSessionInspectionLinks(t, completed.SessionID, afterRead.Links)

	afterDispatchList := getDurableDispatchList(t, server.URL, completed.SessionID)
	assertDispatchListUnchanged(t, beforeDispatchList, afterDispatchList)
	afterDispatchDetail := getDurableDispatchDetail(t, server.URL, completed.SessionID, "dispatch-1")
	assertDispatchDetailUnchanged(t, beforeDispatchDetail, afterDispatchDetail)
	afterArtifactList := getDurableArtifactList(t, server.URL, completed.SessionID)
	assertArtifactListUnchanged(t, beforeArtifactList, afterArtifactList)
}

func TestLifecycleControls_CancelPreservesReadSurfaces(t *testing.T) {
	service := newAPILifecycleRuntimeService(t, "busy-loop.workflow.js", "busy-loop")
	started := startRuntimeBackedDurableSession(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	beforeEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionID, "")

	_, cancelStatus := postFactorySessionLifecycleControl(t, server.URL, started.SessionID, "cancel", nil)
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("cancel status = %d, want 202", cancelStatus)
	}

	read := getDurableFactorySession(t, server.URL, started.SessionID)
	assertDurableSessionInspectionLinks(t, started.SessionID, read.Links)
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling &&
		read.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("status after cancel = %q, want CANCELING or CANCELED", read.Status)
	}

	getDurableFactorySessionResult(t, server.URL, started.SessionID, "")
	getFactorySessionList(t, server.URL, "persisted")

	afterEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionID, "")
	if len(afterEvents) < len(beforeEvents) {
		t.Fatalf("event count after cancel = %d, want at least %d", len(afterEvents), len(beforeEvents))
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
	allEvents []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(allEvents) == 0 {
		t.Fatal("expected at least one event for reconnect assertion")
	}
	firstID := allEvents[0].Id
	afterStart := getDurableFactorySessionEvents(
		t,
		serverURL,
		sessionID,
		"after_event_id="+firstID,
	)
	if len(afterStart) != len(allEvents)-1 {
		t.Fatalf("reconnect event count = %d, want %d", len(afterStart), len(allEvents)-1)
	}
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
