package engine

import (
	"context"
	"testing"

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
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
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
	result, err := engine.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("submit result accepted = false, want true")
	}
	if result.RequestID != request.RequestID {
		t.Fatalf("submit result requestID = %q, want %q", result.RequestID, request.RequestID)
	}

	assertNoTokensInPlace(t, engine, "task:init")
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	assertNoTokensInPlace(t, engine, "task:init")
	if sub.callCount != 0 {
		t.Fatalf("subsystem callCount = %d, want 0 while paused", sub.callCount)
	}

	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume: %v", err)
	}
	snap := engine.GetMarking()
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

	repeated, err := engine.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate SubmitWorkRequest: %v", err)
	}
	if repeated.Accepted {
		t.Fatal("duplicate submit should be idempotent no-op")
	}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after duplicate submit: %v", err)
	}
	snap = engine.GetMarking()
	if len((&snap).TokensInPlace("task:init")) != 1 {
		t.Fatalf("token count after duplicate submit = %d, want 1", len((&snap).TokensInPlace("task:init")))
	}
}

func TestWakeForPendingProcessing_SignalsBufferedSubmissionAfterPausedWake(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	if _, err := submitWorkRequests(context.Background(), engine, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-paused-wake",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	assertNoTokensInPlace(t, engine, "task:init")

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	snap := engine.GetMarking()
	if len((&snap).TokensInPlace("task:init")) != 1 {
		t.Fatal("buffered submission was not reachable after paused wake and resume")
	}
}

func TestResultWhileAutomaticTicksPaused_BuffersUntilResume(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	alreadyDispatched := false
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if alreadyDispatched {
				return nil, nil
			}
			alreadyDispatched = true
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: interfaces.WorkDispatch{DispatchID: "d-paused-result", TransitionID: "t1", WorkerType: "test-worker"},
					Mutations: []interfaces.MarkingMutation{{
						Type:      interfaces.MutationConsume,
						TokenID:   "tok-1",
						FromPlace: "task:init",
						Reason:    "consumed by transition t1",
					}},
				}},
			}, nil
		},
	}

	paused := false
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithAutomaticTicksPaused(func() bool { return paused }),
		WithDispatchHandler(func(interfaces.WorkDispatch) {}),
	)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("initial Tick: %v", err)
	}
	if len(engine.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want 1 before pause", len(engine.RunningDispatches()))
	}

	paused = true
	engine.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "d-paused-result",
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
	runtimeSnap := engine.GetRuntimeStateSnapshot()
	if len(runtimeSnap.Results) != 0 {
		t.Fatalf("observed results = %d, want 0 while paused", len(runtimeSnap.Results))
	}
	if !engine.GetResultBuffer().HasData() {
		t.Fatal("result buffer drained while paused")
	}

	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume: %v", err)
	}
	if len(engine.RunningDispatches()) != 0 {
		t.Fatalf("running dispatches = %d, want 0 after resume", len(engine.RunningDispatches()))
	}
}

func TestWakeForPendingProcessing_SignalsBufferedResultAfterPausedWake(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	alreadyDispatched := false
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if alreadyDispatched {
				return nil, nil
			}
			alreadyDispatched = true
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: interfaces.WorkDispatch{DispatchID: "d-paused-wake-result", TransitionID: "t1", WorkerType: "test-worker"},
				}},
			}, nil
		},
	}

	paused := false
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithAutomaticTicksPaused(func() bool { return paused }),
		WithDispatchHandler(func(interfaces.WorkDispatch) {}),
	)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("initial Tick: %v", err)
	}

	paused = true
	engine.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "d-paused-wake-result",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	engine.NotifyResult()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	if len(engine.RunningDispatches()) != 1 {
		t.Fatal("buffered result retired dispatch while paused")
	}

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	if len(engine.RunningDispatches()) != 0 {
		t.Fatal("buffered result was not reachable after paused wake and resume")
	}
}

func assertNoTokensInPlace(t *testing.T, engine *FactoryEngine, placeID string) {
	t.Helper()
	snap := engine.GetMarking()
	if got := len((&snap).TokensInPlace(placeID)); got != 0 {
		t.Fatalf("tokens in %s = %d, want 0 while paused", placeID, got)
	}
}
