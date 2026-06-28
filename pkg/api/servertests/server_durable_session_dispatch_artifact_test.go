package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestListFactorySessionDispatches_RuntimeBackedReturnsTypedEmptyList(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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

func TestGetFactorySessionDispatch_RuntimeBackedUnknownDispatchReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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
func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsQueuedRunningCompletedPath(t *testing.T) {
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
	defer server.Close()

	sessionRead := getDurableFactorySession(t, server.URL, completed.SessionID)
	if sessionRead.Progress == nil ||
		sessionRead.Progress.TotalDispatches == nil || *sessionRead.Progress.TotalDispatches != 1 ||
		sessionRead.Progress.CompletedDispatches == nil || *sessionRead.Progress.CompletedDispatches != 1 {
		t.Fatalf("session progress = %#v, want one completed dispatch", sessionRead.Progress)
	}

	dispatchList := getDurableDispatchList(t, server.URL, completed.SessionID)
	if len(dispatchList.Dispatches) != 1 {
		t.Fatalf("dispatch list = %#v, want one dispatch", dispatchList.Dispatches)
	}
	dispatchSummary := dispatchList.Dispatches[0]
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

	dispatchDetail := getDurableDispatchDetail(t, server.URL, completed.SessionID, "dispatch-1")
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

	assertAPILiveProviderProviderSessionRef(t, dispatchSummary.ProviderSessionRefs)
	assertAPILiveProviderArtifactLineage(t, server.URL, completed.SessionID, dispatchSummary, dispatchDetail)

	events := getDurableFactorySessionEvents(t, server.URL, completed.SessionID, "")
	assertAPILiveProviderDispatchLifecycleEventsAlignWithReads(
		t,
		events,
		"dispatch-1",
		dispatchSummary,
		dispatchDetail,
	)
}

func TestFakeChildDurableSessionReads_APIPreservesShippedTransportSemantics(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-fake-child-transport-regression-001",
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

	before := captureDurableSessionInspectionSnapshot(t, server.URL, completed.SessionID)
	assertAPIFakeChildInspectionSnapshot(t, before)

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
	assertPostControlEventsAlignWithStatus(t, completed.SessionID, after.events, after.read.Status)

	dispatchDetail := getDurableDispatchDetail(t, server.URL, completed.SessionID, "dispatch-1")
	assertAPIFakeChildDispatchDetail(t, dispatchDetail)
	assertAPIInspectionResponsesExcludeLiveProviderMarkers(t, before, dispatchDetail)
}

func TestSimpleFinalDurableSessionReads_APIPreservesFinalResultWithoutChildDispatches(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-simple-final-transport-regression-001",
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
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	snapshot := captureDurableSessionInspectionSnapshot(t, server.URL, completed.SessionID)
	if snapshot.read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", snapshot.read.Status)
	}
	if snapshot.result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", snapshot.result.ResultStatus)
	}
	if len(snapshot.dispatches.Dispatches) != 0 {
		t.Fatalf("dispatch list = %#v, want empty for simple-final", snapshot.dispatches.Dispatches)
	}
	if len(snapshot.artifacts.Artifacts) != 0 {
		t.Fatalf("artifact list = %#v, want empty for simple-final", snapshot.artifacts.Artifacts)
	}
	assertPostControlEventsAlignWithStatus(t, completed.SessionID, snapshot.events, snapshot.read.Status)
	assertAPIInspectionResponsesExcludeLiveProviderMarkers(t, snapshot, factoryapi.FactoryDispatch{})
}

func TestLiveProviderAndFakeChildSessions_APIPreserveDistinctProviderAndArtifactProjections(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")

	fakeService := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
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

	liveService := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		Provider:          factorysessionexecution.SmokeLiveChildProvider(),
	})
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

func TestLiveProviderChildDispatch_RuntimeBackedAPIProjectsFailedBridgedChildWithTypedFailureDetail(t *testing.T) {
	service := newAPILifecycleFailingChildRuntimeService(t)
	sessionID, dispatchID := startRuntimeBackedFailedSessionWithDispatch(t, service)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	snapshot := captureDurableSessionInspectionSnapshot(t, server.URL, sessionID)

	if snapshot.read.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", snapshot.read.Status)
	}
	if snapshot.read.Progress == nil ||
		snapshot.read.Progress.TotalDispatches == nil || *snapshot.read.Progress.TotalDispatches != 1 ||
		snapshot.read.Progress.FailedDispatches == nil || *snapshot.read.Progress.FailedDispatches != 1 {
		t.Fatalf("session progress = %#v, want one failed dispatch", snapshot.read.Progress)
	}
	if snapshot.read.Failure == nil || snapshot.read.Failure.Reason == nil ||
		*snapshot.read.Failure.Reason == "" {
		t.Fatalf("session failure = %#v, want typed workflow failure", snapshot.read.Failure)
	}

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

	dispatchDetail := getDurableDispatchDetail(t, server.URL, sessionID, dispatchID)
	if dispatchDetail.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch detail status = %q, want FAILED", dispatchDetail.Status)
	}
	if dispatchDetail.Javascript == nil || dispatchDetail.Javascript.ExecutionMode == nil ||
		*dispatchDetail.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeLive {
		t.Fatalf("dispatch detail executionMode = %#v, want live-provider", dispatchDetail.Javascript)
	}
	assertAPILiveProviderDispatchFailureDetail(t, dispatchDetail.FailureDetail)
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

	artifactList := getDurableArtifactList(t, server.URL, sessionID)
	if len(artifactList.Artifacts) != 0 {
		t.Fatalf("artifact list = %#v, want none for failed child", artifactList.Artifacts)
	}
	if snapshot.read.ArtifactRefs != nil && len(*snapshot.read.ArtifactRefs) != 0 {
		t.Fatalf("session artifactRefs = %#v, want none for failed child", snapshot.read.ArtifactRefs)
	}

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

func newAPILiveProviderRuntimeService(t *testing.T) factorysessionexecution.Service {
	t.Helper()
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	return factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		Provider:          factorysessionexecution.SmokeLiveChildProvider(),
	})
}

func newAPILiveProviderBlockingRuntimeService(t *testing.T) (
	*factorysessionexecution.JavaScriptRuntimeService,
	*apiLiveProviderBlockingFixtureProvider,
) {
	t.Helper()
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	provider := &apiLiveProviderBlockingFixtureProvider{}
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		Provider:          provider,
	})
	return service, provider
}

type apiLiveProviderBlockingFixtureProvider struct {
	mu           sync.Mutex
	inferStarted chan struct{}
	release      chan struct{}
}

func (p *apiLiveProviderBlockingFixtureProvider) Infer(ctx context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	if p.inferStarted == nil {
		p.inferStarted = make(chan struct{})
	}
	if p.release == nil {
		p.release = make(chan struct{})
	}
	started := p.inferStarted
	release := p.release
	p.mu.Unlock()

	close(started)
	select {
	case <-ctx.Done():
		return interfaces.InferenceResponse{}, ctx.Err()
	case <-release:
		return interfaces.InferenceResponse{
			Content: `{"text":"live:agent-run-fake-child:summarize-findings:summarize workflows:workflows"}`,
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}, nil
	}
}

func (p *apiLiveProviderBlockingFixtureProvider) waitForInferStart(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	if p.inferStarted == nil {
		p.inferStarted = make(chan struct{})
	}
	started := p.inferStarted
	p.mu.Unlock()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider Infer did not start before timeout")
	}
}

func (p *apiLiveProviderBlockingFixtureProvider) releaseInfer() {
	p.mu.Lock()
	if p.release == nil {
		p.release = make(chan struct{})
	}
	release := p.release
	p.mu.Unlock()
	close(release)
}

func waitForAPIDispatchStatus(
	t *testing.T,
	serverURL, sessionID, dispatchID string,
	want factoryapi.FactoryDispatchStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dispatchList := getDurableDispatchList(t, serverURL, sessionID)
		for _, dispatch := range dispatchList.Dispatches {
			if dispatch.Id == dispatchID && dispatch.Status == want {
				return dispatch
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("dispatch %q did not reach status %q before timeout", dispatchID, want)
	return factoryapi.FactorySessionDispatchSummary{}
}

func assertAPILiveProviderProviderSessionRef(
	t *testing.T,
	refs *[]factoryapi.LoadableProviderSessionRef,
) {
	t.Helper()
	if refs == nil || len(*refs) != 1 {
		t.Fatalf("providerSessionRefs = %#v, want one ref", refs)
	}
	ref := (*refs)[0]
	if ref.Id != "live-provider-session-1" {
		t.Fatalf("providerSessionRef id = %q, want live-provider-session-1", ref.Id)
	}
	if ref.Provider != "mock" {
		t.Fatalf("providerSessionRef provider = %q, want mock", ref.Provider)
	}
	if ref.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("providerSessionRef kind = %q, want session_id", ref.Kind)
	}
}

func assertAPILiveProviderArtifactLineage(
	t *testing.T,
	serverURL, sessionID string,
	dispatchSummary factoryapi.FactorySessionDispatchSummary,
	dispatchDetail factoryapi.FactoryDispatch,
) {
	t.Helper()

	sessionRead := getDurableFactorySession(t, serverURL, sessionID)
	if sessionRead.ArtifactRefs == nil || len(*sessionRead.ArtifactRefs) != 1 ||
		(*sessionRead.ArtifactRefs)[0].Id != "child-artifact-1" {
		t.Fatalf("session artifactRefs = %#v, want child-artifact-1", sessionRead.ArtifactRefs)
	}

	artifactList := getDurableArtifactList(t, serverURL, sessionID)
	if len(artifactList.Artifacts) != 1 {
		t.Fatalf("artifact list = %#v, want one artifact", artifactList.Artifacts)
	}
	artifactSummary := artifactList.Artifacts[0]
	if artifactSummary.Id != "child-artifact-1" {
		t.Fatalf("artifact id = %q, want child-artifact-1", artifactSummary.Id)
	}
	if artifactSummary.Kind != factoryapi.FactoryArtifactKindCHILDRESULT {
		t.Fatalf("artifact kind = %q, want CHILD_RESULT", artifactSummary.Kind)
	}
	if artifactSummary.DispatchId == nil || *artifactSummary.DispatchId != "dispatch-1" {
		t.Fatalf("artifact dispatchId = %#v, want dispatch-1", artifactSummary.DispatchId)
	}
	wantHref := "/factory-sessions/" + sessionID + "/artifacts/child-artifact-1"
	if artifactSummary.RetrievalRef == nil || artifactSummary.RetrievalRef.Href != wantHref {
		t.Fatalf("artifact retrievalRef = %#v, want %q", artifactSummary.RetrievalRef, wantHref)
	}

	artifactDetail := getDurableArtifactDetail(t, serverURL, sessionID, "child-artifact-1")
	if artifactDetail.DispatchId == nil || *artifactDetail.DispatchId != "dispatch-1" {
		t.Fatalf("artifact detail dispatchId = %#v, want dispatch-1", artifactDetail.DispatchId)
	}
	if artifactDetail.Kind != factoryapi.FactoryArtifactKindCHILDRESULT {
		t.Fatalf("artifact detail kind = %q, want CHILD_RESULT", artifactDetail.Kind)
	}
	if artifactDetail.ContentRef == nil || artifactDetail.ContentRef.Href != wantHref {
		t.Fatalf("artifact detail contentRef = %#v, want %q", artifactDetail.ContentRef, wantHref)
	}

	if dispatchSummary.OutputArtifactIds == nil || len(*dispatchSummary.OutputArtifactIds) != 1 ||
		(*dispatchSummary.OutputArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("dispatch outputArtifactIds = %#v, want [child-artifact-1]", dispatchSummary.OutputArtifactIds)
	}
	if dispatchDetail.ArtifactIds == nil || len(*dispatchDetail.ArtifactIds) != 1 ||
		(*dispatchDetail.ArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("dispatch detail artifactIds = %#v, want [child-artifact-1]", dispatchDetail.ArtifactIds)
	}
}

func getDurableArtifactDetail(t *testing.T, serverURL, sessionID, artifactID string) factoryapi.FactorySessionArtifactDetail {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID + "/artifacts/" + artifactID)
	if err != nil {
		t.Fatalf("GET artifact detail: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("artifact detail status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	var response factoryapi.FactorySessionArtifactDetail
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact detail: %v", err)
	}
	return response
}

func assertAPILiveProviderDispatchLifecycleEventsAlignWithReads(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	dispatchID string,
	summary factoryapi.FactorySessionDispatchSummary,
	detail factoryapi.FactoryDispatch,
) {
	t.Helper()

	queued := findAPIFactoryEventByType(events, "DISPATCH_QUEUED", dispatchID)
	if queued == nil {
		t.Fatalf("events = %#v, want DISPATCH_QUEUED for %q", events, dispatchID)
	}
	if queued.Context.DispatchId == nil || *queued.Context.DispatchId != dispatchID {
		t.Fatalf("DISPATCH_QUEUED dispatchId = %#v, want %q", queued.Context.DispatchId, dispatchID)
	}
	payload, err := json.Marshal(queued.Payload)
	if err != nil {
		t.Fatalf("marshal DISPATCH_QUEUED payload: %v", err)
	}
	var queuedBody struct {
		DispatchKind string `json:"dispatchKind"`
	}
	if err := json.Unmarshal(payload, &queuedBody); err != nil {
		t.Fatalf("unmarshal DISPATCH_QUEUED payload: %v", err)
	}
	if queuedBody.DispatchKind != string(factoryapi.FactoryDispatchKindJAVASCRIPTAGENT) {
		t.Fatalf("DISPATCH_QUEUED dispatchKind = %q, want JAVASCRIPT_AGENT", queuedBody.DispatchKind)
	}

	if !isTerminalFactoryDispatchStatus(detail.Status) {
		return
	}

	reconciled := findAPIFactoryEventByType(events, "DISPATCH_RECONCILED", dispatchID)
	if reconciled == nil {
		t.Fatalf("events = %#v, want DISPATCH_RECONCILED for %q", events, dispatchID)
	}
	if reconciled.Context.DispatchId == nil || *reconciled.Context.DispatchId != dispatchID {
		t.Fatalf("DISPATCH_RECONCILED dispatchId = %#v, want %q", reconciled.Context.DispatchId, dispatchID)
	}
	reconciledPayload, err := json.Marshal(reconciled.Payload)
	if err != nil {
		t.Fatalf("marshal DISPATCH_RECONCILED payload: %v", err)
	}
	var reconciledBody struct {
		ReconciledStatus     factoryapi.FactoryDispatchStatus            `json:"reconciledStatus"`
		ReconciliationSource factoryapi.DispatchReconciliationSource     `json:"reconciliationSource"`
		ArtifactIds          *[]string                                   `json:"artifactIds"`
		FailureDetail        *factoryapi.FactoryDispatchFailureDetail    `json:"failureDetail"`
	}
	if err := json.Unmarshal(reconciledPayload, &reconciledBody); err != nil {
		t.Fatalf("unmarshal DISPATCH_RECONCILED payload: %v", err)
	}
	if reconciledBody.ReconciledStatus != detail.Status {
		t.Fatalf("DISPATCH_RECONCILED reconciledStatus = %q, want %q", reconciledBody.ReconciledStatus, detail.Status)
	}
	if detail.Status == factoryapi.FactoryDispatchStatusCOMPLETED {
		if reconciledBody.ReconciliationSource != factoryapi.PROVIDERSESSION {
			t.Fatalf("reconciliationSource = %q, want PROVIDER_SESSION", reconciledBody.ReconciliationSource)
		}
		if summary.OutputArtifactIds == nil || reconciledBody.ArtifactIds == nil ||
			len(*summary.OutputArtifactIds) != len(*reconciledBody.ArtifactIds) {
			t.Fatalf("artifactIds = %#v, want %#v", reconciledBody.ArtifactIds, summary.OutputArtifactIds)
		}
	}
	if detail.Status == factoryapi.FactoryDispatchStatusFAILED {
		assertAPILiveProviderDispatchFailureDetail(t, reconciledBody.FailureDetail)
		assertAPILiveProviderDispatchFailureDetail(t, detail.FailureDetail)
	}
}

func findAPIFactoryEventByType(
	events []factoryapi.FactoryEvent,
	eventType, dispatchID string,
) *factoryapi.FactoryEvent {
	for index := range events {
		event := &events[index]
		if string(event.Type) != eventType {
			continue
		}
		if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
			continue
		}
		return event
	}
	return nil
}

func isTerminalFactoryDispatchStatus(status factoryapi.FactoryDispatchStatus) bool {
	switch status {
	case factoryapi.FactoryDispatchStatusCOMPLETED,
		factoryapi.FactoryDispatchStatusFAILED,
		factoryapi.FactoryDispatchStatusINTERRUPTED:
		return true
	default:
		return false
	}
}

func assertAPILiveProviderDispatchFailureDetail(
	t *testing.T,
	failure *factoryapi.FactoryDispatchFailureDetail,
) {
	t.Helper()
	if failure == nil || failure.Reason == nil {
		t.Fatalf("failureDetail = %#v, want typed provider failure", failure)
	}
	if *failure.Reason != string(interfaces.WorkFailureTypePermanentBadRequest) {
		t.Fatalf("failure reason = %q, want %q", *failure.Reason, interfaces.WorkFailureTypePermanentBadRequest)
	}
	if failure.Message == nil || *failure.Message != "simulated live child error" {
		t.Fatalf("failure message = %#v, want simulated live child error", failure.Message)
	}
	if failure.ErrorClass == nil || *failure.ErrorClass != string(interfaces.WorkFailureFamilyTerminal) {
		t.Fatalf("failure errorClass = %#v, want %q", failure.ErrorClass, interfaces.WorkFailureFamilyTerminal)
	}
}

func assertAPIDispatchStatusTransitions(
	t *testing.T,
	got *[]factoryapi.FactoryDispatchStatus,
	want []factoryapi.FactoryDispatchStatus,
) {
	t.Helper()
	if got == nil {
		t.Fatalf("statusTransitions = nil, want %#v", want)
	}
	if len(*got) != len(want) {
		t.Fatalf("statusTransitions = %#v, want %#v", *got, want)
	}
	for index, status := range *got {
		if status != want[index] {
			t.Fatalf("statusTransitions[%d] = %q, want %q", index, status, want[index])
		}
	}
}

func assertAPIFakeChildInspectionSnapshot(
	t *testing.T,
	snapshot durableSessionInspectionSnapshot,
) {
	t.Helper()
	if snapshot.read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", snapshot.read.Status)
	}
	if snapshot.read.Progress == nil ||
		snapshot.read.Progress.TotalDispatches == nil || *snapshot.read.Progress.TotalDispatches != 1 ||
		snapshot.read.Progress.CompletedDispatches == nil || *snapshot.read.Progress.CompletedDispatches != 1 {
		t.Fatalf("session progress = %#v, want one completed dispatch", snapshot.read.Progress)
	}
	if snapshot.result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", snapshot.result.ResultStatus)
	}
	if len(snapshot.dispatches.Dispatches) != 1 {
		t.Fatalf("dispatch list = %#v, want one dispatch", snapshot.dispatches.Dispatches)
	}
	dispatchSummary := snapshot.dispatches.Dispatches[0]
	if dispatchSummary.Javascript == nil || dispatchSummary.Javascript.ExecutionMode == nil ||
		*dispatchSummary.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeFake {
		t.Fatalf("dispatch executionMode = %#v, want fake", dispatchSummary.Javascript)
	}
	if dispatchSummary.ProviderSessionRefs == nil || len(*dispatchSummary.ProviderSessionRefs) != 1 ||
		(*dispatchSummary.ProviderSessionRefs)[0].Id != "fake-provider-session-1" {
		t.Fatalf("providerSessionRefs = %#v, want fake-provider-session-1", dispatchSummary.ProviderSessionRefs)
	}
	if len(snapshot.artifacts.Artifacts) != 1 {
		t.Fatalf("artifact list = %#v, want one artifact", snapshot.artifacts.Artifacts)
	}
	if snapshot.artifacts.Artifacts[0].DispatchId == nil || *snapshot.artifacts.Artifacts[0].DispatchId != "dispatch-1" {
		t.Fatalf("artifact dispatchId = %#v, want dispatch-1", snapshot.artifacts.Artifacts[0].DispatchId)
	}
}

func assertAPIFakeChildDispatchDetail(t *testing.T, dispatchDetail factoryapi.FactoryDispatch) {
	t.Helper()
	if dispatchDetail.Javascript == nil || dispatchDetail.Javascript.ExecutionMode == nil ||
		*dispatchDetail.Javascript.ExecutionMode != factorysessionexecution.ChildExecutorModeFake {
		t.Fatalf("dispatch detail executionMode = %#v, want fake", dispatchDetail.Javascript)
	}
	if dispatchDetail.Provider == nil || *dispatchDetail.Provider != "fake" {
		t.Fatalf("dispatch provider = %#v, want fake", dispatchDetail.Provider)
	}
}

func assertAPIInspectionResponsesExcludeLiveProviderMarkers(
	t *testing.T,
	snapshot durableSessionInspectionSnapshot,
	dispatchDetail factoryapi.FactoryDispatch,
) {
	t.Helper()
	encoded, err := json.Marshal(struct {
		Read       factoryapi.FactorySessionDurableReadModel
		Result     factoryapi.FactorySessionResult
		Dispatches factoryapi.ListFactorySessionDispatchesResponse
		Artifacts  factoryapi.ListFactorySessionArtifactsResponse
		Events     []factoryapi.FactoryEvent
		Dispatch   factoryapi.FactoryDispatch
	}{
		Read:       snapshot.read,
		Result:     snapshot.result,
		Dispatches: snapshot.dispatches,
		Artifacts:  snapshot.artifacts,
		Events:     snapshot.events,
		Dispatch:   dispatchDetail,
	})
	if err != nil {
		t.Fatalf("marshal inspection snapshot: %v", err)
	}
	responseText := string(encoded)
	if strings.Contains(responseText, "live-provider-session-1") {
		t.Fatalf("fake-child inspection leaked live-provider session ref:\n%s", responseText)
	}
	for _, term := range fixtures.ForbiddenFixtureVocabularyTerms() {
		if strings.Contains(responseText, term) {
			t.Fatalf("inspection response contained forbidden vocabulary %q:\n%s", term, responseText)
		}
	}
}
