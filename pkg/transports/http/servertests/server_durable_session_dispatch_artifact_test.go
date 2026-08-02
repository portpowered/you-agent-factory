package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

const (
	dispatchArtifactSessionID = "dur-sess-api-dispatch-artifact-001"
	apiDispatchID             = "dispatch-1"
	apiArtifactID             = "artifact-1"
)

func TestListFactorySessionDispatches_RuntimeBackedReturnsTypedEmptyList(t *testing.T) {
	service := apiExecutionScript{
		listDispatches: func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
			return factorysessions.ListDispatchesResult{
				SessionID:  dispatchArtifactSessionID,
				Dispatches: []factorysessions.DispatchSummary{},
			}, nil
		},
	}
	server := durableHTTPServer(t, service)
	response := getDurableDispatchList(t, server, dispatchArtifactSessionID)
	if response.SessionId != dispatchArtifactSessionID || response.Dispatches == nil || len(response.Dispatches) != 0 {
		t.Fatalf("response = %#v, want typed empty dispatch list", response)
	}
}

func TestListFactorySessionDispatches_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		listDispatches: func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
			return factorysessions.ListDispatchesResult{}, factorysessions.ErrDurableSessionNotFound
		},
	}
	assertHTTPGetStatus(t, durableHTTPServer(t, service)+"/factory-sessions/dur-sess-missing/dispatches", http.StatusNotFound)
}

func TestListFactorySessionDispatches_LivePetriSessionReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(newDurableAPITestServer(nil).Handler())
	t.Cleanup(server.Close)
	assertHTTPGetStatus(t, server.URL+"/factory-sessions/session-beta/dispatches", http.StatusNotFound)
}

func TestGetFactorySessionDispatch_RuntimeBackedReturnsTypedDetail(t *testing.T) {
	service := apiExecutionScript{
		queryDispatches: func(_ context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
			if request.SessionID != dispatchArtifactSessionID {
				t.Fatalf("query sessionID = %q", request.SessionID)
			}
			switch {
			case request.Filters.Phase == "unknown" && request.Filters.Status == "":
				return factorysessions.ListDispatchesResult{SessionID: dispatchArtifactSessionID, Dispatches: []factorysessions.DispatchSummary{}}, nil
			case request.Filters.Status == "BROKEN":
				return factorysessions.ListDispatchesResult{}, &factorysessions.ExecutionValidationError{Field: "status", Message: "invalid status"}
			default:
				t.Fatalf("unexpected dispatch query = %#v", request)
				return factorysessions.ListDispatchesResult{}, nil
			}
		},
		getDispatch: func(context.Context, string, string) (factorysessions.DispatchDetail, error) {
			return apiDispatchDetail(), nil
		},
	}
	server := durableHTTPServer(t, service)
	assertRuntimeDispatchFilters(t, server, dispatchArtifactSessionID)
	response := getDurableDispatchDetail(t, server, dispatchArtifactSessionID, apiDispatchID)
	if response.Id != apiDispatchID || response.SessionId != dispatchArtifactSessionID ||
		response.OrchestratorKind != factoryapi.JAVASCRIPT ||
		response.Label == nil || *response.Label != "summarize-findings" {
		t.Fatalf("response = %#v, want typed JavaScript dispatch detail", response)
	}
}

func assertRuntimeDispatchFilters(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	filtered := getDispatchListAt(t, serverURL+"/factory-sessions/"+sessionID+"/dispatches?phase=unknown")
	if filtered.Dispatches == nil || len(filtered.Dispatches) != 0 {
		t.Fatalf("filtered dispatches = %#v, want non-nil empty collection", filtered.Dispatches)
	}
	assertHTTPGetStatus(t, serverURL+"/factory-sessions/"+sessionID+"/dispatches?status=BROKEN", http.StatusBadRequest)
}

func TestGetFactorySessionDispatch_RuntimeBackedUnknownDispatchReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		getDispatch: func(context.Context, string, string) (factorysessions.DispatchDetail, error) {
			return factorysessions.DispatchDetail{}, factorysessions.ErrDispatchNotFound
		},
	}
	assertHTTPGetStatus(
		t,
		durableHTTPServer(t, service)+"/factory-sessions/"+dispatchArtifactSessionID+"/dispatches/missing",
		http.StatusNotFound,
	)
}

func TestGetFactorySessionDispatch_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		getDispatch: func(context.Context, string, string) (factorysessions.DispatchDetail, error) {
			return factorysessions.DispatchDetail{}, factorysessions.ErrDurableSessionNotFound
		},
	}
	assertHTTPGetStatus(
		t,
		durableHTTPServer(t, service)+"/factory-sessions/dur-sess-missing/dispatches/"+apiDispatchID,
		http.StatusNotFound,
	)
}

func TestGetFactorySessionDispatch_LivePetriSessionReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(newDurableAPITestServer(nil).Handler())
	t.Cleanup(server.Close)
	assertHTTPGetStatus(t, server.URL+"/factory-sessions/session-beta/dispatches/"+apiDispatchID, http.StatusNotFound)
}

func TestListFactorySessionArtifacts_RuntimeBackedReturnsTypedEmptyList(t *testing.T) {
	service := apiExecutionScript{
		listArtifacts: func(context.Context, string) (factorysessions.ListArtifactsResult, error) {
			return factorysessions.ListArtifactsResult{
				SessionID: dispatchArtifactSessionID,
				Artifacts: []factorysessions.ArtifactSummary{},
			}, nil
		},
	}
	response := getDurableArtifactList(t, durableHTTPServer(t, service), dispatchArtifactSessionID)
	if response.SessionId != dispatchArtifactSessionID || response.Artifacts == nil || len(response.Artifacts) != 0 {
		t.Fatalf("response = %#v, want typed empty artifact list", response)
	}
}

func TestListFactorySessionArtifacts_RuntimeBackedReturnsTypedArtifactList(t *testing.T) {
	service := apiExecutionScript{
		listArtifacts: func(context.Context, string) (factorysessions.ListArtifactsResult, error) {
			return apiArtifactList(), nil
		},
	}
	response := getDurableArtifactList(t, durableHTTPServer(t, service), dispatchArtifactSessionID)
	if len(response.Artifacts) != 1 || response.Artifacts[0].Id != apiArtifactID ||
		response.Artifacts[0].Kind != factoryapi.FactoryArtifactKindFINALRESULT {
		t.Fatalf("response = %#v, want typed workflow artifact", response)
	}
}

func TestListFactorySessionArtifacts_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		listArtifacts: func(context.Context, string) (factorysessions.ListArtifactsResult, error) {
			return factorysessions.ListArtifactsResult{}, factorysessions.ErrDurableSessionNotFound
		},
	}
	assertHTTPGetStatus(t, durableHTTPServer(t, service)+"/factory-sessions/dur-sess-missing/artifacts", http.StatusNotFound)
}

func TestListFactorySessionArtifacts_LivePetriSessionReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(newDurableAPITestServer(nil).Handler())
	t.Cleanup(server.Close)
	assertHTTPGetStatus(t, server.URL+"/factory-sessions/session-beta/artifacts", http.StatusNotFound)
}

func TestGetFactorySessionArtifact_RuntimeBackedReturnsTypedDetail(t *testing.T) {
	service := apiExecutionScript{
		getArtifact: func(context.Context, string, string) (factorysessions.ArtifactDetail, error) {
			return apiArtifactDetail(), nil
		},
	}
	response := getDurableArtifactDetail(t, durableHTTPServer(t, service), dispatchArtifactSessionID, apiArtifactID)
	if response.Id != apiArtifactID || response.SessionId != dispatchArtifactSessionID ||
		response.Kind != factoryapi.FactoryArtifactKindFINALRESULT ||
		response.Content == nil {
		t.Fatalf("response = %#v, want typed artifact detail with content", response)
	}
}

func TestGetFactorySessionArtifact_RuntimeBackedUnknownArtifactReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		getArtifact: func(context.Context, string, string) (factorysessions.ArtifactDetail, error) {
			return factorysessions.ArtifactDetail{}, factorysessions.ErrArtifactNotFound
		},
	}
	assertHTTPGetStatus(
		t,
		durableHTTPServer(t, service)+"/factory-sessions/"+dispatchArtifactSessionID+"/artifacts/missing",
		http.StatusNotFound,
	)
}

func TestGetFactorySessionArtifact_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		getArtifact: func(context.Context, string, string) (factorysessions.ArtifactDetail, error) {
			return factorysessions.ArtifactDetail{}, factorysessions.ErrDurableSessionNotFound
		},
	}
	assertHTTPGetStatus(
		t,
		durableHTTPServer(t, service)+"/factory-sessions/dur-sess-missing/artifacts/"+apiArtifactID,
		http.StatusNotFound,
	)
}

func TestGetFactorySessionArtifact_LivePetriSessionReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(newDurableAPITestServer(nil).Handler())
	t.Cleanup(server.Close)
	assertHTTPGetStatus(t, server.URL+"/factory-sessions/session-beta/artifacts/"+apiArtifactID, http.StatusNotFound)
}

func TestDispatchArtifactReads_PreserveSessionParityWithoutMutation(t *testing.T) {
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			return terminalAPIReadResult(dispatchArtifactSessionID), nil
		},
		getResult: func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			return finalAPIResult(dispatchArtifactSessionID), nil
		},
		listSessions: func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			return factorysessions.ListSessionsResult{
				Scope: factorysessions.SessionListScopeAll,
				DurableSessions: []factorysessions.DurableSessionListSummary{{
					SessionID: dispatchArtifactSessionID,
					Status:    factorysessions.LifecycleStatusSucceeded,
				}},
			}, nil
		},
		listDispatches: func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
			return apiDispatchList(), nil
		},
		getDispatch: func(context.Context, string, string) (factorysessions.DispatchDetail, error) {
			return factorysessions.DispatchDetail{}, factorysessions.ErrDispatchNotFound
		},
		listArtifacts: func(context.Context, string) (factorysessions.ListArtifactsResult, error) {
			return apiArtifactList(), nil
		},
		getArtifact: func(context.Context, string, string) (factorysessions.ArtifactDetail, error) {
			return factorysessions.ArtifactDetail{}, factorysessions.ErrArtifactNotFound
		},
		readEvents: func(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
			return apiTerminalEvents(dispatchArtifactSessionID), nil
		},
	}
	serverURL := durableHTTPServer(t, service)
	beforeList := getFactorySessionList(t, serverURL, "persisted")
	beforeRead := getDurableFactorySession(t, serverURL, dispatchArtifactSessionID)
	beforeResult := getDurableFactorySessionResult(t, serverURL, dispatchArtifactSessionID, "")
	beforeEvents := getDurableFactorySessionEvents(t, serverURL, dispatchArtifactSessionID, "")
	assertDispatchArtifactReadsDoNotCreateSessions(t, serverURL, dispatchArtifactSessionID)
	afterList := getFactorySessionList(t, serverURL, "persisted")
	if len(*afterList.DurableSessions) != len(*beforeList.DurableSessions) {
		t.Fatalf("durable session count changed after reads")
	}
	assertDurableSessionReadUnchanged(t, beforeRead, getDurableFactorySession(t, serverURL, dispatchArtifactSessionID))
	assertDurableSessionResultUnchanged(t, beforeResult, getDurableFactorySessionResult(t, serverURL, dispatchArtifactSessionID, ""))
	afterEvents := getDurableFactorySessionEvents(t, serverURL, dispatchArtifactSessionID, "")
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("event count changed after reads")
	}
}

func assertDispatchArtifactReadsDoNotCreateSessions(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	for _, path := range []string{
		"/factory-sessions/" + sessionID + "/dispatches",
		"/factory-sessions/" + sessionID + "/dispatches/dispatch-missing-001",
		"/factory-sessions/" + sessionID + "/artifacts",
		"/factory-sessions/" + sessionID + "/artifacts/artifact-missing-001",
	} {
		resp, err := http.Get(serverURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 200 or 404", path, resp.StatusCode)
		}
	}
}

func apiDispatchList() factorysessions.ListDispatchesResult {
	return factorysessions.ListDispatchesResult{
		SessionID: dispatchArtifactSessionID,
		Dispatches: []factorysessions.DispatchSummary{{
			ID:           apiDispatchID,
			Status:       factorysessions.DispatchStatus("COMPLETED"),
			DispatchKind: "JAVASCRIPT_AGENT",
			Phase:        "execute",
			Label:        "summarize-findings",
			Attempt:      1,
		}},
	}
}

func apiDispatchDetail() factorysessions.DispatchDetail {
	return factorysessions.DispatchDetail{
		DispatchSummary:  apiDispatchList().Dispatches[0],
		SessionID:        dispatchArtifactSessionID,
		OrchestratorKind: "JAVASCRIPT",
		StatusTransitions: []factorysessions.DispatchStatus{
			"QUEUED", "RUNNING", "COMPLETED",
		},
	}
}

func apiArtifactList() factorysessions.ListArtifactsResult {
	return factorysessions.ListArtifactsResult{
		SessionID: dispatchArtifactSessionID,
		Artifacts: []factorysessions.ArtifactSummary{{
			ID:         apiArtifactID,
			Kind:       "FINAL_RESULT",
			Visibility: "SESSION",
			Label:      "output",
		}},
	}
}

func apiArtifactDetail() factorysessions.ArtifactDetail {
	return factorysessions.ArtifactDetail{
		ArtifactSummary: apiArtifactList().Artifacts[0],
		SessionID:       dispatchArtifactSessionID,
		Content:         json.RawMessage(`[{"type":"text","text":"complete"}]`),
	}
}

func durableHTTPServer(t *testing.T, service factorysessionmapping.DurableExecution) string {
	t.Helper()
	return durableRoleHTTPServer(t, service)
}

func assertHTTPGetStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("GET %s status = %d, want %d: %s", url, resp.StatusCode, want, readBody(t, resp))
	}
}

func getDispatchListAt(t *testing.T, url string) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200: %s", url, resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.ListFactorySessionDispatchesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode dispatch list: %v", err)
	}
	return response
}

func assertDurableSessionReadUnchanged(
	t *testing.T,
	before, after factoryapi.FactorySessionDurableReadModel,
) {
	t.Helper()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("marshal before session read: %v", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("marshal after session read: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("session read changed: before=%s after=%s", beforeJSON, afterJSON)
	}
}

func assertDurableSessionResultUnchanged(
	t *testing.T,
	before, after factoryapi.FactorySessionResult,
) {
	t.Helper()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("marshal before result: %v", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("marshal after result: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("result read changed: before=%s after=%s", beforeJSON, afterJSON)
	}
}
