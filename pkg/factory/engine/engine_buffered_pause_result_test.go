package engine

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestResultWhileAutomaticTicksPaused_BuffersUntilResume(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: interfaces.WorkDispatch{
						DispatchID:   "dispatch-paused-result",
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
		TraceID:    "trace-paused-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick to dispatch: %v", err)
	}
	if len(engine.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want 1 after dispatch", len(engine.RunningDispatches()))
	}

	paused = true
	engine.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "dispatch-paused-result",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	engine.NotifyResult()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused with buffered result: %v", err)
	}
	if len(engine.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want 1 while paused", len(engine.RunningDispatches()))
	}
	snap := engine.GetRuntimeStateSnapshot()
	if len(snap.DispatchHistory) != 0 {
		t.Fatalf("dispatch history = %d, want 0 while paused", len(snap.DispatchHistory))
	}

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume: %v", err)
	}
	if len(engine.RunningDispatches()) != 0 {
		t.Fatalf("running dispatches = %d, want 0 after resume", len(engine.RunningDispatches()))
	}
	snap = engine.GetRuntimeStateSnapshot()
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("dispatch history = %d, want 1 after resume", len(snap.DispatchHistory))
	}
	if snap.DispatchHistory[0].DispatchID != "dispatch-paused-result" {
		t.Fatalf("completed dispatch ID = %q, want dispatch-paused-result", snap.DispatchHistory[0].DispatchID)
	}
}

func TestWakeForPendingProcessing_SignalsBufferedResultAfterPausedWake(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: interfaces.WorkDispatch{
						DispatchID:   "dispatch-paused-wake",
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
		TraceID:    "trace-paused-result-wake",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick to dispatch: %v", err)
	}

	paused = true
	engine.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "dispatch-paused-wake",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	engine.NotifyResult()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	if len(engine.GetRuntimeStateSnapshot().DispatchHistory) != 0 {
		t.Fatal("dispatch completed while paused")
	}

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	if len(engine.GetRuntimeStateSnapshot().DispatchHistory) != 1 {
		t.Fatalf("buffered result was not reachable after paused wake and resume")
	}
}

func TestDispatchResultHookWhileAutomaticTicksPaused_BuffersUntilResume(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: interfaces.WorkDispatch{
						DispatchID:   "dispatch-hook-paused",
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
	hook := newTestDispatchResultHook()

	paused := true
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchResultHook(hook),
		WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	)

	if _, err := submitWorkRequests(context.Background(), engine, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-hook-paused-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick to dispatch: %v", err)
	}
	if len(engine.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want 1 after dispatch", len(engine.RunningDispatches()))
	}

	paused = true
	hook.results = append(hook.results, interfaces.WorkResult{
		DispatchID:   "dispatch-hook-paused",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	select {
	case hook.waitCh <- struct{}{}:
	default:
	}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused with buffered hook result: %v", err)
	}
	if len(engine.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want 1 while paused", len(engine.RunningDispatches()))
	}
	if len(engine.GetRuntimeStateSnapshot().DispatchHistory) != 0 {
		t.Fatal("dispatch completed while paused")
	}

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume: %v", err)
	}
	if len(engine.RunningDispatches()) != 0 {
		t.Fatalf("running dispatches = %d, want 0 after resume", len(engine.RunningDispatches()))
	}
	if len(engine.GetRuntimeStateSnapshot().DispatchHistory) != 1 {
		t.Fatalf("dispatch history = %d, want 1 after resume", len(engine.GetRuntimeStateSnapshot().DispatchHistory))
	}
}
