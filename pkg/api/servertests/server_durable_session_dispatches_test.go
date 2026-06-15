package apiserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

func TestListFactorySessionDispatches_RuntimeBackedReturnsTypedEmptyList(t *testing.T) {
	projectRoot := setupAPIDispatchRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
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
	waitForAPIDispatchRuntimeSessionTerminal(t, service, started.SessionID)

	srv := newAPIDispatchTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + started.SessionID + "/dispatches")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readAPIDispatchBody(t, resp))
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
	projectRoot := setupAPIDispatchRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newAPIDispatchTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-dispatch-001/dispatches")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readAPIDispatchBody(t, resp))
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
	srv := newAPIDispatchTestServer(&testutil.MockFactory{
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
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readAPIDispatchBody(t, resp))
	}
}

func TestGetFactorySessionDispatch_RuntimeBackedReturnsTypedDetail(t *testing.T) {
	projectRoot := setupAPIDispatchRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
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

	srv := newAPIDispatchTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + completed.SessionID + "/dispatches/dispatch-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches/{dispatch_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readAPIDispatchBody(t, resp))
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
	projectRoot := setupAPIDispatchRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
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
	waitForAPIDispatchRuntimeSessionTerminal(t, service, started.SessionID)

	srv := newAPIDispatchTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + started.SessionID + "/dispatches/dispatch-missing-001")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches/{dispatch_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readAPIDispatchBody(t, resp))
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
	projectRoot := setupAPIDispatchRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	srv := newAPIDispatchTestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-dispatch-detail-001/dispatches/dispatch-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/dispatches/{dispatch_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readAPIDispatchBody(t, resp))
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
	srv := newAPIDispatchTestServer(&testutil.MockFactory{
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
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readAPIDispatchBody(t, resp))
	}
}

func newAPIDispatchTestServer(f *testutil.MockFactory) *api.Server {
	logger, _ := zap.NewDevelopment()
	return api.NewServer(f, 8080, logger)
}

func setupAPIDispatchRuntimeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "javascript", "runtime", "testdata", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func waitForAPIDispatchRuntimeSessionTerminal(
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

func readAPIDispatchBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return strings.TrimSpace(string(body))
}
