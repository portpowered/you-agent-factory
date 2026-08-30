package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestTickCallsSubsystem(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-1"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if sub.callCount != 1 {
		t.Errorf("expected subsystem called once, got %d", sub.callCount)
	}
	if sub.lastSnap == nil {
		t.Fatal("subsystem did not receive a marking snapshot")
	}

	tokensInInit := sub.lastSnap.Marking.TokensInPlace("task:init")
	if len(tokensInInit) != 1 {
		t.Fatalf("expected 1 token in task:init, got %d", len(tokensInInit))
	}
	if tokensInInit[0].Color.WorkTypeID != "task" {
		t.Errorf("expected WorkTypeID 'task', got %q", tokensInInit[0].Color.WorkTypeID)
	}
	if tokensInInit[0].Color.TraceID != "trace-1" {
		t.Errorf("expected TraceID 'trace-1', got %q", tokensInInit[0].Color.TraceID)
	}
}

func TestTickNRunsMultipleTicks(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	if err := engine.TickN(context.Background(), 3); err != nil {
		t.Fatalf("TickN() error: %v", err)
	}
	if sub.callCount != 3 {
		t.Errorf("expected 3 calls, got %d", sub.callCount)
	}
}

func TestTickUntilStopsOnPredicate(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	err := engine.TickUntil(context.Background(), func(snap *petri.MarkingSnapshot) bool {
		return snap.TickCount >= 2
	}, 10)
	if err != nil {
		t.Fatalf("TickUntil() error: %v", err)
	}
	if sub.callCount != 2 {
		t.Errorf("expected 2 calls, got %d", sub.callCount)
	}
}

func TestTickUntilReturnsErrorOnMaxTicks(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	err := engine.TickUntil(context.Background(), func(_ *petri.MarkingSnapshot) bool {
		return false
	}, 3)
	if err == nil {
		t.Fatal("expected error when predicate never satisfied")
	}
}

func TestSubsystemsSortedByTickGroup(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	var order []subsystems.TickGroup
	makeSub := func(g subsystems.TickGroup) *mockSubsystem {
		return &mockSubsystem{
			group: g,
			execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
				order = append(order, g)
				return &interfaces.TickResult{}, nil
			},
		}
	}

	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{
		makeSub(subsystems.TerminationCheck),
		makeSub(subsystems.CircuitBreaker),
		makeSub(subsystems.Scheduler),
	})
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	expected := []subsystems.TickGroup{subsystems.CircuitBreaker, subsystems.Scheduler, subsystems.TerminationCheck}
	if len(order) != len(expected) {
		t.Fatalf("expected %d subsystems called, got %d", len(expected), len(order))
	}
	for i, g := range expected {
		if order[i] != g {
			t.Errorf("position %d: expected TickGroup %d, got %d", i, g, order[i])
		}
	}
}

func TestTickWhileAutomaticTicksPaused_SkipsSubsystemExecution(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	paused := true
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if sub.callCount != 0 {
		t.Fatalf("subsystem callCount = %d, want 0 while automatic ticks are paused", sub.callCount)
	}

	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() after resume error: %v", err)
	}
	if sub.callCount != 1 {
		t.Fatalf("subsystem callCount = %d, want 1 after automatic ticks resume", sub.callCount)
	}
}

func TestTickWhileAutomaticTicksPaused_SkipsCascadeMutations(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(&factorytoken.Token{
		ID:      "parent-tok",
		PlaceID: "task:failed",
		Color:   factorytoken.Color{WorkID: "parent-work", WorkTypeID: "task"},
		History: newTestTokenHistory(),
	})
	marking.AddToken(&factorytoken.Token{
		ID:      "child-tok",
		PlaceID: "task:init",
		Color: factorytoken.Color{
			WorkID:     "child-work",
			WorkTypeID: "task",
			Relations: []work.Relation{{
				Type:          work.RelationDependsOn,
				TargetWorkID:  "parent-work",
				RequiredState: "complete",
			}},
		},
		History: newTestTokenHistory(),
	})

	engine := newTestFactoryEngine(
		n,
		marking,
		[]subsystems.Subsystem{subsystems.NewCascadingFailure(n, nil, time.Now)},
		WithAutomaticTicksPaused(func() bool { return true }),
	)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	child, ok := engine.GetMarking().Tokens["child-tok"]
	if !ok {
		t.Fatal("child token missing from marking")
	}
	if child.PlaceID != "task:init" {
		t.Fatalf("child place = %q, want task:init while paused (no cascade)", child.PlaceID)
	}
}

func newTestTokenHistory() factorytoken.History {
	return factorytoken.History{
		TotalVisits:         make(map[string]int),
		ConsecutiveFailures: make(map[string]int),
		PlaceVisits:         make(map[string]int),
	}
}

func TestMutationsAppliedBetweenSubsystems(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(&factorytoken.Token{
		ID:      "tok-1",
		PlaceID: "task:init",
		Color:   factorytoken.Color{WorkTypeID: "task"},
		History: factorytoken.History{
			TotalVisits:         make(map[string]int),
			ConsecutiveFailures: make(map[string]int),
			PlaceVisits:         make(map[string]int),
		},
	})

	mover := &mockSubsystem{
		group: subsystems.Scheduler,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Mutations: []interfaces.MarkingMutation{{
					Type:      interfaces.MutationMove,
					TokenID:   "tok-1",
					FromPlace: "task:init",
					ToPlace:   "task:complete",
				}},
			}, nil
		},
	}
	var observedPlace string
	observer := &mockSubsystem{
		group: subsystems.Tracer,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if tok, ok := snap.Marking.Tokens["tok-1"]; ok {
				observedPlace = tok.PlaceID
			}
			return &interfaces.TickResult{}, nil
		},
	}

	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{mover, observer})
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if observedPlace != "task:complete" {
		t.Errorf("expected observer to see token in 'task:complete', got %q", observedPlace)
	}
}

func TestResumeDrainsMultipleBufferedSubmissionsToQuiescence(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	traceIDs := []string{"trace-resume-a", "trace-resume-b", "trace-resume-c"}
	for _, traceID := range traceIDs {
		if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{
			WorkTypeID: "task",
			TraceID:    traceID,
		}}); err != nil {
			t.Fatalf("SubmitWorkRequest %s: %v", traceID, err)
		}
	}

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused with consumed wake: %v", err)
	}
	assertNoTokensInPlace(t, engine, "task:init")

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}

	snap := engine.GetMarking()
	tokens := (&snap).TokensInPlace("task:init")
	if len(tokens) != len(traceIDs) {
		t.Fatalf("tokens in task:init = %d, want %d after resume drain", len(tokens), len(traceIDs))
	}
	for i, wantTrace := range traceIDs {
		if tokens[i].Color.TraceID != wantTrace {
			t.Fatalf("token[%d] traceID = %q, want %q", i, tokens[i].Color.TraceID, wantTrace)
		}
	}
}

func TestWakeForPendingProcessing_SignalsDispatchHookBacklogAfterPausedWake(t *testing.T) {
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
					Dispatch: work.WorkDispatch{DispatchID: "d-hook-paused-wake", TransitionID: "t1", WorkerType: "test-worker"},
				}},
			}, nil
		},
	}

	hook := newTestDispatchResultHook()
	hook.submit = func(_ context.Context, dispatch work.WorkDispatch) error {
		hook.submits = append(hook.submits, dispatch)
		return nil
	}

	paused := false
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithAutomaticTicksPaused(func() bool { return paused }),
		WithDispatchResultHook(hook),
		WithDispatchHandler(func(work.WorkDispatch) {}),
	)

	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-hook-paused-wake",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.Run(ctx)
	}()

	if err := waitForRunningDispatch(t, engine, "d-hook-paused-wake", time.Second); err != nil {
		t.Fatalf("wait for dispatch before pause: %v", err)
	}

	paused = true
	hook.results = []workerexecution.WorkResult{{
		DispatchID:   "d-hook-paused-wake",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeAccepted,
	}}
	hook.SignalBufferedResults()

	time.Sleep(100 * time.Millisecond)
	if len(engine.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches while paused = %d, want 1", len(engine.RunningDispatches()))
	}
	if !hook.HasBufferedResults() {
		t.Fatal("dispatch hook backlog empty while paused, want buffered completion")
	}

	paused = false
	engine.WakeForPendingProcessing()
	if err := waitForNoRunningDispatches(t, engine, time.Second); err != nil {
		t.Fatalf("wait for dispatch hook drain after resume: %v", err)
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func TestRepeatedPausedWakePreservesBufferedSubmission(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{
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
					Dispatch: work.WorkDispatch{
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
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(work.WorkDispatch) {}),
		WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	)

	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{
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
	engine.GetResultBuffer().Write(context.Background(), workerexecution.WorkResult{
		DispatchID:   "dispatch-repeated-pause",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeAccepted,
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

func waitForRunningDispatch(t *testing.T, engine *FactoryEngine, dispatchID string, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := engine.RunningDispatches()[dispatchID]; ok {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for running dispatch %q", dispatchID)
}

func waitForNoRunningDispatches(t *testing.T, engine *FactoryEngine, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(engine.RunningDispatches()) == 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for running dispatches to drain, still have %d", len(engine.RunningDispatches()))
}

func TestEngine_LateDispatchResultGateOrdersTerminalPlacementBeforeRelease(t *testing.T) {
	const (
		workID     = "work-late-result"
		dispatchID = "dispatch-late-result"
	)
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	net := buildTestNet()
	net.Transitions["late-result"] = &petri.Transition{
		ID:         "late-result",
		Name:       "Late result",
		WorkerType: "worker",
		InputArcs: []petri.Arc{{
			ID: "input", Name: "work", PlaceID: "task:init", Direction: petri.ArcInput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
		OutputArcs: []petri.Arc{{
			ID: "accepted", PlaceID: "task:complete", Direction: petri.ArcOutput,
		}},
		FailureArcs: []petri.Arc{{
			ID: "failed", PlaceID: "task:failed", Direction: petri.ArcOutput,
		}},
	}

	marking := petri.NewMarking("late-result-test")
	source := &factorytoken.Token{
		ID: "late-result-token", PlaceID: "task:init", CreatedAt: now, EnteredAt: now,
		Color:   factorytoken.Color{WorkID: workID, WorkTypeID: "task", TraceID: "trace-late-result"},
		History: newTestTokenHistory(),
	}
	marking.AddToken(source)

	gate := newDeterministicResultGate(workerexecution.WorkResult{
		DispatchID: dispatchID, TransitionID: "late-result", Outcome: workerexecution.OutcomeFailed,
		Error: "late worker failure",
	})
	hook := newTestDispatchResultHook()
	hook.onTick = func(context.Context, interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) ([]workerexecution.WorkResult, error) {
		return []workerexecution.WorkResult{gate.waitForRelease()}, nil
	}
	transitioner := subsystems.NewTransitioner(
		net, nil, func() time.Time { return now },
		token_transformer.New(net.Places, net.WorkTypes, petri.NewWorkIDGenerator()),
		nil, nil, nil,
		factorydefinitions.WorkPropagationPolicyFunc(func(*factorydefinitions.FactoryWorkstationConfig) factorydefinitions.WorkPropagationMode {
			return factorydefinitions.WorkPropagationModeOutputAsPayload
		}),
	)
	engine := newTestFactoryEngine(
		net, marking, []subsystems.Subsystem{transitioner},
		WithDispatchResultHook(hook),
	)
	engine.runtimeState.Dispatches[dispatchID] = &interfaces.DispatchEntry{
		DispatchID: dispatchID, TransitionID: "late-result", StartTime: now,
		ConsumedTokens: factorytoken.ToWorkerSlice([]factorytoken.Token{factorytoken.Clone(*source)}),
		HeldMutations: []interfaces.MarkingMutation{{
			Type: interfaces.MutationConsume, TokenID: source.ID, FromPlace: source.PlaceID,
		}},
	}
	engine.runtimeState.InFlightCount = 1
	t.Cleanup(gate.releaseResult)

	tickDone := make(chan error, 1)
	go func() { tickDone <- engine.Tick(context.Background()) }()
	<-gate.ready
	select {
	case <-gate.delivered:
		t.Fatal("dispatch result was delivered before the terminal placement step")
	default:
	}

	if err := applyMutations(marking, net.Places, []interfaces.MarkingMutation{{
		Type: interfaces.MutationMove, TokenID: source.ID,
		FromPlace: "task:init", ToPlace: "task:complete",
	}}, now); err != nil {
		t.Fatalf("place Work in terminal state: %v", err)
	}
	gate.releaseResult()
	if err := <-tickDone; err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	assertIgnoredLateDispatchCompletion(t, engine.GetRuntimeStateSnapshot(), workID, dispatchID)
}

func TestEngine_LateDispatchResultWinsBeforeOperatorMove(t *testing.T) {
	const (
		workID     = "work-result-wins"
		dispatchID = "dispatch-result-wins"
	)
	now := time.Date(2026, time.August, 29, 12, 5, 0, 0, time.UTC)
	net := buildTestNet()
	net.Transitions["result-wins"] = &petri.Transition{
		ID:         "result-wins",
		Name:       "Result wins",
		WorkerType: "worker",
		InputArcs: []petri.Arc{{
			ID: "input", Name: "work", PlaceID: "task:init", Direction: petri.ArcInput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
		OutputArcs: []petri.Arc{{
			ID: "accepted", PlaceID: "task:complete", Direction: petri.ArcOutput,
		}},
		FailureArcs: []petri.Arc{{
			ID: "failed", PlaceID: "task:failed", Direction: petri.ArcOutput,
		}},
	}

	marking := petri.NewMarking("result-wins-test")
	source := &factorytoken.Token{
		ID: "result-wins-token", PlaceID: "task:init", CreatedAt: now, EnteredAt: now,
		Color:   factorytoken.Color{WorkID: workID, WorkTypeID: "task", TraceID: "trace-result-wins"},
		History: newTestTokenHistory(),
	}
	marking.AddToken(source)
	marking.RemoveToken(source.ID)

	resultReady := make(chan struct{})
	releaseResult := make(chan struct{})
	var resultReadyOnce sync.Once
	hook := newTestDispatchResultHook()
	hook.onTick = func(context.Context, interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) ([]workerexecution.WorkResult, error) {
		resultReadyOnce.Do(func() { close(resultReady) })
		<-releaseResult
		return []workerexecution.WorkResult{{
			DispatchID: dispatchID, TransitionID: "result-wins", Outcome: workerexecution.OutcomeAccepted,
		}}, nil
	}
	transitioner := subsystems.NewTransitioner(
		net, nil, func() time.Time { return now },
		token_transformer.New(net.Places, net.WorkTypes, petri.NewWorkIDGenerator()),
		nil, nil, nil,
		factorydefinitions.WorkPropagationPolicyFunc(func(*factorydefinitions.FactoryWorkstationConfig) factorydefinitions.WorkPropagationMode {
			return factorydefinitions.WorkPropagationModeOutputAsPayload
		}),
	)
	engine := newTestFactoryEngine(
		net, marking, []subsystems.Subsystem{transitioner},
		WithDispatchResultHook(hook),
	)
	engine.runtimeState.Dispatches[dispatchID] = &interfaces.DispatchEntry{
		DispatchID: dispatchID, TransitionID: "result-wins", StartTime: now,
		ConsumedTokens: factorytoken.ToWorkerSlice([]factorytoken.Token{factorytoken.Clone(*source)}),
		HeldMutations: []interfaces.MarkingMutation{{
			Type: interfaces.MutationConsume, TokenID: source.ID, FromPlace: source.PlaceID,
		}},
	}
	engine.runtimeState.InFlightCount = 1

	tickDone := make(chan error, 1)
	go func() { tickDone <- engine.Tick(context.Background()) }()
	<-resultReady

	moveDone := make(chan struct {
		result work.OperatorMoveResult
		err    error
	}, 1)
	go func() {
		result, err := engine.MoveWork(context.Background(), workID, "complete")
		moveDone <- struct {
			result work.OperatorMoveResult
			err    error
		}{result: result, err: err}
	}()
	close(releaseResult)

	if err := <-tickDone; err != nil {
		t.Fatalf("result-winning Tick() error: %v", err)
	}
	move := <-moveDone
	if move.err != nil {
		t.Fatalf("operator move after result winner: %v", move.err)
	}
	if move.result.FromState != "complete" || move.result.ToState != "complete" {
		t.Fatalf("move after result winner = %#v, want complete -> complete", move.result)
	}

	assertNormalLateDispatchCompletion(t, engine.GetRuntimeStateSnapshot(), workID)
}

func assertIgnoredLateDispatchCompletion(t *testing.T, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID, dispatchID string) {
	t.Helper()
	terminal := snapshot.Marking.TokensInPlace("task:complete")
	if len(terminal) != 1 || terminal[0].Color.WorkID != workID {
		t.Fatalf("terminal Work = %#v, want one %s token", terminal, workID)
	}
	if failed := snapshot.Marking.TokensInPlace("task:failed"); len(failed) != 0 {
		t.Fatalf("failed Work = %#v, want no late-result mutation", failed)
	}
	if len(snapshot.DispatchHistory) != 1 || snapshot.DispatchHistory[0].DispatchID != dispatchID {
		t.Fatalf("dispatch history = %#v, want one retired late result", snapshot.DispatchHistory)
	}
	completed := snapshot.DispatchHistory[0]
	if completed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("dispatch completion outcome = %q, want FAILED", completed.Outcome)
	}
	ignored := completed.IgnoredResult
	if ignored == nil || ignored.Reason != interfaces.DispatchResultIgnoredReasonWorkAlreadyTerminal ||
		ignored.ResultOutcome != workerexecution.OutcomeFailed ||
		ignored.ObservedState.Name != "complete" || ignored.ObservedState.Type != interfaces.StateTypeTerminal ||
		completed.IgnoredWorkID != workID {
		t.Fatalf("ignored result marker = %#v, work id = %q, want terminal %s marker", ignored, completed.IgnoredWorkID, workID)
	}
}

func assertNormalLateDispatchCompletion(t *testing.T, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID string) {
	t.Helper()
	terminal := snapshot.Marking.TokensInPlace("task:complete")
	if len(terminal) != 1 || terminal[0].Color.WorkID != workID {
		t.Fatalf("result-winning terminal Work = %#v, want one %s token", terminal, workID)
	}
	if failed := snapshot.Marking.TokensInPlace("task:failed"); len(failed) != 0 {
		t.Fatalf("result-winning failed Work = %#v, want none", failed)
	}
	if len(snapshot.DispatchHistory) != 1 || snapshot.DispatchHistory[0].IgnoredResult != nil {
		t.Fatalf("result-winning dispatch history = %#v, want one normal completion", snapshot.DispatchHistory)
	}
}
