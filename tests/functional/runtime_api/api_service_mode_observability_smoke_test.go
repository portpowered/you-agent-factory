package runtime_api_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
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

// portos:func-length-exception owner=agent-factory reason=service-mode-lifecycle-runtime-api-smoke review=2026-07-19 removal=split-startup-idle-submission-completion-and-cancel-assertions-before-next-service-mode-smoke-change
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

// portos:func-length-exception owner=agent-factory reason=observability-runtime-api-smoke review=2026-07-19 removal=split-snapshot-dashboard-status-and-event-assertions-before-next-observability-smoke-change
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

func waitForDashboardSnapshot(t *testing.T, timeout time.Duration, check func() (DashboardResponse, bool)) DashboardResponse {
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

func waitForStateSnapshot(t *testing.T, timeout time.Duration, check func() (StateResponse, bool)) StateResponse {
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
