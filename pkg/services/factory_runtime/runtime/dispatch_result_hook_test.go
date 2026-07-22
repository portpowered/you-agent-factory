package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type panicExecutor struct {
	message string
}

func (e *panicExecutor) Execute(_ context.Context, _ work.WorkDispatch) (workerexecution.WorkResult, error) {
	panic(e.message)
}

type recordingExecutor struct {
	calls []work.WorkDispatch
}

func (e *recordingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.calls = append(e.calls, dispatch)
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "executor-output",
	}, nil
}

type failingExecutor struct {
	err error
}

func (e failingExecutor) Execute(_ context.Context, _ work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{}, e.err
}

type immediateCompletionPlanner struct{}

func (immediateCompletionPlanner) DeliveryTickForDispatch(work.WorkDispatch) (int, bool, error) {
	return 0, false, nil
}

type plannedCompletionPlanner struct {
	deliveryTick     int
	hasDeliveryTick  bool
	plannedResult    workerexecution.WorkResult
	hasPlannedResult bool
}

type validatingCompletionPlanner struct {
	validatedTicks []int
	validateErr    error
}

type asyncRecordingExecutor struct {
	started chan work.WorkDispatch
	release chan struct{}
}

func (e *asyncRecordingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.started <- dispatch
	<-e.release
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "async-executor-output",
	}, nil
}

func (p *validatingCompletionPlanner) DeliveryTickForDispatch(work.WorkDispatch) (int, bool, error) {
	return 0, false, nil
}

func (p *validatingCompletionPlanner) ValidateReplayTick(currentTick int) error {
	p.validatedTicks = append(p.validatedTicks, currentTick)
	return p.validateErr
}

func (p plannedCompletionPlanner) DeliveryTickForDispatch(work.WorkDispatch) (int, bool, error) {
	return p.deliveryTick, p.hasDeliveryTick, nil
}

func (p plannedCompletionPlanner) PlannedResultForDispatch(dispatch work.WorkDispatch) (workerexecution.WorkResult, bool, error) {
	if !p.hasPlannedResult {
		return workerexecution.WorkResult{}, false, nil
	}
	result := p.plannedResult
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID
	return result, true, nil
}

func TestExecuteDispatchSynchronously_ExecutorPanicReturnsFailedResult(t *testing.T) {
	dispatch := work.WorkDispatch{
		DispatchID:   "dispatch-panic",
		TransitionID: "t-process",
	}
	executors := map[string]workers.WorkerExecutor{
		"mock": &panicExecutor{message: "simulated executor panic"},
	}

	result := executeDispatchSynchronously(context.Background(), dispatch, "mock", executors, testRuntimeClock{})

	if result.DispatchID != dispatch.DispatchID || result.TransitionID != dispatch.TransitionID {
		t.Fatalf("panic result lost dispatch identity: %+v", result)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("panic result outcome = %q, want %q", result.Outcome, workerexecution.OutcomeFailed)
	}
	if !strings.Contains(result.Error, "executor panic:") || !strings.Contains(result.Error, "simulated executor panic") {
		t.Fatalf("panic result error = %q, want panic-derived failure message", result.Error)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithPlannerExecutesSynchronously(t *testing.T) {
	executor := &recordingExecutor{}
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		newWorkerPool(logging.NoopLogger{}, testRuntimeClock{}),
		map[string]workers.WorkerExecutor{"mock": executor},
		logging.NoopLogger{},
		1,
		immediateCompletionPlanner{}, testRuntimeClock{})

	dispatch := work.WorkDispatch{
		DispatchID:   "dispatch-sync",
		TransitionID: "t-process",
	}

	if err := hook.SubmitDispatch(context.Background(), dispatch); err != nil {
		t.Fatalf("SubmitDispatch: %v", err)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("executor call count = %d, want 1", len(executor.calls))
	}
	if executor.calls[0].DispatchID != dispatch.DispatchID {
		t.Fatalf("executor dispatch ID = %q, want %q", executor.calls[0].DispatchID, dispatch.DispatchID)
	}

	result, err := hook.OnTick(context.Background(), interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 0})
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("hook result count = %d, want 1", len(result))
	}
	if result[0].Output != "executor-output" {
		t.Fatalf("hook result output = %q, want executor-output", result[0].Output)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithPlannerDelaysDeliveryUntilDueTick(t *testing.T) {
	executor := &recordingExecutor{}
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		newWorkerPool(logging.NoopLogger{}, testRuntimeClock{}),
		map[string]workers.WorkerExecutor{"mock": executor},
		logging.NoopLogger{},
		1,
		plannedCompletionPlanner{deliveryTick: 3, hasDeliveryTick: true}, testRuntimeClock{})

	dispatch := work.WorkDispatch{
		DispatchID:   "dispatch-delayed",
		TransitionID: "t-process",
	}

	if err := hook.SubmitDispatch(context.Background(), dispatch); err != nil {
		t.Fatalf("SubmitDispatch: %v", err)
	}

	result, err := hook.OnTick(context.Background(), interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 2})
	if err != nil {
		t.Fatalf("OnTick before due tick: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("hook result count before due tick = %d, want 0", len(result))
	}

	result, err = hook.OnTick(context.Background(), interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 3})
	if err != nil {
		t.Fatalf("OnTick at due tick: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("hook result count at due tick = %d, want 1", len(result))
	}
	if result[0].Output != "executor-output" {
		t.Fatalf("hook result output at due tick = %q, want executor-output", result[0].Output)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithPlannerUsesPlannedResultReplacement(t *testing.T) {
	executor := &recordingExecutor{}
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		newWorkerPool(logging.NoopLogger{}, testRuntimeClock{}),
		map[string]workers.WorkerExecutor{"mock": executor},
		logging.NoopLogger{},
		1,
		plannedCompletionPlanner{
			plannedResult: workerexecution.WorkResult{
				Outcome: workerexecution.OutcomeAccepted,
				Output:  "planned-output",
			},
			hasPlannedResult: true,
		}, testRuntimeClock{})

	dispatch := work.WorkDispatch{
		DispatchID:   "dispatch-planned-result",
		TransitionID: "t-process",
	}

	if err := hook.SubmitDispatch(context.Background(), dispatch); err != nil {
		t.Fatalf("SubmitDispatch: %v", err)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("executor call count = %d, want 1", len(executor.calls))
	}

	result, err := hook.OnTick(context.Background(), interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 0})
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("hook result count = %d, want 1", len(result))
	}
	if result[0].Output != "planned-output" {
		t.Fatalf("hook result output = %q, want planned-output", result[0].Output)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchUsesRecordedFailedResultForExpectedFailure(t *testing.T) {
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		newWorkerPool(logging.NoopLogger{}, testRuntimeClock{}),
		map[string]workers.WorkerExecutor{
			"mock": failingExecutor{err: errors.New("recorded provider failure")},
		},
		logging.NoopLogger{},
		1,
		plannedCompletionPlanner{
			plannedResult: workerexecution.WorkResult{
				Outcome: workerexecution.OutcomeFailed,
				Error:   "planned failure",
				RecordedOutputWork: []work.FactoryWorkItem{
					{ID: "failed-work"},
				},
			},
			hasPlannedResult: true,
		}, testRuntimeClock{})

	dispatch := work.WorkDispatch{
		DispatchID:   "dispatch-planned-failure",
		TransitionID: "t-process",
	}

	if err := hook.SubmitDispatch(context.Background(), dispatch); err != nil {
		t.Fatalf("SubmitDispatch: %v", err)
	}
	results, err := hook.OnTick(
		context.Background(),
		interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{},
	)
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("hook result count = %d, want 1", len(results))
	}
	if results[0].Error != "planned failure" || len(results[0].RecordedOutputWork) != 1 {
		t.Fatalf("hook result = %+v, want complete recorded failure", results[0])
	}
}

func TestWorkerPoolDispatchResultHook_OnTickValidatesReplayTick(t *testing.T) {
	planner := &validatingCompletionPlanner{}
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		newWorkerPool(logging.NoopLogger{}, testRuntimeClock{}),
		nil,
		logging.NoopLogger{},
		1,
		planner, testRuntimeClock{})

	_, err := hook.OnTick(context.Background(), interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 7})
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(planner.validatedTicks) != 1 || planner.validatedTicks[0] != 7 {
		t.Fatalf("validated ticks = %#v, want [7]", planner.validatedTicks)
	}

	expectedErr := errors.New("replay tick mismatch")
	planner.validateErr = expectedErr
	_, err = hook.OnTick(context.Background(), interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 8})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("OnTick error = %v, want %v", err, expectedErr)
	}
	if len(planner.validatedTicks) != 2 || planner.validatedTicks[1] != 8 {
		t.Fatalf("validated ticks after error = %#v, want [7 8]", planner.validatedTicks)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithoutPlannerUsesWorkerPoolAsyncFlow(t *testing.T) {
	executor := &asyncRecordingExecutor{
		started: make(chan work.WorkDispatch, 1),
		release: make(chan struct{}),
	}
	pool := newWorkerPool(logging.NoopLogger{}, testRuntimeClock{})
	pool.Register("mock", executor)
	pool.Start()
	defer pool.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		pool,
		nil,
		logging.NoopLogger{},
		1,
		nil, testRuntimeClock{})

	hook.Start(ctx)

	dispatch := work.WorkDispatch{
		DispatchID:   "dispatch-async",
		TransitionID: "t-process",
	}
	if err := hook.SubmitDispatch(context.Background(), dispatch); err != nil {
		t.Fatalf("SubmitDispatch: %v", err)
	}

	select {
	case started := <-executor.started:
		if started.DispatchID != dispatch.DispatchID {
			t.Fatalf("started dispatch ID = %q, want %q", started.DispatchID, dispatch.DispatchID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker-pool executor to start")
	}

	result, err := hook.OnTick(context.Background(), interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 0})
	if err != nil {
		t.Fatalf("OnTick before release: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("hook result count before worker completion = %d, want 0", len(result))
	}

	close(executor.release)

	select {
	case <-hook.WaitCh():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async worker-pool completion signal")
	}

	result, err = hook.OnTick(context.Background(), interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 0})
	if err != nil {
		t.Fatalf("OnTick after release: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("hook result count after worker completion = %d, want 1", len(result))
	}
	if result[0].Output != "async-executor-output" {
		t.Fatalf("hook result output = %q, want async-executor-output", result[0].Output)
	}
}

func TestDispatchRunnerKey_PrefersTransitionWorkerType(t *testing.T) {
	tr := &petri.Transition{WorkerType: "cron-worker"}
	dispatch := work.WorkDispatch{WorkstationName: "scheduled-route", TransitionID: "scheduled-route"}

	if got := dispatchRunnerKey(tr, dispatch); got != "cron-worker" {
		t.Fatalf("runner key = %q, want cron-worker", got)
	}
}

func TestDispatchRunnerKey_UsesWorkstationNameWhenTransitionWorkerTypeEmpty(t *testing.T) {
	tr := &petri.Transition{Name: "scheduled-route"}
	dispatch := work.WorkDispatch{WorkstationName: "scheduled-route", TransitionID: "scheduled-route"}

	if got := dispatchRunnerKey(tr, dispatch); got != "scheduled-route" {
		t.Fatalf("runner key = %q, want scheduled-route", got)
	}
}

func TestDispatchRunnerKey_FallsBackToTransitionIDWhenWorkstationNameMissing(t *testing.T) {
	tr := &petri.Transition{Name: "scheduled-route"}
	dispatch := work.WorkDispatch{TransitionID: "scheduled-route"}

	if got := dispatchRunnerKey(tr, dispatch); got != "scheduled-route" {
		t.Fatalf("runner key = %q, want scheduled-route", got)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithoutPlannerReturnsMissingRunnerError(t *testing.T) {
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		newWorkerPool(logging.NoopLogger{}, testRuntimeClock{}),
		nil,
		logging.NoopLogger{},
		1,
		nil, testRuntimeClock{})

	err := hook.SubmitDispatch(context.Background(), work.WorkDispatch{
		DispatchID:   "dispatch-missing-runner",
		TransitionID: "t-process",
	})
	if err == nil {
		t.Fatal("SubmitDispatch error = nil, want missing runner error")
	}
	if !strings.Contains(err.Error(), `no worker pool runner for worker type "mock"`) {
		t.Fatalf("SubmitDispatch error = %q, want missing runner error", err)
	}
}
