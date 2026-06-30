package mcpcli_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// pkgmaintcheck:ignore-cyclomatic-complexity runtime resume smoke keeps interrupted setup, MCP control, and terminal continuity on one documented stdio path.
func TestRunServe_RuntimeResumeSmoke_InterruptedSessionResumesThroughMCPControl(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeHarness(t)
	client, shutdown, serveErr := startRunServeWithRuntimeService(t, harness.service)
	assertInstallSmokeInitialize(t, client)
	assertRuntimeResumeSmokeDiscovery(t, client)

	sessionID := startMCPRuntimeResumeSmokeInterruptedSession(t, client, harness)

	before := readMCPSessionDurableReadModel(t, client, sessionID)
	if before.SessionId != sessionID {
		t.Fatalf("pre-resume sessionId = %q, want %q", before.SessionId, sessionID)
	}
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("pre-resume status = %q, want INTERRUPTED", before.Status)
	}
	if before.Lifecycle == nil || before.Lifecycle.InterruptedAt == nil {
		t.Fatalf("pre-resume lifecycle = %#v, want interruptedAt", before.Lifecycle)
	}

	resumeResponse := mcpControlResume(t, client, sessionID)
	if resumeResponse.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", resumeResponse.Operation)
	}
	if resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumeResponse.Outcome)
	}
	if resumeResponse.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", resumeResponse.SessionId, sessionID)
	}

	after := waitForMCPSessionStatus(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		8*time.Second,
	)
	if after.SessionId != sessionID {
		t.Fatalf("post-resume sessionId = %q, want %q", after.SessionId, sessionID)
	}
	if after.Lifecycle == nil || after.Lifecycle.InterruptedAt == nil || after.Lifecycle.ResumedAt == nil {
		t.Fatalf("post-resume lifecycle = %#v, want interruptedAt and resumedAt", after.Lifecycle)
	}
	if after.ResultSummary == nil || after.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("post-resume resultSummary = %#v, want FINAL", after.ResultSummary)
	}

	mode := factoryapi.FactorySessionResultModeFinal
	assertRuntimeSmokeTerminalResult(t, client, sessionID, mode)

	if harness.provider.callCount() < 3 {
		t.Fatalf("provider infer calls = %d, want at least 3 after resume completion", harness.provider.callCount())
	}

	shutdown()
	closeRunServeSmokeServer(t, nil, serveErr)
}

type mcpRuntimeResumeSmokeHarness struct {
	service  fse.Service
	provider *mcpRuntimeResumeSmokeBlockingProvider
}

func newMCPRuntimeResumeSmokeHarness(t *testing.T) *mcpRuntimeResumeSmokeHarness {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	projectRoot := setupMCPRuntimeResumeSmokeWorkflowFixture(t, "resumable-two-step-fake-children.workflow.js", workflowName)
	provider := newMCPRuntimeResumeSmokeBlockingProvider(workflowName)

	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
		PersistSessions:   true,
	})
	t.Cleanup(func() {
		drainRuntimeMCPResumeSmokeSessions(t, service)
	})

	return &mcpRuntimeResumeSmokeHarness{
		service:  service,
		provider: provider,
	}
}

func startRunServeWithRuntimeService(
	t *testing.T,
	service fse.Service,
) (*stdioMCPClient, func(), <-chan error) {
	t.Helper()
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

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- mcpcli.RunServe(ctx, mcpcli.ServeConfig{
			Service: service,
			Stdin:   stdinRead,
			Stdout:  stdoutWrite,
		})
	}()

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

func assertRuntimeResumeSmokeDiscovery(t *testing.T, client *stdioMCPClient) {
	t.Helper()
	toolsResult := client.call("tools/list", map[string]any{})
	toolNames := toolNamesFromListResult(t, toolsResult.Result)
	for _, want := range []string{
		mcpfactorysession.ToolStartAsync,
		mcpfactorysession.ToolGetSession,
		mcpfactorysession.ToolGetResult,
		mcpfactorysession.ToolControl,
		mcpfactorysession.ToolListDispatches,
	} {
		if !containsString(toolNames, want) {
			t.Fatalf("tools/list missing %q; got %#v", want, toolNames)
		}
	}
}

func startMCPRuntimeResumeSmokeInterruptedSession(
	t *testing.T,
	client *stdioMCPClient,
	harness *mcpRuntimeResumeSmokeHarness,
) string {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	workflowNamePtr := workflowName
	args := map[string]any{"subject": "workflows"}
	started := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-mcp-runtime-resume-smoke-start-001",
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

	waitForMCPDispatchStatus(
		t,
		client,
		sessionID,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusCOMPLETED,
		5*time.Second,
	)
	waitForMCPDispatchStatus(
		t,
		client,
		sessionID,
		"dispatch-2",
		factoryapi.FactoryDispatchStatusRUNNING,
		5*time.Second,
	)

	interruptReason := "mcp runtime resume smoke interrupt"
	interrupted := decodeToolResponse[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		client.callTool(mcpfactorysession.ToolControl, map[string]any{
			"sessionId":  sessionID,
			"operation":  factoryapi.FactorySessionLifecycleControlKindInterruptDispatch,
			"dispatchId": "dispatch-2",
			"reason":     interruptReason,
		}),
	)
	if interrupted.Error != nil || interrupted.Result == nil {
		t.Fatalf("interrupt_dispatch = %#v, want success", interrupted)
	}
	if interrupted.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("interrupt outcome = %q, want ACCEPTED", interrupted.Result.Outcome)
	}

	harness.provider.waitForCanceledInfer(t, 5*time.Second)
	waitForMCPSessionStatus(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
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

func mcpControlResume(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		client.callTool(mcpfactorysession.ToolControl, map[string]any{
			"sessionId": sessionID,
			"operation": factoryapi.FactorySessionLifecycleControlKindResume,
		}),
	)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("resume = %#v, want success", response)
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

func waitForMCPDispatchStatus(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	dispatchID string,
	want factoryapi.FactoryDispatchStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := decodeToolResponse[factoryapi.ListFactorySessionDispatchesResponse](
			t,
			client.callTool(mcpfactorysession.ToolListDispatches, map[string]any{"sessionId": sessionID}),
		)
		if listed.Error != nil || listed.Result == nil {
			t.Fatalf("list_dispatches = %#v, want success", listed)
		}
		for _, dispatch := range listed.Result.Dispatches {
			if dispatch.Id != dispatchID {
				continue
			}
			if dispatch.Status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dispatch %s did not reach %s within %s", dispatchID, want, timeout)
}

func setupMCPRuntimeResumeSmokeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()

	projectRoot := t.TempDir()
	t.Cleanup(func() {
		_ = os.RemoveAll(projectRoot)
	})
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

func drainRuntimeMCPResumeSmokeSessions(t *testing.T, service fse.Service) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		list, err := service.ListSessions(context.Background(), fse.ListSessionsRequest{
			Scope: fse.SessionListScopeAll,
		})
		if err != nil {
			return
		}

		pending := false
		for _, session := range list.DurableSessions {
			if fse.IsTerminalLifecycleStatus(session.Status) {
				continue
			}
			pending = true
			_, _ = service.Terminate(context.Background(), session.SessionID, fse.ControlRequest{
				Reason: "test cleanup",
			})
		}
		if !pending {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type mcpRuntimeResumeSmokeBlockingProvider struct {
	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled int
	workflowName    string
}

func newMCPRuntimeResumeSmokeBlockingProvider(workflowName string) *mcpRuntimeResumeSmokeBlockingProvider {
	return &mcpRuntimeResumeSmokeBlockingProvider{workflowName: workflowName}
}

func (p *mcpRuntimeResumeSmokeBlockingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *mcpRuntimeResumeSmokeBlockingProvider) Infer(ctx context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		return interfaces.InferenceResponse{
			Content: fmt.Sprintf(`{"text":"live:%s:step-one:step-one:workflows","label":"step-one"}`, p.workflowName),
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return interfaces.InferenceResponse{}, ctx.Err()
	}

	return interfaces.InferenceResponse{
		Content: fmt.Sprintf(`{"text":"live:%s:step-two:step-two:workflows","label":"step-two"}`, p.workflowName),
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-2",
		},
	}, nil
}

func (p *mcpRuntimeResumeSmokeBlockingProvider) waitForCanceledInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled
		p.mu.Unlock()
		if canceled > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("provider Infer did not observe canceled workflow context")
}
