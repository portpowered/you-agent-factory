package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestListFactorySessionDispatches_RuntimeBackedReturnsTypedEmptyList(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-dispatch-list-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "api",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForRuntimeSessionTerminal(t, service, started.SessionID)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + started.SessionID + "/dispatches")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var response factoryapi.ListFactorySessionDispatchesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode dispatch list response: %v", err)
	}
	if response.SessionId != started.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.SessionId, started.SessionID)
	}
	if response.Dispatches == nil {
		t.Fatal("dispatches = nil, want empty typed list")
	}
	if len(response.Dispatches) != 0 {
		t.Fatalf("dispatches = %#v, want empty list for simple-final runtime session", response.Dispatches)
	}
}

func TestListFactorySessionDispatches_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-dispatch-001/dispatches")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches: %v", err)
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

func TestListFactorySessionDispatches_LivePetriSessionReturnsNotFound(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/session-beta/dispatches")
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/dispatches: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestGetFactorySessionDispatch_RuntimeBackedReturnsTypedDetail(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-dispatch-detail-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	assertRuntimeDispatchFilters(t, server.URL, completed.SessionID)

	resp, err := http.Get(server.URL + "/factory-sessions/" + completed.SessionID + "/dispatches/dispatch-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches/{dispatch_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var response factoryapi.FactoryDispatch
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode dispatch detail response: %v", err)
	}
	if response.Id != "dispatch-1" {
		t.Fatalf("id = %q, want dispatch-1", response.Id)
	}
	if response.SessionId != completed.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.SessionId, completed.SessionID)
	}
	if response.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", response.OrchestratorKind)
	}
	if response.Label == nil || *response.Label != "summarize-findings" {
		t.Fatalf("label = %#v, want summarize-findings", response.Label)
	}
}

func assertRuntimeDispatchFilters(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	filteredResp, err := http.Get(serverURL + "/factory-sessions/" + sessionID + "/dispatches?phase=unknown")
	if err != nil {
		t.Fatalf("GET filtered dispatches: %v", err)
	}
	var filtered factoryapi.ListFactorySessionDispatchesResponse
	if filteredResp.StatusCode != http.StatusOK {
		filteredResp.Body.Close()
		t.Fatalf("filtered status = %d, want 200", filteredResp.StatusCode)
	}
	if err := json.NewDecoder(filteredResp.Body).Decode(&filtered); err != nil {
		filteredResp.Body.Close()
		t.Fatalf("decode filtered dispatches: %v", err)
	}
	filteredResp.Body.Close()
	if filtered.Dispatches == nil || len(filtered.Dispatches) != 0 {
		t.Fatalf("filtered dispatches = %#v, want non-nil empty collection", filtered.Dispatches)
	}

	invalidResp, err := http.Get(serverURL + "/factory-sessions/" + sessionID + "/dispatches?status=BROKEN")
	if err != nil {
		t.Fatalf("GET invalid status dispatches: %v", err)
	}
	invalidResp.Body.Close()
	if invalidResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want 400", invalidResp.StatusCode)
	}
}

func TestGetFactorySessionDispatch_RuntimeBackedUnknownDispatchReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-dispatch-detail-missing-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "api",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForRuntimeSessionTerminal(t, service, started.SessionID)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + started.SessionID + "/dispatches/dispatch-missing-001")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches/{dispatch_id}: %v", err)
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

func TestGetFactorySessionDispatch_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-dispatch-detail-001/dispatches/dispatch-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches/{dispatch_id}: %v", err)
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

func TestGetFactorySessionDispatch_LivePetriSessionReturnsNotFound(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/session-beta/dispatches/dispatch-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/dispatches/dispatch-1: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestListFactorySessionArtifacts_RuntimeBackedReturnsTypedEmptyList(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-artifact-list-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "api",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForRuntimeSessionTerminal(t, service, started.SessionID)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + started.SessionID + "/artifacts")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var response factoryapi.ListFactorySessionArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact list response: %v", err)
	}
	if response.SessionId != started.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.SessionId, started.SessionID)
	}
	if response.Artifacts == nil {
		t.Fatal("artifacts = nil, want empty typed list")
	}
	if len(response.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want empty list for simple-final runtime session", response.Artifacts)
	}
}

func TestListFactorySessionArtifacts_RuntimeBackedReturnsTypedArtifactList(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "progress-primitives.workflow.js", "progress-primitives")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-artifact-list-002",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "progress-primitives",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + completed.SessionID + "/artifacts")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var response factoryapi.ListFactorySessionArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact list response: %v", err)
	}
	if response.SessionId != completed.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.SessionId, completed.SessionID)
	}
	if len(response.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one artifact", response.Artifacts)
	}
	artifact := response.Artifacts[0]
	if artifact.Id != "artifact-1" || string(artifact.Kind) != "log" {
		t.Fatalf("artifact = %#v, want artifact-1 log", artifact)
	}
	wantHref := "/factory-sessions/" + completed.SessionID + "/artifacts/artifact-1"
	if artifact.RetrievalRef == nil || artifact.RetrievalRef.Href != wantHref {
		t.Fatalf("retrievalRef = %#v, want %q", artifact.RetrievalRef, wantHref)
	}
}

func TestListFactorySessionArtifacts_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-artifact-001/artifacts")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts: %v", err)
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

func TestListFactorySessionArtifacts_LivePetriSessionReturnsNotFound(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/session-beta/artifacts")
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestGetFactorySessionArtifact_RuntimeBackedReturnsTypedDetail(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "progress-primitives.workflow.js", "progress-primitives")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-artifact-detail-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "progress-primitives",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + completed.SessionID + "/artifacts/artifact-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts/{artifact_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var response factoryapi.FactorySessionArtifactDetail
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact detail response: %v", err)
	}
	if response.Id != "artifact-1" || string(response.Kind) != "log" {
		t.Fatalf("artifact = %#v, want artifact-1 log", response)
	}
	if response.SessionId != completed.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.SessionId, completed.SessionID)
	}
	if response.Label == nil || *response.Label != "step-output" {
		t.Fatalf("label = %#v, want step-output", response.Label)
	}
	wantHref := "/factory-sessions/" + completed.SessionID + "/artifacts/artifact-1"
	if response.ContentRef == nil || response.ContentRef.Href != wantHref {
		t.Fatalf("contentRef = %#v, want %q", response.ContentRef, wantHref)
	}
}

func TestGetFactorySessionArtifact_RuntimeBackedUnknownArtifactReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-artifact-detail-missing-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "api",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForRuntimeSessionTerminal(t, service, started.SessionID)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + started.SessionID + "/artifacts/artifact-missing-001")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts/{artifact_id}: %v", err)
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

func TestGetFactorySessionArtifact_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-artifact-detail-001/artifacts/artifact-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts/{artifact_id}: %v", err)
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

func TestGetFactorySessionArtifact_LivePetriSessionReturnsNotFound(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/session-beta/artifacts/artifact-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/artifacts/artifact-1: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestDispatchArtifactReads_PreserveSessionParityWithoutMutation(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := newAPIJavaScriptRuntimeService(t, projectRoot, factorysessionexecution.ChildExecutorModeFake, nil)
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	started := postDurableAsyncStart(t, server.URL, runtimeBackedAsyncStartRequest("req-api-runtime-parity-001"))
	waitForRuntimeSessionTerminal(t, service, started.SessionId)

	beforeList := getFactorySessionList(t, server.URL, "persisted")
	beforeRead := getDurableFactorySession(t, server.URL, started.SessionId)
	beforeResult := getDurableFactorySessionResult(t, server.URL, started.SessionId, "")
	beforeEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionId, "")

	assertDispatchArtifactReadsDoNotCreateSessions(t, server.URL, started.SessionId)

	afterList := getFactorySessionList(t, server.URL, "persisted")
	if len(*afterList.DurableSessions) != len(*beforeList.DurableSessions) {
		t.Fatalf("persisted durable session count = %d, want %d after dispatch/artifact reads",
			len(*afterList.DurableSessions), len(*beforeList.DurableSessions))
	}

	afterRead := getDurableFactorySession(t, server.URL, started.SessionId)
	assertDurableSessionReadUnchanged(t, beforeRead, afterRead)

	afterResult := getDurableFactorySessionResult(t, server.URL, started.SessionId, "")
	assertDurableSessionResultUnchanged(t, beforeResult, afterResult)

	afterEvents := getDurableFactorySessionEvents(t, server.URL, started.SessionId, "")
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("event count = %d, want %d after dispatch/artifact reads", len(afterEvents), len(beforeEvents))
	}
}

func assertDispatchArtifactReadsDoNotCreateSessions(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	paths := []string{
		"/factory-sessions/" + sessionID + "/dispatches",
		"/factory-sessions/" + sessionID + "/dispatches/dispatch-missing-001",
		"/factory-sessions/" + sessionID + "/artifacts",
		"/factory-sessions/" + sessionID + "/artifacts/artifact-missing-001",
	}
	for _, path := range paths {
		resp, err := http.Get(serverURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 200 or 404: %s", path, resp.StatusCode, readBody(t, resp))
		}
	}
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
