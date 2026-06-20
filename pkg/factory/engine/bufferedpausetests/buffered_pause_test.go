package bufferedpausetests

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/engine"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestSubmitWhileAutomaticTicksPaused_AcceptsAndBuffersUntilResume(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	eng := engine.NewFactoryEngine(n, marking, []subsystems.Subsystem{sub}, engine.WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	request := interfaces.WorkRequest{
		RequestID: "request-paused-submit-001",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "paused-submit",
			WorkTypeID: "task",
			TraceID:    "trace-paused-submit",
		}},
	}
	result, err := eng.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("submit result accepted = false, want true")
	}
	if result.RequestID != request.RequestID {
		t.Fatalf("submit result requestID = %q, want %q", result.RequestID, request.RequestID)
	}

	assertNoTokensInPlace(t, eng, "task:init")
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	assertNoTokensInPlace(t, eng, "task:init")
	if sub.callCount != 0 {
		t.Fatalf("subsystem callCount = %d, want 0 while paused", sub.callCount)
	}

	paused = false
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume: %v", err)
	}
	snap := eng.GetMarking()
	tokens := (&snap).TokensInPlace("task:init")
	if len(tokens) != 1 {
		t.Fatalf("tokens in task:init = %d, want 1 after resume", len(tokens))
	}
	if tokens[0].Color.TraceID != "trace-paused-submit" {
		t.Fatalf("token traceID = %q, want trace-paused-submit", tokens[0].Color.TraceID)
	}
	if sub.callCount != 1 {
		t.Fatalf("subsystem callCount = %d, want 1 after resume", sub.callCount)
	}

	repeated, err := eng.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate SubmitWorkRequest: %v", err)
	}
	if repeated.Accepted {
		t.Fatal("duplicate submit should be idempotent no-op")
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after duplicate submit: %v", err)
	}
	snap = eng.GetMarking()
	if len((&snap).TokensInPlace("task:init")) != 1 {
		t.Fatalf("token count after duplicate submit = %d, want 1", len((&snap).TokensInPlace("task:init")))
	}
}

func TestWakeForPendingProcessing_SignalsBufferedSubmissionAfterPausedWake(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	eng := engine.NewFactoryEngine(n, marking, []subsystems.Subsystem{sub}, engine.WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	if _, err := submitWorkRequests(context.Background(), eng, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-paused-wake",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	assertNoTokensInPlace(t, eng, "task:init")

	paused = false
	eng.WakeForPendingProcessing()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	snap := eng.GetMarking()
	if len((&snap).TokensInPlace("task:init")) != 1 {
		t.Fatalf("buffered submission was not reachable after paused wake and resume")
	}
}

func TestResultWhileAutomaticTicksPaused_BuffersUntilResume(t *testing.T) {
	runPausedResultTest(t, "dispatch-paused-result", "trace-paused-result", false)
}

func TestWakeForPendingProcessing_SignalsBufferedResultAfterPausedWake(t *testing.T) {
	runPausedResultTest(t, "dispatch-paused-wake", "trace-paused-result-wake", true)
}

func TestDispatchResultHookWhileAutomaticTicksPaused_BuffersUntilResume(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := dispatchSubsystem(dispatchExecFn("dispatch-hook-paused"))
	hook := newTestDispatchResultHook()

	paused := true
	eng := engine.NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		engine.WithDispatchResultHook(hook),
		engine.WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	)

	if _, err := submitWorkRequests(context.Background(), eng, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-hook-paused-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	paused = false
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick to dispatch: %v", err)
	}
	if len(eng.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want 1 after dispatch", len(eng.RunningDispatches()))
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
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused with buffered hook result: %v", err)
	}
	if len(eng.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want 1 while paused", len(eng.RunningDispatches()))
	}
	if len(eng.GetRuntimeStateSnapshot().DispatchHistory) != 0 {
		t.Fatal("dispatch completed while paused")
	}

	paused = false
	eng.WakeForPendingProcessing()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume: %v", err)
	}
	if len(eng.RunningDispatches()) != 0 {
		t.Fatalf("running dispatches = %d, want 0 after resume", len(eng.RunningDispatches()))
	}
	if len(eng.GetRuntimeStateSnapshot().DispatchHistory) != 1 {
		t.Fatalf("dispatch history = %d, want 1 after resume", len(eng.GetRuntimeStateSnapshot().DispatchHistory))
	}
}

func TestRepeatedPausedWakePreservesBufferedSubmission(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	eng := engine.NewFactoryEngine(n, marking, []subsystems.Subsystem{sub}, engine.WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	if _, err := submitWorkRequests(context.Background(), eng, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-repeated-pause-submit",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	for range 3 {
		if err := eng.Tick(context.Background()); err != nil {
			t.Fatalf("Tick while paused: %v", err)
		}
		assertNoTokensInPlace(t, eng, "task:init")
	}

	paused = false
	eng.WakeForPendingProcessing()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	snap := eng.GetMarking()
	if len((&snap).TokensInPlace("task:init")) != 1 {
		t.Fatalf("buffered submission was not reachable after repeated paused wakes")
	}
}

func TestRepeatedPausedWakePreservesBufferedResult(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := dispatchSubsystem(dispatchExecFn("dispatch-repeated-pause"))

	paused := true
	eng := engine.NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		engine.WithDispatchHandler(func(interfaces.WorkDispatch) {}),
		engine.WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	)

	if _, err := submitWorkRequests(context.Background(), eng, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-repeated-pause-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	paused = false
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick to dispatch: %v", err)
	}

	paused = true
	eng.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "dispatch-repeated-pause",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	eng.NotifyResult()
	for range 3 {
		if err := eng.Tick(context.Background()); err != nil {
			t.Fatalf("Tick while paused: %v", err)
		}
		if len(eng.GetRuntimeStateSnapshot().DispatchHistory) != 0 {
			t.Fatal("dispatch completed while paused")
		}
	}

	paused = false
	eng.WakeForPendingProcessing()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	if len(eng.GetRuntimeStateSnapshot().DispatchHistory) != 1 {
		t.Fatalf("buffered result was not reachable after repeated paused wakes")
	}
}

func runPausedResultTest(t *testing.T, dispatchID, traceID string, wakeOnly bool) {
	t.Helper()

	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := dispatchSubsystem(dispatchExecFn(dispatchID))

	paused := true
	opts := []engine.Option{
		engine.WithDispatchHandler(func(interfaces.WorkDispatch) {}),
		engine.WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	}
	eng := engine.NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub}, opts...)

	if _, err := submitWorkRequests(context.Background(), eng, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    traceID,
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	paused = false
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick to dispatch: %v", err)
	}
	if len(eng.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want 1 after dispatch", len(eng.RunningDispatches()))
	}

	paused = true
	eng.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   dispatchID,
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	eng.NotifyResult()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused with buffered result: %v", err)
	}
	if len(eng.GetRuntimeStateSnapshot().DispatchHistory) != 0 {
		t.Fatal("dispatch completed while paused")
	}
	if !wakeOnly {
		if len(eng.RunningDispatches()) != 1 {
			t.Fatalf("running dispatches = %d, want 1 while paused", len(eng.RunningDispatches()))
		}
	}

	paused = false
	eng.WakeForPendingProcessing()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume: %v", err)
	}
	if len(eng.RunningDispatches()) != 0 {
		t.Fatalf("running dispatches = %d, want 0 after resume", len(eng.RunningDispatches()))
	}
	snap := eng.GetRuntimeStateSnapshot()
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("dispatch history = %d, want 1 after resume", len(snap.DispatchHistory))
	}
	if !wakeOnly && snap.DispatchHistory[0].DispatchID != dispatchID {
		t.Fatalf("completed dispatch ID = %q, want %s", snap.DispatchHistory[0].DispatchID, dispatchID)
	}
}

func dispatchExecFn(dispatchID string) func(context.Context, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	return func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
		return &interfaces.TickResult{
			Dispatches: []interfaces.DispatchRecord{{
				Dispatch: interfaces.WorkDispatch{
					DispatchID:   dispatchID,
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
	}
}

func assertNoTokensInPlace(t *testing.T, eng *engine.FactoryEngine, placeID string) {
	t.Helper()
	snap := eng.GetMarking()
	if got := len((&snap).TokensInPlace(placeID)); got != 0 {
		t.Fatalf("tokens in %s = %d, want 0 while paused", placeID, got)
	}
}
