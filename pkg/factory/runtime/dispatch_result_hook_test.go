package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type recordingExecutor struct {
	calls []interfaces.WorkDispatch
}

func (e *recordingExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.calls = append(e.calls, dispatch)
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "executor-output",
	}, nil
}

type immediateCompletionPlanner struct{}

func (immediateCompletionPlanner) DeliveryTickForDispatch(interfaces.WorkDispatch) (int, bool, error) {
	return 0, false, nil
}

type plannedCompletionPlanner struct {
	deliveryTick     int
	hasDeliveryTick  bool
	plannedResult    interfaces.WorkResult
	hasPlannedResult bool
}

type validatingCompletionPlanner struct {
	validatedTicks []int
	validateErr    error
}

type asyncRecordingExecutor struct {
	started chan interfaces.WorkDispatch
	release chan struct{}
}

func (e *asyncRecordingExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.started <- dispatch
	<-e.release
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "async-executor-output",
	}, nil
}

func (p *validatingCompletionPlanner) DeliveryTickForDispatch(interfaces.WorkDispatch) (int, bool, error) {
	return 0, false, nil
}

func (p *validatingCompletionPlanner) ValidateReplayTick(currentTick int) error {
	p.validatedTicks = append(p.validatedTicks, currentTick)
	return p.validateErr
}

func (p plannedCompletionPlanner) DeliveryTickForDispatch(interfaces.WorkDispatch) (int, bool, error) {
	return p.deliveryTick, p.hasDeliveryTick, nil
}

func (p plannedCompletionPlanner) PlannedResultForDispatch(dispatch interfaces.WorkDispatch) (interfaces.WorkResult, bool, error) {
	if !p.hasPlannedResult {
		return interfaces.WorkResult{}, false, nil
	}
	result := p.plannedResult
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID
	return result, true, nil
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithPlannerExecutesSynchronously(t *testing.T) {
	executor := &recordingExecutor{}
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		workers.NewWorkerPool(logging.NoopLogger{}),
		map[string]workers.WorkerExecutor{"mock": executor},
		logging.NoopLogger{},
		1,
		immediateCompletionPlanner{},
	)
	dispatch := interfaces.WorkDispatch{
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

	result, err := hook.OnTick(context.Background(), interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: 0,
		},
	})
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("hook result count = %d, want 1", len(result.Results))
	}
	if result.Results[0].Output != "executor-output" {
		t.Fatalf("hook result output = %q, want executor-output", result.Results[0].Output)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithPlannerDelaysDeliveryUntilDueTick(t *testing.T) {
	executor := &recordingExecutor{}
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		workers.NewWorkerPool(logging.NoopLogger{}),
		map[string]workers.WorkerExecutor{"mock": executor},
		logging.NoopLogger{},
		1,
		plannedCompletionPlanner{deliveryTick: 3, hasDeliveryTick: true},
	)
	dispatch := interfaces.WorkDispatch{
		DispatchID:   "dispatch-delayed",
		TransitionID: "t-process",
	}

	if err := hook.SubmitDispatch(context.Background(), dispatch); err != nil {
		t.Fatalf("SubmitDispatch: %v", err)
	}

	result, err := hook.OnTick(context.Background(), interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: 2,
		},
	})
	if err != nil {
		t.Fatalf("OnTick before due tick: %v", err)
	}
	if len(result.Results) != 0 {
		t.Fatalf("hook result count before due tick = %d, want 0", len(result.Results))
	}

	result, err = hook.OnTick(context.Background(), interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: 3,
		},
	})
	if err != nil {
		t.Fatalf("OnTick at due tick: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("hook result count at due tick = %d, want 1", len(result.Results))
	}
	if result.Results[0].Output != "executor-output" {
		t.Fatalf("hook result output at due tick = %q, want executor-output", result.Results[0].Output)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithPlannerUsesPlannedResultReplacement(t *testing.T) {
	executor := &recordingExecutor{}
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		workers.NewWorkerPool(logging.NoopLogger{}),
		map[string]workers.WorkerExecutor{"mock": executor},
		logging.NoopLogger{},
		1,
		plannedCompletionPlanner{
			plannedResult: interfaces.WorkResult{
				Outcome: interfaces.OutcomeAccepted,
				Output:  "planned-output",
			},
			hasPlannedResult: true,
		},
	)
	dispatch := interfaces.WorkDispatch{
		DispatchID:   "dispatch-planned-result",
		TransitionID: "t-process",
	}

	if err := hook.SubmitDispatch(context.Background(), dispatch); err != nil {
		t.Fatalf("SubmitDispatch: %v", err)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("executor call count = %d, want 1", len(executor.calls))
	}

	result, err := hook.OnTick(context.Background(), interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: 0,
		},
	})
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("hook result count = %d, want 1", len(result.Results))
	}
	if result.Results[0].Output != "planned-output" {
		t.Fatalf("hook result output = %q, want planned-output", result.Results[0].Output)
	}
}

func TestWorkerPoolDispatchResultHook_OnTickValidatesReplayTick(t *testing.T) {
	planner := &validatingCompletionPlanner{}
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		workers.NewWorkerPool(logging.NoopLogger{}),
		nil,
		logging.NoopLogger{},
		1,
		planner,
	)

	_, err := hook.OnTick(context.Background(), interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: 7,
		},
	})
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(planner.validatedTicks) != 1 || planner.validatedTicks[0] != 7 {
		t.Fatalf("validated ticks = %#v, want [7]", planner.validatedTicks)
	}

	expectedErr := errors.New("replay tick mismatch")
	planner.validateErr = expectedErr
	_, err = hook.OnTick(context.Background(), interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: 8,
		},
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("OnTick error = %v, want %v", err, expectedErr)
	}
	if len(planner.validatedTicks) != 2 || planner.validatedTicks[1] != 8 {
		t.Fatalf("validated ticks after error = %#v, want [7 8]", planner.validatedTicks)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithoutPlannerUsesWorkerPoolAsyncFlow(t *testing.T) {
	executor := &asyncRecordingExecutor{
		started: make(chan interfaces.WorkDispatch, 1),
		release: make(chan struct{}),
	}
	pool := workers.NewWorkerPool(logging.NoopLogger{})
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
		nil,
	)
	hook.Start(ctx)

	dispatch := interfaces.WorkDispatch{
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

	result, err := hook.OnTick(context.Background(), interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: 0,
		},
	})
	if err != nil {
		t.Fatalf("OnTick before release: %v", err)
	}
	if len(result.Results) != 0 {
		t.Fatalf("hook result count before worker completion = %d, want 0", len(result.Results))
	}

	close(executor.release)

	select {
	case <-hook.WaitCh():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async worker-pool completion signal")
	}

	result, err = hook.OnTick(context.Background(), interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: 0,
		},
	})
	if err != nil {
		t.Fatalf("OnTick after release: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("hook result count after worker completion = %d, want 1", len(result.Results))
	}
	if result.Results[0].Output != "async-executor-output" {
		t.Fatalf("hook result output = %q, want async-executor-output", result.Results[0].Output)
	}
}

func TestWorkerPoolDispatchResultHook_SubmitDispatchWithoutPlannerReturnsMissingRunnerError(t *testing.T) {
	hook := newWorkerPoolDispatchResultHook(
		buildSimpleNet(),
		workers.NewWorkerPool(logging.NoopLogger{}),
		nil,
		logging.NoopLogger{},
		1,
		nil,
	)

	err := hook.SubmitDispatch(context.Background(), interfaces.WorkDispatch{
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
