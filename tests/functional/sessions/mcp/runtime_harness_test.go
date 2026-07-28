package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startRootRuntimeMCPServer(
	t *testing.T,
	projectRoot string,
	provider workerprovider.Provider,
) (*stdioMCPClient, func(), <-chan error) {
	t.Helper()

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{ProviderOverride: provider})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	homeDir := t.TempDir()
	t.Cleanup(func() {
		_ = os.RemoveAll(homeDir)
	})
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- process.Execute(root.Input{
			Args:             []string{"you", "mcp", "serve", "--runtime", "--project-root", projectRoot},
			Env:              env,
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           &stderr,
			Context:          ctx,
			WorkingDirectory: projectRoot,
		})
	}()
	select {
	case err := <-serveErr:
		t.Fatalf("start root MCP runtime process: %v; stderr=%s", err, stderr.String())
	case <-time.After(100 * time.Millisecond):
	}

	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			cancel()
			_ = stdinWrite.Close()
		})
	}
	t.Cleanup(shutdown)

	return newStdioMCPClient(t, stdinWrite, stdoutRead), shutdown, serveErr
}

func setupWorkflowFixture(t *testing.T, factoryID, fixtureFile, workflowName string) string {
	t.Helper()

	projectRoot := support.ScaffoldSingleStepFactory(t, factoryID)
	t.Cleanup(func() {
		_ = os.RemoveAll(projectRoot)
	})
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "javascript_runtime", fixtureFile))
	if err != nil {
		t.Fatalf("read %s workflow fixture: %v", workflowName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func setupBusyLoopWorkflowFixture(t *testing.T) string {
	t.Helper()
	return setupWorkflowFixture(t, "sessions-mcp-controls", "busy-loop.workflow.js", "busy-loop")
}

func setupSimpleFinalWorkflowFixture(t *testing.T) string {
	t.Helper()
	return setupWorkflowFixture(t, "sessions-mcp-controls-sync", "simple-final.workflow.js", "simple-final")
}

func startMCPSyncSucceededSession(
	t *testing.T,
	client *stdioMCPClient,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	const workflowName = "simple-final"
	workflowNamePtr := workflowName
	args := map[string]any{"subject": "workflows", "count": 2, "prefix": "you"}
	response := decodeToolResponse[factoryapi.FactorySessionSyncExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartSync, factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-sessions-mcp-controls-sync-success-001",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowNamePtr,
			},
			Args: &args,
		}),
	)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("start_sync = %#v, want success", response)
	}
	result := *response.Result
	if result.SessionId == "" {
		t.Fatal("sessionId missing from sync start response")
	}
	if result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("sync status = %q, want SUCCEEDED", result.Status)
	}
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", result.SyncOutcome)
	}
	if result.Result == nil {
		t.Fatal("terminal result missing from sync start response")
	}
	if result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("sync result status = %q, want FINAL", result.Result.ResultStatus)
	}
	return result
}

func startMCPRunningSession(t *testing.T, client *stdioMCPClient) string {
	t.Helper()

	const workflowName = "busy-loop"
	workflowNamePtr := workflowName
	started := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-sessions-mcp-controls-running-001",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowNamePtr,
			},
		}),
	)
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start_async = %#v, want success", started)
	}
	sessionID := started.Result.SessionId
	if sessionID == "" {
		t.Fatal("sessionId missing from async start response")
	}
	waitForMCPSessionStatus(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		5*time.Second,
	)
	return sessionID
}

func readMCPSessionDurableReadModel(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionDurableReadModel](
		t,
		client.callTool(mcpfactorysession.ToolGetSession, map[string]any{"sessionId": sessionID}),
	)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("get = %#v, want success", response)
	}
	return *response.Result
}

func mcpControl(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
	wantOutcome factoryapi.FactorySessionLifecycleControlOutcome,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		client.callTool(mcpfactorysession.ToolControl, map[string]any{
			"sessionId": sessionID,
			"operation": operation,
		}),
	)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("%s control = %#v, want success", operation, response)
	}
	if response.Result.Outcome != wantOutcome {
		t.Fatalf("%s outcome = %q, want %q", operation, response.Result.Outcome, wantOutcome)
	}
	if response.Result.SessionId != sessionID {
		t.Fatalf("%s sessionId = %q, want %q", operation, response.Result.SessionId, sessionID)
	}
	return *response.Result
}

func waitForMCPSessionStatus(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readMCPSessionDurableReadModel(t, client, sessionID)
		if session.Status == want {
			return session
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readMCPSessionDurableReadModel(t, client, sessionID)
	t.Fatalf("session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
	return session
}

func assertCanonicalSessionID(
	t *testing.T,
	gotSessionID string,
	wantSessionID string,
	context string,
) {
	t.Helper()
	if gotSessionID != wantSessionID {
		t.Fatalf("%s sessionId = %q, want canonical %q", context, gotSessionID, wantSessionID)
	}
}

func startFunctionalAPIServerForMCPControls(
	t *testing.T,
	projectRoot string,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                projectRoot,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
}

func readAPIFactorySessionDurableReadModel(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	read := support.GetJSON[factoryapi.FactorySessionDurableReadModel](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
	assertCanonicalSessionID(t, read.SessionId, sessionID, "api read")
	return read
}

func assertFactorySessionOutputExcludesForbiddenVocabulary(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, term := range []string{"DynamicWorkflowRun", "workflow run"} {
		if strings.Contains(lower, strings.ToLower(term)) {
			t.Fatalf("output introduced forbidden vocabulary %q:\n%s", term, text)
		}
	}
}

func assertAPIReadMatchesMCPSharedFactorySessionVocabulary(
	t *testing.T,
	mcpRead factoryapi.FactorySessionDurableReadModel,
	apiRead factoryapi.FactorySessionDurableReadModel,
) {
	t.Helper()
	if mcpRead.Status != apiRead.Status {
		t.Fatalf("mcp status = %q, api status = %q, want matching shared status", mcpRead.Status, apiRead.Status)
	}
	if apiRead.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("api orchestratorKind = %q, want %q", apiRead.OrchestratorKind, factoryapi.JAVASCRIPT)
	}
	encoded, err := json.Marshal(apiRead)
	if err != nil {
		t.Fatalf("marshal api durable read model: %v", err)
	}
	assertFactorySessionOutputExcludesForbiddenVocabulary(t, string(encoded))
}
