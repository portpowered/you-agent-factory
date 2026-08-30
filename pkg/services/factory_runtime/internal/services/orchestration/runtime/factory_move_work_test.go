package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestMoveWork_AcceptsWhileFactoryPaused(t *testing.T) {
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []work.SubmitRequest{{
		WorkID:     "work-paused-move",
		WorkTypeID: "task",
		TraceID:    "trace-paused-move",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	result, err := f.MoveWork(ctx, "work-paused-move", "complete", work.WorkStateChangeSourceCLI, "")
	if err != nil {
		t.Fatalf("MoveWork while paused: %v", err)
	}
	if result.FromState != "init" || result.ToState != "complete" {
		t.Fatalf("move result = %#v, want init -> complete", result)
	}
	assertOperatorWorkStateChangeRecord(t, history, "work-paused-move", "init", "complete", work.WorkStateChangeSourceCLI)

	snap, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snap.Marking, "work-paused-move", "task:complete") {
		t.Fatalf("marking = %#v, want work-paused-move at task:complete", snap.Marking.Tokens)
	}
}

func buildMoveControlNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "complete", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	places := make(map[string]*petri.Place)
	for _, place := range wt.GeneratePlaces() {
		places[place.ID] = place
	}
	return &state.Net{
		ID:          "move-control-net",
		Places:      places,
		Transitions: make(map[string]*petri.Transition),
		WorkTypes:   map[string]*state.WorkType{"task": wt},
		Resources:   make(map[string]*state.ResourceDef),
	}
}

func TestMoveWork_SubscribeReceivesWorkStateChangeInOrder(t *testing.T) {
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	submitCtx := context.Background()
	if _, err := submitWorkRequests(submitCtx, f, []work.SubmitRequest{{
		WorkID:     "work-stream-move",
		WorkTypeID: "task",
		TraceID:    "trace-stream-move",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(submitCtx); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	stream, err := f.SubscribeFactoryEvents(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if stream.StreamGenerationID == "" || history.CallCount("Subscribe") != 1 {
		t.Fatalf("subscription = %#v calls=%v, want injected ledger seam", stream, history.CallsSnapshot())
	}

	if _, err := f.MoveWork(submitCtx, "work-stream-move", "complete", work.WorkStateChangeSourceAPI, ""); err != nil {
		t.Fatalf("MoveWork: %v", err)
	}
	assertOperatorWorkStateChangeRecord(t, history, "work-stream-move", "init", "complete", work.WorkStateChangeSourceAPI)
	// Recordings owns publication of the root work-state-change fact to the
	// subscribed canonical stream and proves replay-before-live ordering.
}

func assertOperatorWorkStateChangeRecord(
	t *testing.T,
	history *recordingfixtures.ScriptedRuntimeLedger,
	workID string,
	fromState string,
	toState string,
	source work.WorkStateChangeSource,
) {
	t.Helper()
	for _, record := range history.WorkStateChanges {
		if record.WorkID == workID && record.FromState == fromState && record.ToState == toState {
			if record.Source != source {
				t.Fatalf("record source = %q, want %q", record.Source, source)
			}
			return
		}
	}
	t.Fatalf("records = %#v, want work-state change for %s %s -> %s", history.WorkStateChanges, workID, fromState, toState)
}

// dispatchReleaseObservationBudget bounds every wait in these guards. A wedged
// dispatch is only released today by the two-hour workstation execution
// timeout, so any bound this small distinguishes "the terminal outcome released
// the dispatch" from "an outer timeout eventually did".
const dispatchReleaseObservationBudget = 10 * time.Second

// terminalOnReleaseExecutor holds one worker dispatch open until the test
// releases it, then reports the exact terminal shape that a Worker Session
// end-state hands back to Factory Runtime.
//
// The held phase reproduces the reported incident state: the dispatch is in
// flight and its consumed work token is absent from the marking. The release
// phase is the fix's contract -- a terminal worker outcome must retire the
// dispatch on its own, without waiting on the execution timeout.
type terminalOnReleaseExecutor struct {
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseGate sync.Once
	cause       error
}

// oneShotScheduler keeps this move regression focused on the dispatch being
// retired. Once the operator move restores and relocates the consumed token,
// the normal runtime scheduler is allowed to observe the terminal Work without
// creating a replacement dispatch.
type oneShotScheduler struct {
	delegate scheduler.Scheduler
	mu       sync.Mutex
	selected bool
}

func (s *oneShotScheduler) Select(
	enabled []interfaces.EnabledTransition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) []interfaces.FiringDecision {
	if len(enabled) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.selected {
		return nil
	}
	decisions := s.delegate.Select(enabled, snapshot)
	if len(decisions) > 0 {
		s.selected = true
	}
	return decisions
}

// releaseOnce ends the held dispatch and is safe to call more than once, so a
// failed assertion can unwind the harness through the same path the happy case
// uses.
func (e *terminalOnReleaseExecutor) releaseOnce() {
	e.releaseGate.Do(func() { close(e.release) })
}

func (e *terminalOnReleaseExecutor) Execute(
	_ context.Context,
	dispatch work.WorkDispatch,
) (workers.WorkResult, error) {
	e.startOnce.Do(func() { close(e.started) })
	<-e.release
	return workers.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workers.OutcomeFailed,
		Error:        e.cause.Error(),
	}, e.cause
}

// TestMoveWork_CanceledWorkerSessionReleasesDispatch is the operator-cancelled
// half of the wedged-token incident. `you worker-sessions cancel` drives the
// attempt to ErrWorkstationDispatchCanceled; the work item's dispatch must
// retire from that alone so `work move` succeeds immediately afterwards.
func TestMoveWork_CanceledWorkerSessionReleasesDispatch(t *testing.T) {
	assertTerminalWorkerOutcomeReleasesDispatch(
		t,
		"work-canceled-session-release",
		workers.ErrWorkstationDispatchCanceled,
	)
}

// TestMoveWork_ProcessGoneWorkerSessionReleasesDispatch is the process-gone
// half. The operator kills the worker's OS process, the runner's bounded wait
// turns that into a terminal failure, and the dispatch must retire without the
// operator waiting out the two-hour execution timeout.
func TestMoveWork_ProcessGoneWorkerSessionReleasesDispatch(t *testing.T) {
	assertTerminalWorkerOutcomeReleasesDispatch(
		t,
		"work-process-gone-release",
		workers.ErrWorkstationDispatchProcessGone,
	)
}

// assertTerminalWorkerOutcomeReleasesDispatch proves the move-side incident
// boundary for one terminal cause: an operator move succeeds while the
// attempt is in flight, then the late worker outcome releases without moving
// the Work away from the operator's terminal target.
func assertTerminalWorkerOutcomeReleasesDispatch(t *testing.T, workID string, cause error) {
	t.Helper()

	executor := &terminalOnReleaseExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
		cause:   cause,
	}
	harness := startServiceModeRunHarness(t,
		withNet(buildSimpleNetWithFailureArc()),
		withServiceMode(),
		withScheduler(&oneShotScheduler{delegate: scheduler.NewFIFOScheduler()}),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	// Registered before stop so it unwinds first: a failed assertion must never
	// leave the executor blocked, or the harness shutdown hangs and reports a
	// second, misleading failure.
	t.Cleanup(harness.stop)
	t.Cleanup(executor.releaseOnce)

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, harness.Factory, []work.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    "trace-" + workID,
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	select {
	case <-executor.started:
	case <-time.After(dispatchReleaseObservationBudget):
		t.Fatalf("timed out waiting for the %s dispatch to reach the worker boundary", workID)
	}
	waitForAggregateSnapshot(t, harness.Factory, func(
		snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	) bool {
		return snapshot.InFlightCount > 0
	})

	// The operator move restores the consumed token, commits the terminal
	// target, and invalidates the matching Runtime intent before returning.
	result, err := harness.Factory.MoveWork(
		ctx, workID, "done", work.WorkStateChangeSourceCLI, "",
	)
	if err != nil {
		t.Fatalf("MoveWork while the attempt is in flight: %v", err)
	}
	if result.FromState != "init" || result.ToState != "done" {
		t.Fatalf("MoveWork result = %#v, want init -> done", result)
	}

	executor.releaseOnce()

	// The late worker outcome must retire the invalidated dispatch. Nothing
	// here waits on the workstation execution timeout.
	waitForAggregateSnapshotWithTimeout(
		t,
		harness.Factory,
		dispatchReleaseObservationBudget,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			if snapshot.InFlightCount != 0 {
				return false
			}
			return markingContainsWorkAtPlace(&snapshot.Marking, workID, "task:done") &&
				!markingContainsWorkAtPlace(&snapshot.Marking, workID, "task:failed")
		},
	)

	snapshot, err := harness.Factory.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, workID, "task:done") {
		t.Fatalf("marking = %#v, want %s at task:done", snapshot.Marking.Tokens, workID)
	}
}
