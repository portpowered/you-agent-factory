package engine

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
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
		[]subsystems.Subsystem{subsystems.NewCascadingFailure(n, logging.NoopLogger{}, time.Now)},
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

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("initial dispatch tick: %v", err)
	}
	paused = true
	hook.results = []workerexecution.WorkResult{{
		DispatchID: "d-hook-paused-wake", TransitionID: "t1", Outcome: workerexecution.OutcomeAccepted,
	}}
	hook.SignalBufferedResults()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("paused result tick: %v", err)
	}
	if len(engine.RunningDispatches()) != 0 || hook.HasBufferedResults() {
		t.Fatal("paused tick did not drain and retire the dispatch result")
	}
	if dispatchSub.callCount != 1 {
		t.Fatalf("dispatcher calls = %d, want 1 before resume", dispatchSub.callCount)
	}
	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("resume tick: %v", err)
	}
	if len(engine.GetRuntimeStateSnapshot().DispatchHistory) != 1 {
		t.Fatal("resume duplicated completion")
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

func TestRepeatedPausedWakeCompletesResultOnce(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if len(snapshot.DispatchHistory) > 0 {
				return nil, nil
			}
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
		if len(engine.GetRuntimeStateSnapshot().DispatchHistory) != 1 {
			t.Fatal("paused wake did not preserve exactly one completion")
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

func TestFactoryEngineSelectedLoggerTickParity(t *testing.T) {
	t.Parallel()
	var outcomes []interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	for _, mode := range []string{"capture", "noop"} {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := logging.NewZapLogger(zap.New(core).With(zap.String("session_id", "ticks")), false)
		if mode == "noop" {
			logger = logging.NoopLogger{}
		}
		outcomes = append(outcomes, runSelectedLoggerTicks(t, logger, "ticks"))
		if mode == "noop" {
			if logs.Len() != 0 {
				t.Fatal("quiet tick leaked diagnostics")
			}
			continue
		}
		assertSelectedTickDiagnostics(t, logs)
	}
	// Topology instances are fresh, but their values and all projected state agree.
	if !reflect.DeepEqual(outcomes[0], outcomes[1]) {
		t.Fatalf("capture/Noop state differs: %+v / %+v", outcomes[0], outcomes[1])
	}
}

func runSelectedLoggerTicks(t *testing.T, logger logging.Logger, scope string) interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	marking := petri.NewMarking(scope)
	dispatchID := scope + "-dispatch"
	var effects []string
	hook := newTestDispatchResultHook()
	var engine *FactoryEngine
	hook.submit = func(_ context.Context, dispatch work.WorkDispatch) error {
		if engine.runtimeState.Dispatches[dispatch.DispatchID] == nil {
			t.Fatal("dispatch forwarded before registration")
		}
		effects = append(effects, "forward:"+dispatch.DispatchID)
		return nil
	}
	mover := &mockSubsystem{group: subsystems.Scheduler, execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
		if snap.TickCount > 1 {
			return nil, nil
		}
		effects = append(effects, "move")
		return &interfaces.TickResult{
			Mutations:  []interfaces.MarkingMutation{{Type: interfaces.MutationMove, TokenID: "tok-task-1", FromPlace: "task:init", ToPlace: "task:complete"}},
			Dispatches: []interfaces.DispatchRecord{{Dispatch: work.WorkDispatch{DispatchID: dispatchID, TransitionID: "transition", WorkerType: "script", InputTokens: workerexecution.InputTokens(factorytoken.ToWorker(*snap.Marking.Tokens["tok-task-1"])), Execution: work.ExecutionMetadata{TraceID: scope + "-trace", WorkIDs: []string{scope + "-work"}}}}},
		}, nil
	}}
	reader := &mockSubsystem{group: subsystems.Tracer, execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
		effects = append(effects, snap.Marking.Tokens["tok-task-1"].PlaceID)
		if snap.TickCount == 2 && (len(snap.Results) != 1 || snap.Dispatches[dispatchID] == nil) {
			t.Fatal("completion retired before subsystem observation")
		}
		return &interfaces.TickResult{}, nil
	}}
	paused := true
	engine = newTestFactoryEngineWithLogger(buildTestNet(), marking, []subsystems.Subsystem{reader, mover}, logger,
		WithAutomaticTicksPaused(func() bool { return paused }), WithDispatchResultHook(hook),
		WithDispatchRecorder(func(rec interfaces.FactoryDispatchRecord) { effects = append(effects, "record:"+rec.DispatchID) }),
		WithCompletionRecorder(func(rec interfaces.FactoryCompletionRecord) { effects = append(effects, "completion:"+rec.DispatchID) }),
		func(e *FactoryEngine) {
			e.recordResponse = func(_ int, result workerexecution.WorkResult, completed interfaces.CompletedDispatch) {
				effects = append(effects, "response:"+result.DispatchID)
			}
		},
	)
	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{RequestID: scope + "-request", WorkID: scope + "-work", WorkTypeID: "task", TraceID: scope + "-trace"}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(effects) != 0 {
		t.Fatal("paused tick executed effects")
	}
	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	hook.results = []workerexecution.WorkResult{{DispatchID: dispatchID, TransitionID: "transition", Outcome: workerexecution.OutcomeAccepted}}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := engine.GetRuntimeStateSnapshot()
	assertSelectedTickEffects(t, effects, snap, hook, scope)
	return snap
}

func assertSelectedTickDiagnostics(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()
	for _, message := range []string{"engine: [START] running engine tick", "engine: [END] tick complete"} {
		entries := logs.FilterMessage(message).All()
		if len(entries) != 2 {
			t.Fatalf("%s = %+v", message, entries)
		}
		for i, entry := range entries {
			if entry.Level != zapcore.InfoLevel || entry.ContextMap()["tick"] != int64(i+1) {
				t.Fatalf("tick diagnostic = %+v", entry)
			}
		}
	}
	entries := logs.FilterMessage("engine: executing subsystem").All()
	if len(entries) != 4 || logs.FilterMessage("engine: skipping automatic tick while factory is paused").Len() != 1 {
		t.Fatalf("subsystem/pause diagnostics = %+v", logs.All())
	}
	for _, entry := range entries {
		if entry.Level != zapcore.DebugLevel || entry.ContextMap()["subsystem"] == nil {
			t.Fatalf("subsystem diagnostic = %+v", entry)
		}
	}
}

func TestFactoryEngineSelectedLoggerScopeIsolation(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zapcore.DebugLevel)
	for _, scope := range []string{"first", "second", "quiet"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			logger := logging.NewZapLogger(zap.New(core).With(zap.String("session_id", scope), zap.String("folder_path", "/"+scope), zap.String("factory_dir", "/factory/"+scope)), false)
			if scope == "quiet" {
				logger = logging.NoopLogger{}
			}
			runSelectedLoggerTicks(t, logger, scope)
		})
	}
	t.Cleanup(func() {
		if logs.Len() != 18 {
			t.Fatalf("scoped records = %+v", logs.All())
		}
		for _, entry := range logs.All() {
			fields := entry.ContextMap()
			scope := fields["session_id"]
			if scope != "first" && scope != "second" {
				t.Fatalf("unexpected scope: %+v", entry)
			}
			if fields["folder_path"] != "/"+scope.(string) || fields["factory_dir"] != "/factory/"+scope.(string) {
				t.Fatalf("crossed context: %+v", entry)
			}
		}
	})
}

func assertSelectedTickEffects(t *testing.T, effects []string, snap interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], hook *testDispatchResultHook, scope string) {
	t.Helper()
	want := []string{"move", "record:" + scope + "-dispatch", "forward:" + scope + "-dispatch", "task:complete", "completion:" + scope + "-dispatch", "task:complete", "response:" + scope + "-dispatch"}
	if !reflect.DeepEqual(effects, want) {
		t.Fatalf("effects = %v, want %v", effects, want)
	}
	if len(snap.Dispatches) != 0 || len(snap.DispatchHistory) != 1 || snap.DispatchHistory[0].DispatchID != scope+"-dispatch" || snap.DispatchHistory[0].ConsumedTokens[0].Color.WorkID != scope+"-work" {
		t.Fatalf("retirement or identity = %+v", snap)
	}
	if hook.submits[0].Execution.TraceID != scope+"-trace" || hook.submits[0].Execution.DispatchCreatedTick != 1 {
		t.Fatalf("dispatch identity = %+v", hook.submits)
	}
}

// The engine owns result draining and phase ordering; routing is a controlled
// collaborator that produces the same marking/completion contract as production.
func TestPausedResultAppliesWorkMutationWithoutSchedulingOrTermination(t *testing.T) {
	marking := petri.NewMarking("test-wf")
	marking.AddToken(&factorytoken.Token{ID: "work-1", PlaceID: "task:init", Color: factorytoken.Color{WorkTypeID: "task"}})
	scheduler := &mockSubsystem{group: subsystems.Dispatcher}
	termination := &mockSubsystem{group: subsystems.TerminationCheck}
	router := &mockSubsystem{group: subsystems.Transitioner, execFn: func(_ context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
		if len(snapshot.Results) == 0 {
			return nil, nil
		}
		if len(snapshot.Results) != 1 || snapshot.Results[0].DispatchID != "dispatch-1" {
			t.Fatalf("routing results = %#v, want one correlated result", snapshot.Results)
		}
		return &interfaces.TickResult{
			Mutations:           []interfaces.MarkingMutation{{Type: interfaces.MutationMove, TokenID: "work-1", FromPlace: "task:init", ToPlace: "task:complete"}},
			CompletedDispatches: []interfaces.CompletedDispatch{{DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workerexecution.OutcomeAccepted}},
		}, nil
	}}
	engine := newTestFactoryEngine(buildTestNet(), marking, []subsystems.Subsystem{scheduler, router, termination}, WithAutomaticTicksPaused(func() bool { return true }))
	engine.runtimeState.Dispatches["dispatch-1"] = &interfaces.DispatchEntry{DispatchID: "dispatch-1", TransitionID: "t1"}
	engine.runtimeState.InFlightCount = 1
	engine.GetResultBuffer().Write(context.Background(), workerexecution.WorkResult{DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workerexecution.OutcomeAccepted})
	engine.NotifyResult()
	for range 2 {
		if err := engine.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := engine.GetRuntimeStateSnapshot()
	if got := snapshot.Marking.Tokens["work-1"].PlaceID; got != "task:complete" {
		t.Fatalf("Work location = %q, want task:complete before resume", got)
	}
	if len(snapshot.DispatchHistory) != 1 || len(snapshot.Dispatches) != 0 || snapshot.InFlightCount != 0 {
		t.Fatalf("completion bookkeeping = %#v", snapshot)
	}
	if scheduler.callCount != 0 || termination.callCount != 0 {
		t.Fatalf("paused phase calls: scheduler=%d termination=%d", scheduler.callCount, termination.callCount)
	}
}
