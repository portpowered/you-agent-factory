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
	if errResp.Code != factoryapi.NOTFOUND {
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
	if errResp.Code != factoryapi.NOTFOUND {
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
	if errResp.Code != factoryapi.NOTFOUND {
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
