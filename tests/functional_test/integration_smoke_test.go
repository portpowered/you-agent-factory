package functional_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

const (
	scriptTimeoutCompanionSignalTimeout      = 10 * time.Second
	scriptTimeoutCompanionCompletionTimeout  = 20 * time.Second
	scriptTimeoutCompanionCompletionInterval = 100 * time.Millisecond
)

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sliceValue[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}

func mapValue[K comparable, V any](values *map[K]V) map[K]V {
	if values == nil {
		return nil
	}
	return *values
}

// portos:func-length-exception owner=agent-factory reason=legacy-timeout-requeue-smoke-fixture review=2026-07-19 removal=split-timeout-setup-dispatch-wait-and-requeue-assertions-before-next-timeout-smoke-change
func TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	traceID := "trace-script-timeout-companion-001"
	workID := "work-script-timeout-companion-001"

	workstationAgentsPath := filepath.Join(dir, "workstations", "run-script", "AGENTS.md")
	agentsMD := "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxExecutionTime: 10ms\n---\nExecute the script.\n"
	if err := os.WriteFile(workstationAgentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("timeout companion payload"),
	})

	runner := newTimeoutThenReleaseCommandRunner()
	server := StartFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.CommandRunnerOverride = runner
		},
	)

	waitForScriptTimeoutCompanionRetryStarted(t, server, runner, workID)

	engineState := server.GetEngineStateSnapshot(t)
	workDispatches := dispatchesForConsumedWork(engineState.DispatchHistory, workID)
	if len(workDispatches) == 0 {
		t.Fatalf(
			"missing first timeout dispatch after retry signal; %s",
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}
	if workDispatches[0].Outcome != interfaces.OutcomeFailed {
		t.Fatalf(
			"first timeout dispatch outcome = %s, want %s; first dispatch = %s; %s",
			workDispatches[0].Outcome,
			interfaces.OutcomeFailed,
			summarizeScriptTimeoutCompanionDispatch(workDispatches[0]),
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}
	if workDispatches[0].Reason != "execution timeout" {
		t.Fatalf(
			"first timeout dispatch reason = %q, want %q; first dispatch = %s; %s",
			workDispatches[0].Reason,
			"execution timeout",
			summarizeScriptTimeoutCompanionDispatch(workDispatches[0]),
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}
	if !scriptTimeoutCompanionDispatchRequeued(workDispatches[0], "task:init", workID) {
		t.Fatalf(
			"missing requeue to task:init after first timeout dispatch; first dispatch = %s; %s",
			summarizeScriptTimeoutCompanionDispatch(workDispatches[0]),
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}

	close(runner.releaseCh)
	waitForScriptTimeoutCompanionCompletion(t, server, runner, workID)

	if runner.CallCount() < 2 {
		t.Fatalf(
			"missing retry dispatch call after release; %s",
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}

	finalEngineState := server.GetEngineStateSnapshot(t)
	finalWorkDispatches := dispatchesForConsumedWork(finalEngineState.DispatchHistory, workID)
	if len(finalWorkDispatches) < 2 {
		t.Fatalf(
			"missing retry dispatch in final dispatch history; final work DispatchHistory length = %d, want at least 2; %s",
			len(finalWorkDispatches),
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}
	last := finalWorkDispatches[len(finalWorkDispatches)-1]
	if last.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf(
			"failed late completion dispatch outcome = %s, want %s; last dispatch = %s; %s",
			last.Outcome,
			interfaces.OutcomeAccepted,
			summarizeScriptTimeoutCompanionDispatch(last),
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}

	work := server.ListWork(t)
	if len(work.Results) != 1 {
		t.Fatalf(
			"missing late completion result; completed work count = %d, want 1; %s",
			len(work.Results),
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}
	if work.Results[0].WorkId != workID {
		t.Fatalf(
			"late completion result work ID = %q, want %q; %s",
			work.Results[0].WorkId,
			workID,
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}
	if work.Results[0].TraceId != traceID {
		t.Fatalf(
			"late completion result trace ID = %q, want %q; %s",
			work.Results[0].TraceId,
			traceID,
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-shared-command-runner-smoke-fixture review=2026-07-19 removal=split-worker-setup-command-capture-and-shared-runner-assertions-before-next-command-runner-smoke-change
func TestIntegrationSmoke_ScriptAndProviderWorkersShareCommandRunner(t *testing.T) {
	cfg := twoStagePipelineConfig()
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["workingDirectory"] = "/tmp/script-command-smoke"
	workstations[0]["env"] = map[string]any{"SCRIPT_ENV": "script-value"}
	workstations[1]["workingDirectory"] = "/tmp/provider-command-smoke"
	workstations[1]["env"] = map[string]any{"PROVIDER_ENV": "provider-value"}

	dir := scaffoldFactory(t, cfg)
	setWorkingDirectory(t, dir)
	writeNamedWorkstationPromptTemplate(t, dir, "step-two", "Provider received {{ (index .Inputs 0).Payload }}.")
	writeAgentConfig(t, dir, "worker-a", `---
type: SCRIPT_WORKER
command: script-tool
args:
  - "{{ (index .Inputs 0).WorkID }}"
  - "{{ (index .Inputs 0).Payload }}"
---
`)
	writeAgentConfig(t, dir, "worker-b", buildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "mixed-command-smoke-work",
		WorkTypeID: "task",
		TraceID:    "trace-mixed-command-smoke",
		Payload:    []byte("script-input"),
	})

	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("script-output")},
		workers.CommandResult{
			Stdout: []byte("provider-output COMPLETE"),
			Stderr: []byte(`{"event":"session.created","session_id":"sess_mixed_command"}`),
		},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
		testutil.WithProviderCommandRunner(runner),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed")
	assertTokenPayload(t, h.Marking(), "task:complete", "provider-output COMPLETE")

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("shared command runner request count = %d, want 2", len(requests))
	}

	scriptReq := requests[0]
	if scriptReq.Command != "script-tool" {
		t.Fatalf("script command = %q, want %q", scriptReq.Command, "script-tool")
	}
	if !reflect.DeepEqual(scriptReq.Args, []string{"mixed-command-smoke-work", "script-input"}) {
		t.Fatalf("script args = %v, want rendered work ID and payload args", scriptReq.Args)
	}
	if scriptReq.WorkDir != resolvedRuntimePath(dir, "/tmp/script-command-smoke") {
		t.Fatalf("script work dir = %q, want %q", scriptReq.WorkDir, resolvedRuntimePath(dir, "/tmp/script-command-smoke"))
	}
	if !containsEnv(scriptReq.Env, "SCRIPT_ENV=script-value") {
		t.Fatalf("script env missing SCRIPT_ENV in %v", scriptReq.Env)
	}
	if len(scriptReq.Stdin) != 0 {
		t.Fatalf("script stdin = %q, want empty stdin", string(scriptReq.Stdin))
	}
	if !containsString(scriptReq.Execution.WorkIDs, "mixed-command-smoke-work") {
		t.Fatalf("script execution work IDs = %v, want mixed-command-smoke-work", scriptReq.Execution.WorkIDs)
	}

	providerReq := requests[1]
	if providerReq.Command != string(workers.ModelProviderCodex) {
		t.Fatalf("provider command = %q, want %q", providerReq.Command, workers.ModelProviderCodex)
	}
	assertArgsContainSequence(t, providerReq.Args, []string{"exec"})
	// assertArgsContainSequence(t, providerReq.Args, []string{"--cd", "/tmp/provider-command-smoke"})
	assertArgsContainSequence(t, providerReq.Args, []string{"--model", "gpt-5-codex"})
	if providerReq.Args[len(providerReq.Args)-1] != "-" {
		t.Fatalf("provider prompt placeholder = %q, want -", providerReq.Args[len(providerReq.Args)-1])
	}
	if !strings.Contains(string(providerReq.Stdin), "script-output") {
		t.Fatalf("provider stdin = %q, want it to include script output", string(providerReq.Stdin))
	}
	if providerReq.WorkDir != resolvedRuntimePath(dir, "/tmp/provider-command-smoke") {
		t.Fatalf("provider work dir = %q, want %q", providerReq.WorkDir, resolvedRuntimePath(dir, "/tmp/provider-command-smoke"))
	}
	if !containsEnv(providerReq.Env, "PROVIDER_ENV=provider-value") {
		t.Fatalf("provider env missing PROVIDER_ENV in %v", providerReq.Env)
	}
	if !containsString(providerReq.Execution.WorkIDs, "mixed-command-smoke-work") {
		t.Fatalf("provider execution work IDs = %v, want mixed-command-smoke-work", providerReq.Execution.WorkIDs)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-service-mode-lifecycle-smoke review=2026-07-19 removal=split-startup-idle-submission-completion-and-cancel-assertions-before-next-service-mode-smoke-change
func TestServiceModeSmoke_EmptyStartupIdleSubmissionAndPostCompletionIdleStayReachableUntilCanceled(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow service-mode lifecycle smoke")
	dir := scaffoldFactory(t, twoStagePipelineConfig())

	dispatchRelease := make(chan struct{})
	dispatchExecutor := &blockingExecutor{releaseCh: dispatchRelease, mu: &sync.Mutex{}, calls: new(int)}

	server := StartFunctionalServer(t, dir, false,
		factory.WithServiceMode(),
		factory.WithWorkerExecutor("worker-a", dispatchExecutor),
		factory.WithWorkerExecutor("worker-b", staticOutcomeExecutor{outcome: interfaces.OutcomeAccepted}),
	)

	initialState := waitForStateSnapshot(
		t,
		5*time.Second,
		func() (StateResponse, bool) {
			stateResp := server.GetState(t)
			return stateResp, stateResp.FactoryState == "RUNNING" && stateResp.RuntimeStatus == string(interfaces.RuntimeStatusIdle)
		},
	)
	if initialState.TotalTokens != 0 {
		t.Fatalf("initial total tokens = %d, want 0", initialState.TotalTokens)
	}

	initialDashboard := server.GetDashboard(t)
	if initialDashboard.FactoryState != "RUNNING" {
		t.Fatalf("initial dashboard factory_state = %q, want RUNNING", initialDashboard.FactoryState)
	}
	if initialDashboard.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("initial dashboard runtime_status = %q, want %q", initialDashboard.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}
	if initialDashboard.Runtime.InFlightDispatchCount != 0 {
		t.Fatalf("initial in-flight dispatches = %d, want 0", initialDashboard.Runtime.InFlightDispatchCount)
	}

	stream := server.OpenDashboardStream(t)
	initialStreamSnapshot := waitForStreamSnapshot(
		t,
		stream,
		5*time.Second,
		func(snapshot DashboardResponse) bool {
			return snapshot.FactoryState == "RUNNING" && snapshot.RuntimeStatus == string(interfaces.RuntimeStatusIdle)
		},
	)
	if initialStreamSnapshot.Runtime.Session.CompletedCount != 0 {
		t.Fatalf("initial stream completed count = %d, want 0", initialStreamSnapshot.Runtime.Session.CompletedCount)
	}

	traceID := server.SubmitWork(t, "task", []byte(`{"title":"service-mode smoke item"}`))
	if traceID == "" {
		t.Fatal("expected POST /work to return a trace ID")
	}

	activeSnapshot := waitForStreamSnapshot(
		t,
		stream,
		10*time.Second,
		func(snapshot DashboardResponse) bool {
			return snapshot.RuntimeStatus == string(interfaces.RuntimeStatusActive) && snapshot.Runtime.InFlightDispatchCount > 0
		},
	)
	if activeSnapshot.FactoryState != "RUNNING" {
		t.Fatalf("active snapshot factory_state = %q, want RUNNING", activeSnapshot.FactoryState)
	}

	_, activeWorkItem := findActiveWorkItemByTraceID(t, activeSnapshot, traceID)

	close(dispatchRelease)

	finalIdleSnapshot := waitForStreamSnapshot(
		t,
		stream,
		10*time.Second,
		func(snapshot DashboardResponse) bool {
			return snapshot.FactoryState == "RUNNING" &&
				snapshot.RuntimeStatus == string(interfaces.RuntimeStatusIdle) &&
				snapshot.Runtime.InFlightDispatchCount == 0 &&
				snapshot.Runtime.Session.CompletedCount > 0
		},
	)
	if finalIdleSnapshot.Runtime.Session.CompletedCount == 0 {
		t.Fatal("expected final idle snapshot to report completed work")
	}

	work := server.ListWork(t)
	if len(work.Results) != 1 {
		t.Fatalf("work result count = %d, want 1", len(work.Results))
	}
	if work.Results[0].TraceId != traceID {
		t.Fatalf("completed work trace ID = %q, want %q", work.Results[0].TraceId, traceID)
	}
	if work.Results[0].PlaceId != "task:complete" {
		t.Fatalf("completed work place = %q, want task:complete", work.Results[0].PlaceId)
	}
	if work.Results[0].WorkId != activeWorkItem.WorkId {
		t.Fatalf("completed work ID = %q, want %q", work.Results[0].WorkId, activeWorkItem.WorkId)
	}

	postCompletionState := server.GetState(t)
	if postCompletionState.FactoryState != "RUNNING" {
		t.Fatalf("post-completion factory_state = %q, want RUNNING", postCompletionState.FactoryState)
	}
	if postCompletionState.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("post-completion runtime_status = %q, want %q", postCompletionState.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}

	select {
	case <-server.done:
		t.Fatal("service-mode runtime exited after returning to idle; expected it to stay alive until cancellation")
	case <-time.After(500 * time.Millisecond):
	}

	server.cancel()
	select {
	case <-server.done:
	case <-time.After(5 * time.Second):
		t.Fatal("service-mode runtime did not stop after cancellation")
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-observability-runtime-smoke review=2026-07-19 removal=split-snapshot-dashboard-status-and-event-assertions-before-next-observability-smoke-change
func TestObservabilitySmoke_CanonicalServiceSnapshotMatchesStateAndDashboardAcrossRuntimeTransitions(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow observability smoke")
	dir := scaffoldFactory(t, twoStagePipelineConfig())

	dispatchRelease := make(chan struct{})
	dispatchExecutor := &blockingExecutor{releaseCh: dispatchRelease, mu: &sync.Mutex{}, calls: new(int)}

	server := StartFunctionalServer(t, dir, false,
		factory.WithServiceMode(),
		factory.WithWorkerExecutor("worker-a", dispatchExecutor),
		factory.WithWorkerExecutor("worker-b", staticOutcomeExecutor{outcome: interfaces.OutcomeAccepted}),
	)

	idleEngineState := waitForEngineStateSnapshot(
		t,
		server,
		5*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return snapshot.FactoryState == "RUNNING" &&
				snapshot.RuntimeStatus == interfaces.RuntimeStatusIdle &&
				snapshot.InFlightCount == 0
		},
	)
	idleState := server.GetState(t)
	idleDashboard := server.GetDashboard(t)
	assertCanonicalObservabilityAlignment(t, idleEngineState, idleState, idleDashboard)

	stream := server.OpenDashboardStream(t)
	idleStreamSnapshot := waitForStreamSnapshot(
		t,
		stream,
		5*time.Second,
		func(snapshot DashboardResponse) bool {
			return snapshot.FactoryState == idleEngineState.FactoryState &&
				snapshot.RuntimeStatus == string(idleEngineState.RuntimeStatus) &&
				snapshot.Runtime.InFlightDispatchCount == idleEngineState.InFlightCount
		},
	)
	assertDashboardMatchesEngineState(t, "idle stream snapshot", idleEngineState, idleStreamSnapshot)

	traceID := server.SubmitWork(t, "task", []byte(`{"title":"canonical-state-smoke"}`))
	if traceID == "" {
		t.Fatal("expected POST /work to return a trace ID")
	}

	activeEngineState := waitForEngineStateSnapshot(
		t,
		server,
		10*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return snapshot.FactoryState == "RUNNING" &&
				snapshot.RuntimeStatus == interfaces.RuntimeStatusActive &&
				snapshot.InFlightCount > 0
		},
	)
	if activeEngineState.Topology == nil || len(activeEngineState.Topology.Transitions) == 0 {
		t.Fatal("expected aggregate engine-state snapshot to include topology")
	}
	if len(activeEngineState.Dispatches) == 0 {
		t.Fatal("expected aggregate engine-state snapshot to include raw in-flight dispatch records")
	}
	activeState := waitForStateSnapshot(
		t,
		10*time.Second,
		func() (StateResponse, bool) {
			stateResp := server.GetState(t)
			return stateResp, stateResp.FactoryState == "RUNNING" &&
				stateResp.RuntimeStatus == string(interfaces.RuntimeStatusActive)
		},
	)
	activeDashboard := waitForDashboardSnapshot(
		t,
		10*time.Second,
		func() (DashboardResponse, bool) {
			dashboard := server.GetDashboard(t)
			return dashboard, dashboard.FactoryState == "RUNNING" &&
				dashboard.RuntimeStatus == string(interfaces.RuntimeStatusActive) &&
				dashboard.Runtime.InFlightDispatchCount > 0
		},
	)
	assertCanonicalObservabilityAlignment(t, activeEngineState, activeState, activeDashboard)

	activeStreamSnapshot := waitForStreamSnapshot(
		t,
		stream,
		10*time.Second,
		func(snapshot DashboardResponse) bool {
			return snapshot.FactoryState == activeEngineState.FactoryState &&
				snapshot.RuntimeStatus == string(activeEngineState.RuntimeStatus) &&
				snapshot.Runtime.InFlightDispatchCount == activeEngineState.InFlightCount
		},
	)
	assertStreamDashboardMatchesEngineState(t, "active stream snapshot", activeEngineState, activeStreamSnapshot)

	_, activeWorkItem := findActiveWorkItemByTraceID(t, activeDashboard, traceID)
	if stringValue(activeWorkItem.TraceId) != traceID {
		t.Fatalf("active dashboard work item trace ID = %q, want %q", stringValue(activeWorkItem.TraceId), traceID)
	}

	close(dispatchRelease)

	completedEngineState := waitForEngineStateSnapshot(
		t,
		server,
		10*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return snapshot.FactoryState == "RUNNING" &&
				snapshot.RuntimeStatus == interfaces.RuntimeStatusIdle &&
				snapshot.InFlightCount == 0 &&
				hasWorkTokenInPlace(snapshot.Marking, "task:complete", activeWorkItem.WorkId)
		},
	)
	if len(completedEngineState.DispatchHistory) == 0 {
		t.Fatal("expected aggregate engine-state snapshot to retain raw completed dispatch history")
	}
	completedState := waitForStateSnapshot(
		t,
		10*time.Second,
		func() (StateResponse, bool) {
			stateResp := server.GetState(t)
			return stateResp, stateResp.FactoryState == "RUNNING" &&
				stateResp.RuntimeStatus == string(interfaces.RuntimeStatusIdle) &&
				stateResp.Categories.Terminal > 0
		},
	)
	completedDashboard := waitForDashboardSnapshot(
		t,
		10*time.Second,
		func() (DashboardResponse, bool) {
			dashboard := server.GetDashboard(t)
			return dashboard, dashboard.FactoryState == "RUNNING" &&
				dashboard.RuntimeStatus == string(interfaces.RuntimeStatusIdle) &&
				dashboard.Runtime.InFlightDispatchCount == 0 &&
				dashboard.Runtime.Session.CompletedCount > 0
		},
	)
	assertCanonicalObservabilityAlignment(t, completedEngineState, completedState, completedDashboard)

	completedStreamSnapshot := waitForStreamSnapshot(
		t,
		stream,
		10*time.Second,
		func(snapshot DashboardResponse) bool {
			return snapshot.FactoryState == completedEngineState.FactoryState &&
				snapshot.RuntimeStatus == string(completedEngineState.RuntimeStatus) &&
				snapshot.Runtime.InFlightDispatchCount == completedEngineState.InFlightCount &&
				snapshot.Runtime.Session.CompletedCount == completedDashboard.Runtime.Session.CompletedCount
		},
	)
	assertStreamDashboardMatchesEngineState(t, "completed stream snapshot", completedEngineState, completedStreamSnapshot)

	server.cancel()
	select {
	case <-server.done:
	case <-time.After(5 * time.Second):
		t.Fatal("service-mode runtime did not stop after cancellation")
	}
}

type staticOutcomeExecutor struct {
	outcome interfaces.WorkOutcome
}

func (e staticOutcomeExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      e.outcome,
	}, nil
}

func waitForDashboardSnapshot(
	t *testing.T,
	timeout time.Duration,
	check func() (DashboardResponse, bool),
) DashboardResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot, ok := check()
		if ok {
			return snapshot
		}
		time.Sleep(100 * time.Millisecond)
	}

	snapshot, _ := check()
	t.Fatalf("timed out waiting for dashboard condition within %s", timeout)
	return snapshot
}

func waitForEngineStateSnapshot(
	t *testing.T,
	server *FunctionalServer,
	timeout time.Duration,
	match func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		if match(snapshot) {
			return snapshot
		}
		time.Sleep(100 * time.Millisecond)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	t.Fatalf("timed out waiting for engine state snapshot within %s", timeout)
	return snapshot
}

func assertCanonicalObservabilityAlignment(
	t *testing.T,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	stateResp StateResponse,
	dashboard DashboardResponse,
) {
	t.Helper()

	if stateResp.FactoryState != engineState.FactoryState {
		t.Fatalf("state factory_state = %q, want %q", stateResp.FactoryState, engineState.FactoryState)
	}
	if stateResp.RuntimeStatus != string(engineState.RuntimeStatus) {
		t.Fatalf("state runtime_status = %q, want %q", stateResp.RuntimeStatus, engineState.RuntimeStatus)
	}
	if stateResp.TotalTokens != len(engineState.Marking.Tokens) {
		t.Fatalf("state total_tokens = %d, want %d", stateResp.TotalTokens, len(engineState.Marking.Tokens))
	}

	if dashboard.FactoryState != engineState.FactoryState {
		t.Fatalf("dashboard factory_state = %q, want %q", dashboard.FactoryState, engineState.FactoryState)
	}
	if dashboard.RuntimeStatus != string(engineState.RuntimeStatus) {
		t.Fatalf("dashboard runtime_status = %q, want %q", dashboard.RuntimeStatus, engineState.RuntimeStatus)
	}
	if dashboard.TickCount < engineState.TickCount {
		t.Fatalf("dashboard tick_count = %d, want at least %d", dashboard.TickCount, engineState.TickCount)
	}
	if dashboard.Runtime.InFlightDispatchCount != engineState.InFlightCount {
		t.Fatalf("dashboard in-flight dispatch count = %d, want %d", dashboard.Runtime.InFlightDispatchCount, engineState.InFlightCount)
	}
	if dashboard.UptimeSeconds != int64(engineState.Uptime/time.Second) {
		t.Fatalf("dashboard uptime_seconds = %d, want %d", dashboard.UptimeSeconds, int64(engineState.Uptime/time.Second))
	}
	assertDashboardMatchesEngineState(t, "dashboard", engineState, dashboard)
}

func assertDashboardMatchesEngineState(
	t *testing.T,
	label string,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	dashboard DashboardResponse,
) {
	t.Helper()
	assertDashboardMatchesEngineStateWithTickCheck(t, label, engineState, dashboard, true)
}

func assertStreamDashboardMatchesEngineState(
	t *testing.T,
	label string,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	dashboard DashboardResponse,
) {
	t.Helper()
	assertDashboardMatchesEngineStateWithTickCheck(t, label, engineState, dashboard, false)
}

func assertDashboardMatchesEngineStateWithTickCheck(
	t *testing.T,
	label string,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	dashboard DashboardResponse,
	requireCurrentTick bool,
) {
	t.Helper()

	if dashboard.FactoryState != engineState.FactoryState {
		t.Fatalf("%s factory_state = %q, want %q", label, dashboard.FactoryState, engineState.FactoryState)
	}
	if dashboard.RuntimeStatus != string(engineState.RuntimeStatus) {
		t.Fatalf("%s runtime_status = %q, want %q", label, dashboard.RuntimeStatus, engineState.RuntimeStatus)
	}
	if requireCurrentTick && dashboard.TickCount < engineState.TickCount {
		t.Fatalf("%s tick_count = %d, want at least %d", label, dashboard.TickCount, engineState.TickCount)
	}
	if dashboard.Runtime.InFlightDispatchCount != engineState.InFlightCount {
		t.Fatalf("%s in-flight dispatch count = %d, want %d", label, dashboard.Runtime.InFlightDispatchCount, engineState.InFlightCount)
	}
	if dashboard.UptimeSeconds != int64(engineState.Uptime/time.Second) {
		t.Fatalf("%s uptime_seconds = %d, want %d", label, dashboard.UptimeSeconds, int64(engineState.Uptime/time.Second))
	}

	if engineState.Topology != nil && len(engineState.Topology.Transitions) > 0 && len(sliceValue(dashboard.Topology.WorkstationNodeIds)) == 0 {
		t.Fatalf("%s topology workstation node count = 0, want topology derived from event world view", label)
	}
	if engineState.Topology != nil && len(engineState.Topology.Resources) > 0 && len(sliceValue(dashboard.Resources)) == 0 {
		t.Fatalf("%s resource count = 0, want marking-derived resource usage from aggregate snapshot", label)
	}
	if len(engineState.DispatchHistory) > 0 && dashboard.Runtime.Session.CompletedCount == 0 && dashboard.Runtime.Session.FailedCount == 0 {
		t.Fatalf("%s session counts are empty despite aggregate dispatch history", label)
	}
}

func waitForStateSnapshot(
	t *testing.T,
	timeout time.Duration,
	check func() (StateResponse, bool),
) StateResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot, ok := check()
		if ok {
			return snapshot
		}
		time.Sleep(100 * time.Millisecond)
	}

	snapshot, _ := check()
	t.Fatalf("timed out waiting for state condition within %s", timeout)
	return snapshot
}

func waitForStreamSnapshot(
	t *testing.T,
	stream *DashboardStream,
	timeout time.Duration,
	match func(DashboardResponse) bool,
) DashboardResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := stream.NextSnapshot(time.Until(deadline))
		if match(snapshot) {
			return snapshot
		}
	}

	t.Fatalf("timed out waiting for matching dashboard stream snapshot within %s", timeout)
	return DashboardResponse{}
}

func findActiveWorkItemByTraceID(
	t *testing.T,
	snapshot DashboardResponse,
	traceID string,
) (DashboardActiveExecution, DashboardWorkItemRef) {
	t.Helper()

	for _, dispatchID := range sliceValue(snapshot.Runtime.ActiveDispatchIds) {
		execution, ok := mapValue(snapshot.Runtime.ActiveExecutionsByDispatchId)[dispatchID]
		if !ok {
			continue
		}
		for _, workItem := range sliceValue(execution.WorkItems) {
			if stringValue(workItem.TraceId) == traceID {
				return execution, workItem
			}
		}
	}

	for _, execution := range mapValue(snapshot.Runtime.ActiveExecutionsByDispatchId) {
		for _, workItem := range sliceValue(execution.WorkItems) {
			if stringValue(workItem.TraceId) == traceID {
				return execution, workItem
			}
		}
	}

	t.Fatalf("expected an active dashboard work item for trace %q", traceID)
	return DashboardActiveExecution{}, DashboardWorkItemRef{}
}

type timeoutThenReleaseCommandRunner struct {
	mu               sync.Mutex
	callCount        int
	releaseCh        chan struct{}
	firstTimeoutCh   chan struct{}
	retryStartedCh   chan struct{}
	firstTimeoutOnce sync.Once
	retryStartedOnce sync.Once
}

func newTimeoutThenReleaseCommandRunner() *timeoutThenReleaseCommandRunner {
	return &timeoutThenReleaseCommandRunner{
		releaseCh:      make(chan struct{}),
		firstTimeoutCh: make(chan struct{}),
		retryStartedCh: make(chan struct{}),
	}
}

func (r *timeoutThenReleaseCommandRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		<-ctx.Done()
		r.signalFirstTimeout()
		return workers.CommandResult{}, ctx.Err()
	}

	if call == 2 {
		r.signalRetryStarted()
	}

	select {
	case <-r.releaseCh:
		return workers.CommandResult{Stdout: []byte("script-output-after-timeout-retry")}, nil
	case <-ctx.Done():
		return workers.CommandResult{}, ctx.Err()
	}
}

func (r *timeoutThenReleaseCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func (r *timeoutThenReleaseCommandRunner) waitForFirstTimeout(timeout time.Duration) bool {
	select {
	case <-r.firstTimeoutCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (r *timeoutThenReleaseCommandRunner) waitForRetryStarted(timeout time.Duration) bool {
	select {
	case <-r.retryStartedCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (r *timeoutThenReleaseCommandRunner) signalFirstTimeout() {
	r.firstTimeoutOnce.Do(func() {
		close(r.firstTimeoutCh)
	})
}

func (r *timeoutThenReleaseCommandRunner) signalRetryStarted() {
	r.retryStartedOnce.Do(func() {
		close(r.retryStartedCh)
	})
}

func waitForScriptTimeoutCompanionRetryStarted(
	t *testing.T,
	server *FunctionalServer,
	runner *timeoutThenReleaseCommandRunner,
	workID string,
) {
	t.Helper()

	deadline := time.Now().Add(scriptTimeoutCompanionSignalTimeout)
	if !runner.waitForFirstTimeout(scriptTimeoutCompanionSignalTimeout) {
		t.Fatalf(
			"missing first script timeout signal within %s; %s",
			scriptTimeoutCompanionSignalTimeout,
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}

	remaining := time.Until(deadline)
	if remaining <= 0 || !runner.waitForRetryStarted(remaining) {
		t.Fatalf(
			"missing retry dispatch signal within %s after first timeout signal; %s",
			scriptTimeoutCompanionSignalTimeout,
			scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
		)
	}
}

func waitForScriptTimeoutCompanionCompletion(
	t *testing.T,
	server *FunctionalServer,
	runner *timeoutThenReleaseCommandRunner,
	workID string,
) {
	t.Helper()

	deadline := time.Now().Add(scriptTimeoutCompanionCompletionTimeout)
	var lastState string
	var lastTokenCount int
	for time.Now().Before(deadline) {
		stateResp := server.GetState(t)
		lastState = stateResp.FactoryState
		lastTokenCount = stateResp.TotalTokens
		if stateResp.FactoryState == "COMPLETED" {
			return
		}
		time.Sleep(scriptTimeoutCompanionCompletionInterval)
	}

	t.Fatalf(
		"missing late completion within %s after retry release; last factory state = %s; last total tokens = %d; %s",
		scriptTimeoutCompanionCompletionTimeout,
		lastState,
		lastTokenCount,
		scriptTimeoutCompanionDiagnostics(t, server, runner, workID),
	)
}

func scriptTimeoutCompanionDiagnostics(
	t *testing.T,
	server *FunctionalServer,
	runner *timeoutThenReleaseCommandRunner,
	workID string,
) string {
	t.Helper()

	engineState := server.GetEngineStateSnapshot(t)
	return fmt.Sprintf(
		"runner call count = %d; marking = %v; active dispatches = %s; work dispatches = %s; total dispatch history = %d",
		runner.CallCount(),
		engineState.Marking.PlaceTokens,
		summarizeScriptTimeoutCompanionActiveDispatches(engineState.Dispatches),
		summarizeScriptTimeoutCompanionDispatches(dispatchesForConsumedWork(engineState.DispatchHistory, workID)),
		len(engineState.DispatchHistory),
	)
}

func scriptTimeoutCompanionDispatchRequeued(dispatch interfaces.CompletedDispatch, placeID, workID string) bool {
	for _, mutation := range dispatch.OutputMutations {
		if mutation.ToPlace != placeID {
			continue
		}
		if mutation.Token != nil && mutation.Token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func summarizeScriptTimeoutCompanionDispatches(dispatches []interfaces.CompletedDispatch) string {
	if len(dispatches) == 0 {
		return "[]"
	}

	const maxDispatchDiagnostics = 4
	start := 0
	if len(dispatches) > maxDispatchDiagnostics {
		start = len(dispatches) - maxDispatchDiagnostics
	}
	parts := make([]string, 0, len(dispatches)-start+1)
	if start > 0 {
		parts = append(parts, fmt.Sprintf("... %d earlier", start))
	}
	for _, dispatch := range dispatches[start:] {
		parts = append(parts, summarizeScriptTimeoutCompanionDispatch(dispatch))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func summarizeScriptTimeoutCompanionDispatch(dispatch interfaces.CompletedDispatch) string {
	return fmt.Sprintf(
		"{id:%s transition:%s workstation:%s outcome:%s reason:%q provider_failure:%s consumed:%s mutations:%s}",
		dispatch.DispatchID,
		dispatch.TransitionID,
		dispatch.WorkstationName,
		dispatch.Outcome,
		dispatch.Reason,
		formatScriptTimeoutCompanionProviderFailure(dispatch.ProviderFailure),
		formatScriptTimeoutCompanionTokens(dispatch.ConsumedTokens),
		formatScriptTimeoutCompanionMutations(dispatch.OutputMutations),
	)
}

func summarizeScriptTimeoutCompanionActiveDispatches(dispatches map[string]*interfaces.DispatchEntry) string {
	if len(dispatches) == 0 {
		return "[]"
	}

	const maxActiveDispatchDiagnostics = 4
	parts := make([]string, 0, min(len(dispatches), maxActiveDispatchDiagnostics)+1)
	i := 0
	for _, dispatch := range dispatches {
		if i == maxActiveDispatchDiagnostics {
			parts = append(parts, fmt.Sprintf("... %d more", len(dispatches)-i))
			break
		}
		parts = append(parts, fmt.Sprintf(
			"{id:%s transition:%s workstation:%s consumed:%s}",
			dispatch.DispatchID,
			dispatch.TransitionID,
			dispatch.WorkstationName,
			formatScriptTimeoutCompanionTokens(dispatch.ConsumedTokens),
		))
		i++
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatScriptTimeoutCompanionTokens(tokens []interfaces.Token) string {
	if len(tokens) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, fmt.Sprintf("%s@%s", token.Color.WorkID, token.PlaceID))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatScriptTimeoutCompanionMutations(mutations []interfaces.TokenMutationRecord) string {
	if len(mutations) == 0 {
		return "[]"
	}

	const maxMutationDiagnostics = 4
	parts := make([]string, 0, min(len(mutations), maxMutationDiagnostics)+1)
	for i, mutation := range mutations {
		if i == maxMutationDiagnostics {
			parts = append(parts, fmt.Sprintf("... %d more", len(mutations)-i))
			break
		}
		workID := ""
		lastError := ""
		if mutation.Token != nil {
			workID = mutation.Token.Color.WorkID
			lastError = mutation.Token.History.LastError
		}
		parts = append(parts, fmt.Sprintf(
			"{type:%s from:%s to:%s work:%s last_error:%q reason:%q}",
			mutation.Type,
			mutation.FromPlace,
			mutation.ToPlace,
			workID,
			lastError,
			mutation.Reason,
		))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatScriptTimeoutCompanionProviderFailure(failure *interfaces.ProviderFailureMetadata) string {
	if failure == nil {
		return "<nil>"
	}
	return fmt.Sprintf("{type:%s family:%s}", failure.Type, failure.Family)
}

func hasWorkTokenInPlace(marking petri.MarkingSnapshot, placeID, workID string) bool {
	for _, token := range marking.Tokens {
		if token == nil {
			continue
		}
		if token.PlaceID == placeID && token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func dispatchesForConsumedWork(history []interfaces.CompletedDispatch, workID string) []interfaces.CompletedDispatch {
	dispatches := make([]interfaces.CompletedDispatch, 0, len(history))
	for _, dispatch := range history {
		if dispatchConsumesWork(dispatch, workID) {
			dispatches = append(dispatches, dispatch)
		}
	}
	return dispatches
}

func dispatchConsumesWork(dispatch interfaces.CompletedDispatch, workID string) bool {
	for _, token := range dispatch.ConsumedTokens {
		if token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func writeNamedWorkstationPromptTemplate(t *testing.T, dir, workstationName, templateBody string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	content := "---\ntype: MODEL_WORKSTATION\n---\n" + templateBody + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
