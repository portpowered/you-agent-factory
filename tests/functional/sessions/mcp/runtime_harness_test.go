package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type mcpSharedFixture struct {
	process     *initializerapplication.Process
	openRuntime factorysessions.ExecutionRuntimeOpeningFunc
}

type mcpRuntimeHost struct {
	events chan factorydefinitions.FactoryEvent
}

type mcpObservedDurableExecution struct {
	mcpfactorysession.DurableExecution
	consume factorysessions.FactoryEventConsumer
}

func (execution *mcpObservedDurableExecution) StartAsync(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	request.EventConsumer = execution.consume
	return execution.DurableExecution.StartAsync(ctx, request)
}

type mcpCanonicalEventSubscription struct {
	events <-chan factorydefinitions.FactoryEvent
	cancel context.CancelFunc
	once   sync.Once
}

var mcpSharedFixtureState struct {
	once    sync.Once
	fixture *mcpSharedFixture
	err     error
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	if fixture := mcpSharedFixtureState.fixture; fixture != nil && fixture.process != nil {
		if err := fixture.process.Close(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "close shared MCP process: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}
	os.Exit(exitCode)
}

func mcpSharedProcessForTest(t *testing.T) *mcpSharedFixture {
	t.Helper()
	mcpSharedFixtureState.once.Do(func() {
		mcpSharedFixtureState.fixture, mcpSharedFixtureState.err = newMCPSharedFixture()
	})
	if mcpSharedFixtureState.err != nil {
		t.Fatalf("build shared MCP process: %v", mcpSharedFixtureState.err)
	}
	return mcpSharedFixtureState.fixture
}

func newMCPSharedFixture() (*mcpSharedFixture, error) {
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = process.Close(context.Background())
		}
	}()

	capability := process.ExecutionRuntimeOpening()
	if capability == nil {
		return nil, fmt.Errorf("shared MCP process did not expose execution runtime opening")
	}
	opening, ok := capability.ExecutionRuntimeOpening().(factorysessions.ExecutionRuntimeOpeningFunc)
	if !ok || opening == nil {
		return nil, fmt.Errorf("shared MCP process execution runtime opening has type %T, want factorysessions.ExecutionRuntimeOpeningFunc", capability.ExecutionRuntimeOpening())
	}
	closeOnError = false
	return &mcpSharedFixture{process: process, openRuntime: opening}, nil
}

func startRootRuntimeMCPServer(
	t *testing.T,
	projectRoot string,
) (*stdioMCPClient, func(), <-chan error, *mcpRuntimeHost) {
	t.Helper()

	fixture := mcpSharedProcessForTest(t)
	stdinRead, stdinWrite, stdoutRead, stdoutWrite := openMCPStdioPipes(t)
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
	events := make(chan factorydefinitions.FactoryEvent, 256)
	opened, observedExecution := openObservedMCPExecution(t, fixture, projectRoot, homeDir, events)
	serverFactory := fixture.process.MCPServerFactory()
	if serverFactory == nil {
		_ = opened.Close()
		t.Fatalf("build MCP server: process MCP server factory is required")
	}
	server, err := serverFactory(observedExecution)
	if err != nil {
		_ = opened.Close()
		t.Fatalf("build MCP server: %v", err)
	}

	serveErr := make(chan error, 1)
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		runErr := server.ServeStdio(ctx, stdinRead, stdoutWrite)
		if closeErr := opened.Close(); closeErr != nil {
			runErr = errors.Join(runErr, closeErr)
		}
		serveErr <- runErr
	}()
	select {
	case err := <-serveErr:
		t.Fatalf("start root MCP runtime process: %v", err)
	default:
	}

	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			cancel()
			_ = stdinWrite.Close()
		})
	}
	t.Cleanup(func() {
		shutdown()
		select {
		case <-serveDone:
		case <-time.After(5 * time.Second):
			t.Errorf("root MCP runtime process did not shut down during cleanup")
		}
	})

	return newStdioMCPClient(t, stdinWrite, stdoutRead), shutdown, serveErr, &mcpRuntimeHost{events: events}
}

func openMCPStdioPipes(t *testing.T) (*os.File, *os.File, *os.File, *os.File) {
	t.Helper()
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		t.Fatalf("stdout pipe: %v", err)
	}
	return stdinRead, stdinWrite, stdoutRead, stdoutWrite
}

func openObservedMCPExecution(
	t *testing.T,
	fixture *mcpSharedFixture,
	projectRoot string,
	homeDir string,
	events chan<- factorydefinitions.FactoryEvent,
) (factorysessions.OpenedExecutionRuntime, mcpfactorysession.DurableExecution) {
	t.Helper()
	opened, err := fixture.openRuntime(t.Context(), factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot:      projectRoot,
		SystemConfigHome: homeDir,
	})
	if err != nil {
		t.Fatalf("open MCP runtime: %v", err)
	}
	if opened.Execution == nil || opened.Close == nil {
		if opened.Close != nil {
			_ = opened.Close()
		}
		t.Fatalf("open MCP runtime returned incomplete execution owner: execution=%T close=%t", opened.Execution, opened.Close != nil)
	}
	execution, ok := any(opened.Execution).(mcpfactorysession.DurableExecution)
	if !ok || execution == nil {
		_ = opened.Close()
		t.Fatalf("MCP runtime execution has type %T, want MCP durable execution capability", opened.Execution)
	}
	observedExecution := &mcpObservedDurableExecution{
		DurableExecution: execution,
		consume: func(observed []factorydefinitions.FactoryEvent) {
			for _, event := range observed {
				events <- event.Clone()
			}
		},
	}
	return opened, observedExecution
}

func (host *mcpRuntimeHost) subscribeCanonicalEvents(t *testing.T) *mcpCanonicalEventSubscription {
	t.Helper()
	_, cancel := context.WithCancel(t.Context())
	return &mcpCanonicalEventSubscription{
		events: host.events,
		cancel: cancel,
	}
}

func (subscription *mcpCanonicalEventSubscription) next(ctx context.Context) (factorydefinitions.FactoryEvent, error) {
	if subscription == nil {
		return factorydefinitions.FactoryEvent{}, errors.New("canonical Factory Event subscription is nil")
	}
	select {
	case event := <-subscription.events:
		return event, nil
	case <-ctx.Done():
		return factorydefinitions.FactoryEvent{}, ctx.Err()
	}
}

func (subscription *mcpCanonicalEventSubscription) detach() {
	if subscription == nil {
		return
	}
	subscription.once.Do(func() {
		subscription.cancel()
	})
}

func waitForMCPCanonicalTerminalEvent(
	t *testing.T,
	subscription *mcpCanonicalEventSubscription,
	want factorysessions.LifecycleStatus,
	timeout time.Duration,
) factorydefinitions.FactoryEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		event, err := subscription.next(ctx)
		if err != nil {
			t.Fatalf("wait for canonical Factory Event %q: %v", want, err)
		}
		status, terminal := mcpCanonicalTerminalStatus(event)
		if !terminal {
			continue
		}
		if status != want {
			t.Fatalf("canonical Factory Session terminal event = %s/%s, want status %q", event.Type, status, want)
		}
		return event
	}
}

func mcpCanonicalTerminalStatus(event factorydefinitions.FactoryEvent) (factorysessions.LifecycleStatus, bool) {
	if event.Type != factorydefinitions.FactoryEventTypeSessionCompleted {
		return "", false
	}
	var payload struct {
		FinalStatus factorysessions.LifecycleStatus `json:"finalStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	switch payload.FinalStatus {
	case factorysessions.LifecycleStatusSucceeded,
		factorysessions.LifecycleStatusFailed,
		factorysessions.LifecycleStatusCanceled:
		return payload.FinalStatus, true
	default:
		return "", false
	}
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

func observeMCPAsyncSessionToTerminalSuccess(
	t *testing.T,
	client *stdioMCPClient,
	subscription *mcpCanonicalEventSubscription,
	sessionID string,
	timeout time.Duration,
) (factoryapi.FactorySessionDurableReadModel, factoryapi.FactorySessionResult) {
	t.Helper()

	mode := factoryapi.FactorySessionResultModeFinal
	session := readMCPSessionDurableReadModel(t, client, sessionID)
	switch session.Status {
	case factoryapi.FactorySessionDurableLifecycleStatusRunning:
		assertMCPAsyncRunningResultNotReady(t, client, sessionID, mode)
	case factoryapi.FactorySessionDurableLifecycleStatusSucceeded:
		// The retained cursor still proves the canonical terminal event when the
		// async workflow finished before the first follow-up read.
	case factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusCanceled:
		t.Fatalf("get status = %q, want RUNNING or SUCCEEDED while observing async success", session.Status)
	default:
		t.Fatalf("get status = %q, want RUNNING or SUCCEEDED while observing async success", session.Status)
	}

	waitForMCPCanonicalTerminalEvent(t, subscription, factorysessions.LifecycleStatusSucceeded, timeout)
	session = readMCPSessionDurableReadModel(t, client, sessionID)
	if session.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session %s status = %q after terminal Factory Event, want SUCCEEDED", sessionID, session.Status)
	}
	return session, readMCPAsyncTerminalResult(t, client, sessionID, mode)
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

func observeMCPAsyncSessionToTerminalFailure(
	t *testing.T,
	client *stdioMCPClient,
	subscription *mcpCanonicalEventSubscription,
	sessionID string,
	timeout time.Duration,
) (factoryapi.FactorySessionDurableReadModel, factoryapi.FactorySessionResult) {
	t.Helper()

	mode := factoryapi.FactorySessionResultModeFinal
	session := readMCPSessionDurableReadModel(t, client, sessionID)
	switch session.Status {
	case factoryapi.FactorySessionDurableLifecycleStatusRunning:
		assertMCPAsyncRunningResultNotReady(t, client, sessionID, mode)
	case factoryapi.FactorySessionDurableLifecycleStatusFailed:
		// The retained cursor still proves the canonical terminal event when the
		// async workflow finished before the first follow-up read.
	case factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		factoryapi.FactorySessionDurableLifecycleStatusCanceled:
		t.Fatalf("get status = %q, want RUNNING or FAILED while observing async failure", session.Status)
	default:
		t.Fatalf("get status = %q, want RUNNING or FAILED while observing async failure", session.Status)
	}

	waitForMCPCanonicalTerminalEvent(t, subscription, factorysessions.LifecycleStatusFailed, timeout)
	session = readMCPSessionDurableReadModel(t, client, sessionID)
	if session.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session %s status = %q after terminal Factory Event, want FAILED", sessionID, session.Status)
	}
	return session, readMCPAsyncTerminalFailureResult(t, client, sessionID, mode)
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
	if started.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("start_async status = %q, want RUNNING", started.Result.Status)
	}
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
