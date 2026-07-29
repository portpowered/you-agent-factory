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
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/inference"
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

func setupThrowErrorWorkflowFixture(t *testing.T) string {
	t.Helper()
	return setupWorkflowFixture(t, "sessions-mcp-controls-sync-fail", "throw-error.workflow.js", "throw-error")
}

func setupAsyncThrowErrorWorkflowFixture(t *testing.T) string {
	t.Helper()
	return setupWorkflowFixture(t, "sessions-mcp-controls-async-fail", "throw-error.workflow.js", "throw-error")
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

func startMCPSyncFailedSession(
	t *testing.T,
	client *stdioMCPClient,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	const workflowName = "throw-error"
	workflowNamePtr := workflowName
	args := map[string]any{"subject": "workflows"}
	response := decodeToolResponse[factoryapi.FactorySessionSyncExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartSync, factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-sessions-mcp-controls-sync-failure-001",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowNamePtr,
			},
			Args: &args,
		}),
	)
	if response.Error != nil {
		t.Fatalf("start_sync error envelope = %#v, want sync result-path failure", response.Error)
	}
	if response.Result == nil {
		t.Fatal("start_sync result missing from sync failure response")
	}
	result := *response.Result
	if result.SessionId == "" {
		t.Fatal("sessionId missing from sync failure response")
	}
	if result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("sync status = %q, want FAILED", result.Status)
	}
	assertMCPSyncFailureNotTerminalSuccess(t, result)
	return result
}

func assertMCPSyncFailureNotTerminalSuccess(
	t *testing.T,
	syncResult factoryapi.FactorySessionSyncExecutionResponse,
) {
	t.Helper()
	if syncResult.Status == factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("sync status = %q, want non-success terminal failure", syncResult.Status)
	}
	if syncResult.SyncOutcome == factoryapi.FactorySessionSyncExecutionOutcomeCompleted &&
		syncResult.Result != nil &&
		syncResult.Result.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("sync result = %#v, want no terminal success primary result", syncResult.Result)
	}
	if syncResult.Result == nil {
		t.Fatal("sync embedded result missing for structured failure response")
	}
	embedded := syncResult.Result
	if embedded.SessionStatus == nil ||
		*embedded.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("sync embedded sessionStatus = %#v, want FAILED", embedded.SessionStatus)
	}
	if embedded.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("sync embedded resultStatus = %q, want non-FINAL failure availability", embedded.ResultStatus)
	}
	if embedded.PrimaryResult != nil {
		t.Fatalf("sync embedded primaryResult = %#v, want no terminal success payload", embedded.PrimaryResult)
	}
}

func assertMCPStructuredFailureDetail(
	t *testing.T,
	failureDetail *factoryapi.FailureDetail,
	context string,
) {
	t.Helper()
	if failureDetail == nil {
		t.Fatalf("%s failureDetail missing, want structured reason and message", context)
	}
	if strings.TrimSpace(string(failureDetail.Reason)) == "" {
		t.Fatalf("%s failureDetail.reason missing, want stable machine-readable reason", context)
	}
	if strings.TrimSpace(failureDetail.Message) == "" {
		t.Fatalf("%s failureDetail.message missing, want customer-safe failure message", context)
	}
}

func startMCPAsyncSucceededSession(
	t *testing.T,
	client *stdioMCPClient,
) (string, factoryapi.FactorySessionExecutionResponse) {
	t.Helper()

	const workflowName = "simple-final"
	workflowNamePtr := workflowName
	args := map[string]any{"subject": "workflows", "count": 2, "prefix": "you"}
	started := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-sessions-mcp-controls-async-success-001",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowNamePtr,
			},
			Args: &args,
		}),
	)
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start_async = %#v, want success", started)
	}
	sessionID := started.Result.SessionId
	if sessionID == "" {
		t.Fatal("sessionId missing from async start response")
	}
	if started.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("start_async status = %q, want RUNNING", started.Result.Status)
	}
	return sessionID, *started.Result
}

func pollMCPAsyncSessionToTerminalSuccess(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	timeout time.Duration,
) (factoryapi.FactorySessionDurableReadModel, factoryapi.FactorySessionResult) {
	t.Helper()

	mode := factoryapi.FactorySessionResultModeFinal
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		session := readMCPSessionDurableReadModel(t, client, sessionID)
		switch session.Status {
		case factoryapi.FactorySessionDurableLifecycleStatusRunning:
			if mcpAsyncResultIsFinal(t, client, sessionID, mode) {
				// Result can reach FINAL before durable session status catches up.
				break
			}
			assertMCPAsyncRunningResultNotReady(t, client, sessionID, mode)
		case factoryapi.FactorySessionDurableLifecycleStatusSucceeded:
			result := readMCPAsyncTerminalResult(t, client, sessionID, mode)
			return session, result
		default:
			t.Fatalf("get status = %q, want RUNNING or SUCCEEDED while polling async success", session.Status)
		}
		time.Sleep(15 * time.Millisecond)
	}

	session := readMCPSessionDurableReadModel(t, client, sessionID)
	t.Fatalf(
		"session %s status = %q, want SUCCEEDED with terminal result within %s",
		sessionID,
		session.Status,
		timeout,
	)
	return session, factoryapi.FactorySessionResult{}
}

func mcpAsyncResultIsFinal(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	mode factoryapi.FactorySessionResultMode,
) bool {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      mode,
		}),
	)
	return response.Error == nil &&
		response.Result != nil &&
		response.Result.ResultStatus == factoryapi.FactorySessionResultStatusFinal
}

func assertMCPAsyncRunningResultNotReady(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	mode factoryapi.FactorySessionResultMode,
) {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      mode,
		}),
	)
	if response.Result != nil {
		switch response.Result.ResultStatus {
		case factoryapi.FactorySessionResultStatusFinal,
			factoryapi.FactorySessionResultStatusUnavailable:
			// Result can reach a terminal shape before durable session status catches up.
			return
		default:
			t.Fatalf("get_result running = %#v, want not-ready envelope", response.Result)
		}
	}
	if response.Error == nil || response.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("get_result error = %#v, want factory_session.result.not_ready", response.Error)
	}
}

func startMCPAsyncFailedSession(
	t *testing.T,
	client *stdioMCPClient,
) (string, factoryapi.FactorySessionExecutionResponse) {
	t.Helper()

	const workflowName = "throw-error"
	workflowNamePtr := workflowName
	args := map[string]any{"subject": "workflows"}
	started := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-sessions-mcp-controls-async-failure-001",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowNamePtr,
			},
			Args: &args,
		}),
	)
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start_async = %#v, want success", started)
	}
	sessionID := started.Result.SessionId
	if sessionID == "" {
		t.Fatal("sessionId missing from async start response")
	}
	if started.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("start_async status = %q, want RUNNING", started.Result.Status)
	}
	return sessionID, *started.Result
}

func pollMCPAsyncSessionToTerminalFailure(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	timeout time.Duration,
) (factoryapi.FactorySessionDurableReadModel, factoryapi.FactorySessionResult) {
	t.Helper()

	mode := factoryapi.FactorySessionResultModeFinal
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		session := readMCPSessionDurableReadModel(t, client, sessionID)
		switch session.Status {
		case factoryapi.FactorySessionDurableLifecycleStatusRunning:
			if mcpAsyncResultIsUnavailableFailure(t, client, sessionID, mode) {
				// Result can reach terminal failure before durable session status catches up.
				break
			}
			assertMCPAsyncRunningResultNotReady(t, client, sessionID, mode)
		case factoryapi.FactorySessionDurableLifecycleStatusFailed:
			result := readMCPAsyncTerminalFailureResult(t, client, sessionID, mode)
			return session, result
		default:
			t.Fatalf("get status = %q, want RUNNING or FAILED while polling async failure", session.Status)
		}
		time.Sleep(15 * time.Millisecond)
	}

	session := readMCPSessionDurableReadModel(t, client, sessionID)
	t.Fatalf(
		"session %s status = %q, want FAILED with terminal failure result within %s",
		sessionID,
		session.Status,
		timeout,
	)
	return session, factoryapi.FactorySessionResult{}
}

func mcpAsyncResultIsUnavailableFailure(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	mode factoryapi.FactorySessionResultMode,
) bool {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      mode,
		}),
	)
	return response.Error == nil &&
		response.Result != nil &&
		response.Result.ResultStatus == factoryapi.FactorySessionResultStatusUnavailable &&
		response.Result.SessionStatus != nil &&
		*response.Result.SessionStatus == factoryapi.FactorySessionDurableLifecycleStatusFailed
}

func readMCPAsyncTerminalFailureResult(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	mode factoryapi.FactorySessionResultMode,
) factoryapi.FactorySessionResult {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      mode,
		}),
	)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("get_result terminal failure = %#v, want unavailable failure result", response)
	}
	assertMCPAsyncFailureNotTerminalSuccess(t, *response.Result)
	return *response.Result
}

func assertMCPAsyncFailureNotTerminalSuccess(
	t *testing.T,
	result factoryapi.FactorySessionResult,
) {
	t.Helper()
	if result.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("async failure resultStatus = %q, want non-FINAL failure availability", result.ResultStatus)
	}
	if result.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("async failure resultStatus = %q, want UNAVAILABLE", result.ResultStatus)
	}
	if result.SessionStatus == nil ||
		*result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("async failure sessionStatus = %#v, want FAILED", result.SessionStatus)
	}
	if result.PrimaryResult != nil {
		t.Fatalf("async failure primaryResult = %#v, want no terminal success payload", result.PrimaryResult)
	}
}

func readMCPAsyncTerminalResult(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	mode factoryapi.FactorySessionResultMode,
) factoryapi.FactorySessionResult {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      mode,
		}),
	)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("get_result terminal = %#v, want terminal result", response)
	}
	if response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", response.Result.ResultStatus)
	}
	if response.Result.PrimaryResult == nil {
		t.Fatal("primaryResult missing from terminal async result")
	}
	return *response.Result
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

func waitForAPIFactorySessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		read := readAPIFactorySessionDurableReadModel(t, baseURL, sessionID)
		if read.Status == want {
			return read
		}
		time.Sleep(15 * time.Millisecond)
	}
	read := readAPIFactorySessionDurableReadModel(t, baseURL, sessionID)
	t.Fatalf("api session %s status = %q, want %q within %s", sessionID, read.Status, want, timeout)
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

