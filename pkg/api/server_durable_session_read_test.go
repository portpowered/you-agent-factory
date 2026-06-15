package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestListFactorySessions_RuntimeBackedIncludesLiveAndPersistedScopes(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newTestServer(&testutil.MockFactory{
		DurableExecutionService: service,
		FactorySessions: factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{
					Id:         "~default",
					FactoryDir: "/workspace/root",
					FolderPath: "/workspace/root",
					Project:    "root",
					IsDefault:  true,
				},
			},
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-runtime-list-live-001"))
	waitForRuntimeSessionTerminal(t, service, started.SessionId)

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
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
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
	if read.ResolvedSource.SourceRef == nil || *read.ResolvedSource.SourceRef != workflowsource.ProjectClaudeWorkflowsDir+"/simple-final.js" {
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

func TestGetFactorySession_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newTestServer(&testutil.MockFactory{DurableExecutionService: service})
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
	if errResp.Code != factoryapi.NOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestGetFactorySession_LivePetriSessionRemainsCompatible(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
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

func waitForRuntimeSessionTerminal(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		read, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession while waiting for terminal status: %v", err)
		}
		if read.Status == factorysessionexecution.LifecycleStatusSucceeded {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("session %q did not reach SUCCEEDED within timeout", sessionID)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}
