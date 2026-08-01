package mcp_resume_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRunServe_RuntimeResumeSmoke_InterruptedSessionResumesThroughMCPControl(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeHarness(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, harness.projectRoot, harness.provider)
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

	resumeResponse := mcpControlResumeWhenInterrupted(t, client, sessionID, 5*time.Second)
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

func TestRunServe_RuntimeResumeSmoke_DispatchContinuityPreservesCompletedChildDispatchesWithoutReplay(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeHarness(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, harness.projectRoot, harness.provider)
	assertInstallSmokeInitialize(t, client)

	sessionID := startMCPRuntimeResumeSmokeInterruptedSession(t, client, harness)

	before := readMCPSessionDurableReadModel(t, client, sessionID)
	assertMCPDurableProgressCounts(t, before.Progress, 1, 2, 0)

	beforeDispatches := listMCPDispatches(t, client, sessionID)
	dispatchOneBefore := requireMCPDispatchSummary(t, beforeDispatches, "dispatch-1", factoryapi.FactoryDispatchStatusCOMPLETED)
	dispatchTwoBefore := requireMCPDispatchSummary(
		t,
		beforeDispatches,
		"dispatch-2",
		factoryapi.FactoryDispatchStatusINTERRUPTED,
		factoryapi.FactoryDispatchStatusRUNNING,
	)
	if len(beforeDispatches.Dispatches) != 2 {
		t.Fatalf("pre-resume dispatch count = %d, want 2", len(beforeDispatches.Dispatches))
	}

	resumeResponse := mcpControlResumeWhenInterrupted(t, client, sessionID, 5*time.Second)
	if resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumeResponse.Outcome)
	}

	after := waitForMCPSessionStatus(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		8*time.Second,
	)
	assertMCPDurableProgressCounts(t, after.Progress, 2, 2, 0)
	if after.Lifecycle == nil || after.Lifecycle.InterruptedAt == nil || after.Lifecycle.ResumedAt == nil {
		t.Fatalf("post-resume lifecycle = %#v, want interruptedAt and resumedAt continuity", after.Lifecycle)
	}
	if before.Lifecycle == nil || before.Lifecycle.InterruptedAt == nil {
		t.Fatal("pre-resume lifecycle missing interruptedAt")
	}
	if !after.Lifecycle.InterruptedAt.Equal(*before.Lifecycle.InterruptedAt) {
		t.Fatalf(
			"interruptedAt changed across resume: before=%s after=%s",
			before.Lifecycle.InterruptedAt,
			after.Lifecycle.InterruptedAt,
		)
	}

	afterDispatches := listMCPDispatches(t, client, sessionID)
	if len(afterDispatches.Dispatches) != 2 {
		t.Fatalf("post-resume dispatch count = %d, want 2 (no replayed child dispatches)", len(afterDispatches.Dispatches))
	}
	dispatchOneAfter := requireMCPDispatchSummary(t, afterDispatches, "dispatch-1", factoryapi.FactoryDispatchStatusCOMPLETED)
	requireMCPDispatchSummary(t, afterDispatches, "dispatch-2", factoryapi.FactoryDispatchStatusCOMPLETED)
	assertMCPDispatchSummaryParity(t, dispatchOneBefore, dispatchOneAfter)
	if dispatchTwoBefore.Status == factoryapi.FactoryDispatchStatusINTERRUPTED {
		dispatchTwoAfter := requireMCPDispatchSummary(t, afterDispatches, "dispatch-2", factoryapi.FactoryDispatchStatusCOMPLETED)
		if dispatchTwoAfter.Id != dispatchTwoBefore.Id {
			t.Fatalf("dispatch-2 id changed across resume: %q -> %q", dispatchTwoBefore.Id, dispatchTwoAfter.Id)
		}
	}

	if harness.provider.callCount() != 3 {
		t.Fatalf("provider infer calls = %d, want exactly 3 (step-one once, blocked step-two once, resumed step-two once)", harness.provider.callCount())
	}

	shutdown()
	closeRunServeSmokeServer(t, nil, serveErr)
}

func TestRunServe_RuntimeResumeSmoke_TerminalSessionResumeReturnsTypedRejectionAndPreservesSessionRead(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeSucceededHarness(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, harness.projectRoot, nil)
	assertInstallSmokeInitialize(t, client)

	sessionID := startMCPRuntimeResumeSmokeSucceededSession(t, client)

	before := readMCPSessionDurableReadModel(t, client, sessionID)
	if before.SessionId != sessionID {
		t.Fatalf("pre-resume sessionId = %q, want %q", before.SessionId, sessionID)
	}
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("pre-resume status = %q, want SUCCEEDED", before.Status)
	}

	response := mcpControlResumeExpectingOutcome(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession,
	)
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", response.Operation)
	}
	if response.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", response.SessionId, sessionID)
	}

	after := readMCPSessionDurableReadModel(t, client, sessionID)
	if after.SessionId != sessionID {
		t.Fatalf("post-resume sessionId = %q, want %q", after.SessionId, sessionID)
	}
	if after.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("post-resume status = %q, want SUCCEEDED unchanged", after.Status)
	}
	if before.ResultSummary == nil || after.ResultSummary == nil {
		t.Fatal("expected result summary before and after rejected resume")
	}
	if after.ResultSummary.ResultStatus != before.ResultSummary.ResultStatus {
		t.Fatalf(
			"result status changed after rejected resume: before=%q after=%q",
			before.ResultSummary.ResultStatus,
			after.ResultSummary.ResultStatus,
		)
	}

	shutdown()
	closeRunServeSmokeServer(t, nil, serveErr)
}

func TestRunServe_RuntimeResumeSmoke_RunningSessionResumeReturnsTypedNoOpAndPreservesSessionRead(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeRunningHarness(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, harness.projectRoot, nil)
	assertInstallSmokeInitialize(t, client)

	sessionID := startMCPRuntimeResumeSmokeRunningSession(t, client)

	before := readMCPSessionDurableReadModel(t, client, sessionID)
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("pre-resume status = %q, want RUNNING", before.Status)
	}

	response := mcpControlResumeExpectingOutcome(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
	)
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", response.Operation)
	}
	if response.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", response.SessionId, sessionID)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume status = %q, want RUNNING", response.Status)
	}

	after := readMCPSessionDurableReadModel(t, client, sessionID)
	if after.SessionId != sessionID {
		t.Fatalf("post-resume sessionId = %q, want %q", after.SessionId, sessionID)
	}
	if after.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("post-resume status = %q, want RUNNING unchanged", after.Status)
	}

	shutdown()
	closeRunServeSmokeServer(t, nil, serveErr)
}

type mcpRuntimeResumeSmokeHarness struct {
	projectRoot string
	provider    *mcpRuntimeResumeSmokeBlockingProvider
}

func newMCPRuntimeResumeSmokeHarness(t *testing.T) *mcpRuntimeResumeSmokeHarness {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	projectRoot := setupMCPRuntimeResumeSmokeWorkflowFixture(t, "resumable-two-step-fake-children.workflow.js", workflowName)
	provider := newMCPRuntimeResumeSmokeBlockingProvider(workflowName)

	return &mcpRuntimeResumeSmokeHarness{
		projectRoot: projectRoot,
		provider:    provider,
	}
}

type mcpRuntimeResumeSmokeSucceededHarness struct {
	projectRoot string
}

func newMCPRuntimeResumeSmokeSucceededHarness(t *testing.T) *mcpRuntimeResumeSmokeSucceededHarness {
	t.Helper()

	const workflowName = "simple-final"
	projectRoot := setupMCPRuntimeResumeSmokeWorkflowFixture(t, "simple-final.workflow.js", workflowName)
	return &mcpRuntimeResumeSmokeSucceededHarness{projectRoot: projectRoot}
}

type mcpRuntimeResumeSmokeRunningHarness struct {
	projectRoot string
}

func newMCPRuntimeResumeSmokeRunningHarness(t *testing.T) *mcpRuntimeResumeSmokeRunningHarness {
	t.Helper()

	const workflowName = "busy-loop"
	projectRoot := setupMCPRuntimeResumeSmokeWorkflowFixture(t, "busy-loop.workflow.js", workflowName)
	return &mcpRuntimeResumeSmokeRunningHarness{projectRoot: projectRoot}
}

func startRootRuntimeMCPServer(
	t *testing.T,
	projectRoot string,
	provider workerexecution.Runner,
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
		// Remove initializer-owned files before testing.TempDir performs its
		// strict Windows cleanup, matching the project-root persistence cleanup.
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

func startMCPRuntimeResumeSmokeSucceededSession(t *testing.T, client *stdioMCPClient) string {
	t.Helper()

	const workflowName = "simple-final"
	workflowNamePtr := workflowName
	args := map[string]any{"subject": "workflows", "count": 2, "prefix": "you"}
	started := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-mcp-runtime-resume-smoke-succeeded-001",
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
	waitForMCPSessionStatus(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		15*time.Second,
	)
	return sessionID
}

func startMCPRuntimeResumeSmokeRunningSession(t *testing.T, client *stdioMCPClient) string {
	t.Helper()

	const workflowName = "busy-loop"
	workflowNamePtr := workflowName
	started := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-mcp-runtime-resume-smoke-running-001",
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
		15*time.Second,
	)
	waitForMCPDispatchStatus(
		t,
		client,
		sessionID,
		"dispatch-2",
		factoryapi.FactoryDispatchStatusRUNNING,
		15*time.Second,
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
	return mcpControlResumeExpectingOutcome(t, client, sessionID, factoryapi.FactorySessionLifecycleControlOutcomeAccepted)
}

func mcpControlResumeWhenInterrupted(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	timeout time.Duration,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	waitForMCPSessionStatus(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		timeout,
	)

	deadline := time.Now().Add(timeout)
	var last factoryapi.FactorySessionLifecycleControlResponse
	for time.Now().Before(deadline) {
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
		last = *response.Result
		if last.Outcome == factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
			return last
		}
		if last.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
			t.Fatalf("resume outcome = %q, want ACCEPTED", last.Outcome)
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("resume outcome = %q, want ACCEPTED within %s", last.Outcome, timeout)
	return last
}

func mcpControlResumeExpectingOutcome(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	want factoryapi.FactorySessionLifecycleControlOutcome,
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
	if response.Result.Outcome != want {
		t.Fatalf("resume outcome = %q, want %q", response.Result.Outcome, want)
	}
	return *response.Result
}

func listMCPDispatches(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	listed := decodeToolResponse[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		client.callTool(mcpfactorysession.ToolListDispatches, map[string]any{"sessionId": sessionID}),
	)
	if listed.Error != nil || listed.Result == nil {
		t.Fatalf("list_dispatches = %#v, want success", listed)
	}
	if listed.Result.SessionId != sessionID {
		t.Fatalf("dispatch sessionId = %q, want %q", listed.Result.SessionId, sessionID)
	}
	if listed.Result.Dispatches == nil {
		t.Fatal("dispatch list unexpectedly missing")
	}
	return *listed.Result
}

func requireMCPDispatchSummary(
	t *testing.T,
	listed factoryapi.ListFactorySessionDispatchesResponse,
	dispatchID string,
	allowedStatuses ...factoryapi.FactoryDispatchStatus,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()

	for _, dispatch := range listed.Dispatches {
		if dispatch.Id != dispatchID {
			continue
		}
		for _, want := range allowedStatuses {
			if dispatch.Status == want {
				return dispatch
			}
		}
		t.Fatalf("dispatch %s status = %q, want one of %#v", dispatchID, dispatch.Status, allowedStatuses)
	}
	t.Fatalf("dispatch %s missing from %#v", dispatchID, listed.Dispatches)
	return factoryapi.FactorySessionDispatchSummary{}
}

func assertMCPDispatchSummaryParity(
	t *testing.T,
	before, after factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if before.Id != after.Id {
		t.Fatalf("dispatch id changed: %q -> %q", before.Id, after.Id)
	}
	if before.Status != after.Status && after.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch %s status drifted unexpectedly: before=%q after=%q", before.Id, before.Status, after.Status)
	}
	if before.DispatchKind != after.DispatchKind {
		t.Fatalf("dispatch %s kind changed: %q -> %q", before.Id, before.DispatchKind, after.DispatchKind)
	}
	if before.OutputArtifactIds != nil && after.OutputArtifactIds != nil {
		if len(*before.OutputArtifactIds) != len(*after.OutputArtifactIds) {
			t.Fatalf(
				"dispatch %s outputArtifactIds length changed: before=%#v after=%#v",
				before.Id,
				*before.OutputArtifactIds,
				*after.OutputArtifactIds,
			)
		}
	}
}

func assertMCPDurableProgressCounts(
	t *testing.T,
	progress *factoryapi.FactorySessionDurableProgressCounts,
	wantCompleted, wantTotal, wantInFlight int,
) {
	t.Helper()
	if progress == nil {
		t.Fatalf("progress = nil, want completed=%d total=%d inFlight=%d", wantCompleted, wantTotal, wantInFlight)
	}
	if mcpIntValueOrZero(progress.CompletedDispatches) != wantCompleted {
		t.Fatalf("completedDispatches = %#v, want %d", progress.CompletedDispatches, wantCompleted)
	}
	if mcpIntValueOrZero(progress.TotalDispatches) != wantTotal {
		t.Fatalf("totalDispatches = %#v, want %d", progress.TotalDispatches, wantTotal)
	}
	if mcpIntValueOrZero(progress.InFlightDispatches) != wantInFlight {
		t.Fatalf("inFlightDispatches = %#v, want %d", progress.InFlightDispatches, wantInFlight)
	}
}

func mcpIntValueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
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
	var last []factoryapi.FactorySessionDispatchSummary
	for time.Now().Before(deadline) {
		listed := decodeToolResponse[factoryapi.ListFactorySessionDispatchesResponse](
			t,
			client.callTool(mcpfactorysession.ToolListDispatches, map[string]any{"sessionId": sessionID}),
		)
		if listed.Error != nil || listed.Result == nil {
			t.Fatalf("list_dispatches = %#v, want success", listed)
		}
		last = listed.Result.Dispatches
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
	lastJSON, _ := json.Marshal(last)
	t.Fatalf("dispatch %s did not reach %s within %s; last dispatches = %s", dispatchID, want, timeout, lastJSON)
}

func setupMCPRuntimeResumeSmokeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()

	projectRoot := support.ScaffoldSingleStepFactory(t, "mcp-resume-smoke")
	t.Cleanup(func() {
		_ = os.RemoveAll(projectRoot)
	})
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "javascript_runtime", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
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

func (p *mcpRuntimeResumeSmokeBlockingProvider) Execute(ctx context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		return workerexecution.InferenceResponse{
			Content: fmt.Sprintf(`{"text":"live:%s:step-one:step-one:workflows","label":"step-one"}`, p.workflowName),
			ProviderSession: &workerexecution.ProviderSessionMetadata{
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
		return workerexecution.InferenceResponse{}, ctx.Err()
	}

	return workerexecution.InferenceResponse{
		Content: fmt.Sprintf(`{"text":"live:%s:step-two:step-two:workflows","label":"step-two"}`, p.workflowName),
		ProviderSession: &workerexecution.ProviderSessionMetadata{
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
