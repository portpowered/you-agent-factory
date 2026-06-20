package engine

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestRepeatedPausedWakePreservesBufferedSubmission(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	if _, err := submitWorkRequests(context.Background(), engine, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-repeated-pause-submit",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	for range 3 {
		if err := engine.Tick(context.Background()); err != nil {
			t.Fatalf("Tick while paused: %v", err)
		}
		assertNoTokensInPlace(t, engine, "task:init")
	}

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	snap := engine.GetMarking()
	if len((&snap).TokensInPlace("task:init")) != 1 {
		t.Fatalf("buffered submission was not reachable after repeated paused wakes")
	}
}

func TestRepeatedPausedWakePreservesBufferedResult(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: interfaces.WorkDispatch{
						DispatchID:   "dispatch-repeated-pause",
						TransitionID: "t1",
						WorkerType:   "test-worker",
					},
					Mutations: []interfaces.MarkingMutation{{
						Type:      interfaces.MutationConsume,
						TokenID:   "tok-1",
						FromPlace: "task:init",
					}},
				}},
			}, nil
		},
	}

	paused := true
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(interfaces.WorkDispatch) {}),
		WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	)

	if _, err := submitWorkRequests(context.Background(), engine, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-repeated-pause-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick to dispatch: %v", err)
	}

	paused = true
	engine.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "dispatch-repeated-pause",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	engine.NotifyResult()
	for range 3 {
		if err := engine.Tick(context.Background()); err != nil {
			t.Fatalf("Tick while paused: %v", err)
		}
		if len(engine.GetRuntimeStateSnapshot().DispatchHistory) != 0 {
			t.Fatal("dispatch completed while paused")
		}
	}

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	if len(engine.GetRuntimeStateSnapshot().DispatchHistory) != 1 {
		t.Fatalf("buffered result was not reachable after repeated paused wakes")
	}
}
