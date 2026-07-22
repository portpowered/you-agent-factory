package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestListFactorySessions_RuntimeBackedIncludesLiveAndPersistedScopes(t *testing.T) {
	const sessionID = "dur-sess-api-list-001"
	service := apiExecutionScript{
		startAsync: func(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.AsyncStartResult, error) {
			return factorysessionexecution.AsyncStartResult{
				SessionID: sessionID,
				Status:    string(factorysessionexecution.LifecycleStatusRunning),
			}, nil
		},
		listSessions: func(_ context.Context, request factorysessionexecution.ListSessionsRequest) (factorysessionexecution.ListSessionsResult, error) {
			if request.Scope != factorysessionexecution.SessionListScopeAll {
				t.Fatalf("service scope = %q, want all aggregation read", request.Scope)
			}
			return factorysessionexecution.ListSessionsResult{
				Scope: factorysessionexecution.SessionListScopeAll,
				LiveSessions: []factorysessionexecution.LiveSessionSummary{{
					ID: "~default", FactoryDir: "/workspace/root", FolderPath: "/workspace/root",
					Project: "root", IsDefault: true,
				}},
				DurableSessions: []factorysessionexecution.DurableSessionListSummary{{
					SessionID: sessionID,
					Status:    factorysessionexecution.LifecycleStatusSucceeded,
				}},
			}, nil
		},
	}
	srv := newDurableAndLiveAPITestServer(service, apiLiveSessionScript{
		list: func(context.Context) (factoryapi.ListFactorySessionsResponse, error) {
			return factoryapi.ListFactorySessionsResponse{
				Sessions: []factoryapi.FactorySessionSummary{
					{
						Id:         "~default",
						FactoryDir: "/workspace/root",
						FolderPath: "/workspace/root",
						Project:    "root",
						IsDefault:  true,
					},
				},
			}, nil
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-runtime-list-live-001"))

	liveList := getFactorySessionList(t, server.URL, "live")
	if !containsLiveSessionID(liveList.Sessions, "~default") {
		t.Fatalf("live list sessions = %#v, want workspace row ~default", liveList.Sessions)
	}
	if containsLiveSessionID(liveList.Sessions, started.SessionId) {
		t.Fatalf("live list sessions = %#v, should not include terminal durable row %q", liveList.Sessions, started.SessionId)
	}

	persistedList := getFactorySessionList(t, server.URL, "persisted")
	if !containsDurableSessionID(persistedList, started.SessionId) {
		t.Fatalf("persisted durableSessions = %#v, want %q", persistedList.DurableSessions, started.SessionId)
	}
}

func TestGetFactorySession_RuntimeBackedReturnsTerminalReadModel(t *testing.T) {
	const sessionID = "dur-sess-api-get-001"
	service := apiExecutionScript{
		startSync: func(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.SyncStartResult, error) {
			return factorysessionexecution.SyncStartResult{
				AsyncStartResult: factorysessionexecution.AsyncStartResult{
					SessionID: sessionID,
					Status:    string(factorysessionexecution.LifecycleStatusSucceeded),
				},
				SyncOutcome: factorysessionexecution.SyncOutcome("COMPLETED"),
			}, nil
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			return terminalAPIReadResult(sessionID), nil
		},
	}
	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-get-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("simple-final")
	syncBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal sync request: %v", err)
	}
	syncResp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", strings.NewReader(string(syncBody)))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer syncResp.Body.Close()
	if syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncResp.StatusCode, readBody(t, syncResp))
	}
	var syncResult factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(syncResp.Body).Decode(&syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if syncResult.SessionId == "" {
		t.Fatal("sync sessionId missing")
	}

	read := getDurableFactorySession(t, server.URL, syncResult.SessionId)
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", read.Status)
	}
	if read.ResolvedSource.SourceRef == nil || *read.ResolvedSource.SourceRef != factory.WorkflowSourceProjectClaudeWorkflowsDir+"/simple-final.js" {
		t.Fatalf("resolved source ref = %#v", read.ResolvedSource.SourceRef)
	}
	if read.ResultSummary == nil || read.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", read.ResultSummary)
	}
	if read.SourceHash == nil || *read.SourceHash == "" {
		t.Fatal("expected source hash on durable session read")
	}
	if read.EffectivePolicyHash == nil || *read.EffectivePolicyHash == "" {
		t.Fatal("expected effective policy hash on durable session read")
	}
}

func TestGetFactorySession_RuntimeBackedReturnsOrderedPhasesAndLatestCheckpoint(t *testing.T) {
	const sessionID = "dur-sess-api-introspection-001"
	service := apiExecutionScript{
		startSync: func(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.SyncStartResult, error) {
			return factorysessionexecution.SyncStartResult{
				AsyncStartResult: factorysessionexecution.AsyncStartResult{
					SessionID: sessionID,
					Status:    string(factorysessionexecution.LifecycleStatusSucceeded),
				},
				SyncOutcome: factorysessionexecution.SyncOutcome("COMPLETED"),
			}, nil
		},
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			result := terminalAPIReadResult(sessionID)
			result.PhaseSummaries = []factorysessionexecution.PhaseSummary{
				{Phase: "setup"},
				{Phase: "execute"},
			}
			result.LatestCheckpoint = &factorysessionexecution.CheckpointRef{
				ID:    "checkpoint-after-artifact",
				Label: "after-artifact",
				Phase: "execute",
			}
			return result, nil
		},
	}
	server := httptest.NewServer(newDurableAPITestServer(service).Handler())
	defer server.Close()

	request := runtimeBackedAsyncStartRequest("req-api-runtime-introspection-001")
	request.Source.Kind = factoryapi.FactorySessionExecutionSourceKindWorkflowName
	request.Source.WorkflowName = strPtr("progress-primitives")
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(server.URL+"/factory-sessions/sync", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /factory-sessions/sync: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&started); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}

	read := getDurableFactorySession(t, server.URL, started.SessionId)
	if read.PhaseSummaries == nil || len(*read.PhaseSummaries) != 2 || (*read.PhaseSummaries)[0].Phase != "setup" || (*read.PhaseSummaries)[1].Phase != "execute" {
		t.Fatalf("phaseSummaries = %#v, want ordered setup, execute", read.PhaseSummaries)
	}
	if read.LatestCheckpoint == nil || read.LatestCheckpoint.Id == "" || read.LatestCheckpoint.Label == nil || *read.LatestCheckpoint.Label != "after-artifact" {
		t.Fatalf("latestCheckpoint = %#v, want stable after-artifact checkpoint", read.LatestCheckpoint)
	}
	if read.LatestCheckpoint.Phase == nil || *read.LatestCheckpoint.Phase != "execute" {
		t.Fatalf("latestCheckpoint.phase = %#v, want execute", read.LatestCheckpoint.Phase)
	}
}

func TestGetFactorySession_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	service := apiExecutionScript{
		getSession: func(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
			return factorysessionexecution.SessionReadResult{}, factorysessionexecution.ErrDurableSessionNotFound
		},
	}
	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-001")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestGetFactorySession_LivePetriSessionRemainsCompatible(t *testing.T) {
	srv := newDurableAndLiveAPITestServer(nil, apiLiveSessionScript{
		get: func(context.Context, string) (factoryapi.FactorySession, error) {
			return factoryapi.FactorySession{
				Id:         "session-beta",
				FactoryDir: "/workspace/root/beta",
				FolderPath: "/workspace/root",
				Project:    "beta",
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySession
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode live session response: %v", err)
	}
	if response.Id != "session-beta" {
		t.Fatalf("id = %q, want session-beta", response.Id)
	}
}

func getFactorySessionList(t *testing.T, serverURL, scope string) factoryapi.ListFactorySessionsResponse {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions?scope=" + scope)
	if err != nil {
		t.Fatalf("GET /factory-sessions?scope=%s: %v", scope, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.ListFactorySessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return response
}

func getDurableFactorySession(t *testing.T, serverURL, sessionID string) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s: %v", sessionID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.FactorySessionDurableReadModel
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode durable session response: %v", err)
	}
	return response
}

func containsLiveSessionID(sessions []factoryapi.FactorySessionSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.Id == sessionID {
			return true
		}
	}
	return false
}

func containsDurableSessionID(response factoryapi.ListFactorySessionsResponse, sessionID string) bool {
	if response.DurableSessions == nil {
		return false
	}
	for _, row := range *response.DurableSessions {
		if row.SessionId == sessionID {
			return true
		}
	}
	return false
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}

func serverURLForLifecycle(t *testing.T, service factorysessionexecution.ExecutionService) string {
	t.Helper()
	srv := newDurableAPITestServer(service)
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	return server.URL
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
