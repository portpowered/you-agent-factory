package mcp_resume_test

import (
	"encoding/json"
	"testing"
	"time"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestRunServe_RuntimeResumeSmoke_InterruptedSessionResumesThroughMCPControl(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeHarness(t)
	client := harness.client
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

	if harness.provider.callCount(sessionID) < 3 {
		t.Fatalf("provider execute calls = %d, want at least 3 after resume completion", harness.provider.callCount(sessionID))
	}
}

func TestRunServe_RuntimeResumeSmoke_DispatchContinuityPreservesCompletedChildDispatchesWithoutReplay(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeHarness(t)
	client := harness.client

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

	if harness.provider.callCount(sessionID) != 3 {
		t.Fatalf("provider execute calls = %d, want exactly 3 (step-one once, blocked step-two once, resumed step-two once)", harness.provider.callCount(sessionID))
	}
}

func TestRunServe_RuntimeResumeSmoke_TerminalSessionResumeReturnsTypedRejectionAndPreservesSessionRead(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeSucceededHarness(t)
	client := harness.client

	sessionID := startMCPRuntimeResumeSmokeSucceededSession(t, client, harness.fixture)

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
}

func TestRunServe_RuntimeResumeSmoke_RunningSessionResumeReturnsTypedNoOpAndPreservesSessionRead(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeRunningHarness(t)
	client := harness.client

	sessionID := startMCPRuntimeResumeSmokeRunningSession(t, client, harness.fixture)

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
}

type mcpRuntimeResumeSmokeHarness struct {
	provider *mcpRuntimeResumeSmokeProviderRouter
	fixture  *mcpResumePackageFixture
	client   *stdioMCPClient
}

func newMCPRuntimeResumeSmokeHarness(t *testing.T) *mcpRuntimeResumeSmokeHarness {
	t.Helper()

	fixture := mcpResumePackageFixtureForTest(t)
	return &mcpRuntimeResumeSmokeHarness{
		provider: fixture.provider,
		fixture:  fixture,
		client:   fixture.client,
	}
}

type mcpRuntimeResumeSmokeSucceededHarness struct {
	fixture *mcpResumePackageFixture
	client  *stdioMCPClient
}

func newMCPRuntimeResumeSmokeSucceededHarness(t *testing.T) *mcpRuntimeResumeSmokeSucceededHarness {
	t.Helper()

	fixture := mcpResumePackageFixtureForTest(t)
	return &mcpRuntimeResumeSmokeSucceededHarness{
		fixture: fixture,
		client:  fixture.client,
	}
}

type mcpRuntimeResumeSmokeRunningHarness struct {
	fixture *mcpResumePackageFixture
	client  *stdioMCPClient
}

func newMCPRuntimeResumeSmokeRunningHarness(t *testing.T) *mcpRuntimeResumeSmokeRunningHarness {
	t.Helper()

	fixture := mcpResumePackageFixtureForTest(t)
	return &mcpRuntimeResumeSmokeRunningHarness{
		fixture: fixture,
		client:  fixture.client,
	}
}

func startMCPRuntimeResumeSmokeSucceededSession(
	t *testing.T,
	client *stdioMCPClient,
	fixture *mcpResumePackageFixture,
) string {
	t.Helper()

	const workflowName = "simple-final"
	workflowNamePtr := workflowName
	args := map[string]any{"subject": "workflows", "count": 2, "prefix": "you"}
	started := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, factoryapi.FactorySessionExecutionRequest{
			RequestId: fixture.nextRequestID("succeeded"),
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
	fixture.trackSession(t, client, sessionID)
	waitForMCPSessionStatus(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		15*time.Second,
	)
	return sessionID
}

func startMCPRuntimeResumeSmokeRunningSession(
	t *testing.T,
	client *stdioMCPClient,
	fixture *mcpResumePackageFixture,
) string {
	t.Helper()

	const workflowName = "busy-loop"
	workflowNamePtr := workflowName
	started := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, factoryapi.FactorySessionExecutionRequest{
			RequestId: fixture.nextRequestID("running"),
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
	fixture.trackSession(t, client, sessionID)
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
			RequestId: harness.fixture.nextRequestID("interrupted"),
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
	harness.fixture.trackSession(t, client, sessionID)

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
	harness.provider.waitForExecuteBlocked(t, sessionID, 5*time.Second)
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

	harness.provider.waitForCanceledExecute(t, sessionID, 5*time.Second)
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

func mcpControlResumeWhenInterrupted(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	timeout time.Duration,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	// The public MCP control response is the synchronization witness here: the
	// provider cancellation channel proves edge progress, but only retrying the
	// control operation proves the server has observed the persisted interrupted
	// state and accepted the requested resume. Keep the bounded retry without a
	// fixed startup wait.
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

	// The durable read model is the required witness for persisted lifecycle
	// state. A provider or runtime signal cannot prove that the public MCP read
	// projection has reached the requested status, so this functional poll is
	// intentionally bounded and uses only a short yield between reads.
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

	// Dispatch status is owned by the public MCP dispatch projection; provider
	// completion alone cannot prove that projection has recorded the requested
	// status. Poll the public list response with the existing bounded yield.
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
