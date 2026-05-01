package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/logging"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/workers"
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

type validatingCompletionPlanner struct {
	validatedTicks []int
	validateErr    error
}

func (p *validatingCompletionPlanner) DeliveryTickForDispatch(interfaces.WorkDispatch) (int, bool, error) {
	return 0, false, nil
}

func (p *validatingCompletionPlanner) ValidateReplayTick(currentTick int) error {
	p.validatedTicks = append(p.validatedTicks, currentTick)
	return p.validateErr
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
