package engine

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDispatchRecordsTrackedInRunningDispatches(t *testing.T) {
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
					Dispatch: work.WorkDispatch{DispatchID: "d1", TransitionID: "t1", WorkerType: "test-worker"},
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

	var dispatched []string
	eng := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(d work.WorkDispatch) {
			dispatched = append(dispatched, d.TransitionID)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if len(dispatched) != 1 || dispatched[0] != "t1" {
		t.Errorf("expected 1 dispatch for t1, got %v", dispatched)
	}

	running := eng.RunningDispatches()
	if len(running) != 1 {
		t.Fatalf("expected 1 running dispatch, got %d", len(running))
	}
	mutations, ok := running["d1"]
	if !ok {
		t.Fatal("expected running dispatch for d1")
	}
	if len(mutations) != 1 || mutations[0].TokenID != "tok-1" {
		t.Errorf("expected 1 mutation consuming tok-1, got %v", mutations)
	}

	eng.GetResultBuffer().Write(context.Background(), workerexecution.WorkResult{
		DispatchID:   "d1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeAccepted,
	})
	eng.NotifyResult()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if got := len(eng.RunningDispatches()); got != 0 {
		t.Errorf("expected 0 running dispatches after result, got %d", got)
	}
}

func TestDispatchResultHook_RecordsDispatchBeforeSubmittingToHook(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: work.WorkDispatch{
						DispatchID:   "dispatch-1",
						TransitionID: "transition-1",
						WorkerType:   "worker-a",
						Execution: work.ExecutionMetadata{
							TraceID:   "trace-1",
							WorkIDs:   []string{"work-1"},
							ReplayKey: "transition-1/trace-1/work-1",
						},
						InputTokens: workers.InputTokens(factorytoken.Token{
							ID:      "token-1",
							PlaceID: "task:init",
						}),
					},
					Mutations: []interfaces.MarkingMutation{{
						Type:      interfaces.MutationConsume,
						TokenID:   "token-1",
						FromPlace: "task:init",
					}},
				}},
			}, nil
		},
	}

	var records []interfaces.FactoryDispatchRecord
	hook := newTestDispatchResultHook()
	var eng *FactoryEngine
	hook.submit = func(_ context.Context, dispatch work.WorkDispatch) error {
		if len(records) != 1 {
			t.Fatalf("dispatch submitted before recorder observed it; record count = %d", len(records))
		}
		if _, ok := eng.runtimeState.Dispatches[dispatch.DispatchID]; !ok {
			t.Fatalf("dispatch %q submitted before engine running-dispatch tracking", dispatch.DispatchID)
		}
		return nil
	}

	eng = newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchResultHook(hook),
		WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
			records = append(records, record)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	assertSingleDispatchRecord(t, records, "dispatch-1")
	if len(hook.submits) != 1 {
		t.Fatalf("expected hook to receive 1 dispatch, got %d", len(hook.submits))
	}
	if hook.submits[0].Execution.DispatchCreatedTick != 1 || hook.submits[0].Execution.CurrentTick != 1 {
		t.Fatalf("hook dispatch execution metadata = %#v, want created/current tick 1", hook.submits[0].Execution)
	}
}

func TestDispatchEntry_SubmitsRawInterfacesWorkDispatch(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	inputToken := factorytoken.Token{
		ID:      "token-raw",
		PlaceID: "task:init",
		Color: factorytoken.Color{
			WorkID:     "work-raw",
			WorkTypeID: "task",
			TraceID:    "trace-raw",
		},
	}
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: work.WorkDispatch{
						DispatchID:      "dispatch-raw",
						TransitionID:    "transition-raw",
						WorkerType:      "worker-raw",
						WorkstationName: "station-raw",
						InputTokens:     workers.InputTokens(inputToken),
						InputBindings:   map[string][]string{"work": {"token-raw"}},
					},
					Mutations: []interfaces.MarkingMutation{{
						Type:      interfaces.MutationConsume,
						TokenID:   "token-raw",
						FromPlace: "task:init",
					}},
				}},
			}, nil
		},
	}

	hook := newTestDispatchResultHook()
	var handled []work.WorkDispatch
	eng := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchResultHook(hook),
		WithDispatchHandler(func(dispatch work.WorkDispatch) {
			handled = append(handled, dispatch)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if len(hook.submits) != 1 {
		t.Fatalf("hook submits = %d, want 1", len(hook.submits))
	}
	if len(handled) != 1 {
		t.Fatalf("handled dispatches = %d, want 1", len(handled))
	}

	for label, dispatch := range map[string]work.WorkDispatch{"hook": hook.submits[0], "handler": handled[0]} {
		if dispatch.DispatchID != "dispatch-raw" || dispatch.WorkerType != "worker-raw" {
			t.Fatalf("%s dispatch identity = %#v, want raw dispatch identity", label, dispatch)
		}
		if len(dispatch.InputBindings) == 0 {
			t.Fatalf("%s dispatch payload = %#v, want canonical dispatch-owned bindings preserved", label, dispatch)
		}
		if got := dispatch.InputBindings["work"]; len(got) != 1 || got[0] != "token-raw" {
			t.Fatalf("%s input bindings = %#v, want token-raw binding", label, dispatch.InputBindings)
		}
		tokens := workers.WorkDispatchInputTokens(dispatch)
		if len(tokens) != 1 || tokens[0].ID != "token-raw" {
			t.Fatalf("%s input tokens = %#v, want token-raw", label, tokens)
		}
		if dispatch.Execution.DispatchCreatedTick != 1 || dispatch.Execution.CurrentTick != 1 {
			t.Fatalf("%s dispatch execution = %#v, want tick metadata from raw entry", label, dispatch.Execution)
		}
	}
}

func TestDispatchResultHook_CompletionRecordedAtObservedTick(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	hook := newTestDispatchResultHook()
	hook.results = []workerexecution.WorkResult{{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		Outcome:      workerexecution.OutcomeAccepted,
	}}

	var records []interfaces.FactoryCompletionRecord
	observer := &mockSubsystem{
		group: subsystems.History,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if len(snap.Results) != 1 {
				t.Fatalf("expected completion result visible to subsystem, got %d results", len(snap.Results))
			}
			return &interfaces.TickResult{}, nil
		},
	}
	eng := newTestFactoryEngine(n, marking, []subsystems.Subsystem{observer},
		WithDispatchResultHook(hook),
		WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			records = append(records, record)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 completion record, got %d", len(records))
	}
	if records[0].ObservedTick != 1 {
		t.Fatalf("observed tick = %d, want 1", records[0].ObservedTick)
	}
	if records[0].DispatchID != "dispatch-1" {
		t.Fatalf("dispatch ID = %q, want dispatch-1", records[0].DispatchID)
	}
}

func TestTokenNamePopulatedOnDispatchAndCompletion(t *testing.T) {
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
					Dispatch: work.WorkDispatch{
						DispatchID:   "d1",
						TransitionID: "t1",
						WorkerType:   "test-worker",
						InputTokens: workers.InputTokens(factorytoken.Token{
							ID:      "tok-1",
							PlaceID: "task:init",
							Color: factorytoken.Color{
								Name:       "my-task-name",
								WorkID:     "work-1",
								WorkTypeID: "task",
							},
						}),
					},
					Mutations: []interfaces.MarkingMutation{{
						Type:      interfaces.MutationConsume,
						TokenID:   "tok-1",
						FromPlace: "task:init",
						Reason:    "consumed",
					}},
				}},
			}, nil
		},
	}

	eng := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub}, WithDispatchHandler(func(_ work.WorkDispatch) {}))
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	assertConsumedTokenName(t, eng.GetRuntimeStateSnapshot().Dispatches["d1"].ConsumedTokens, "my-task-name", "DispatchEntry")

	eng.GetResultBuffer().Write(context.Background(), workerexecution.WorkResult{
		DispatchID:   "d1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeAccepted,
	})
	eng.NotifyResult()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	snap := eng.GetRuntimeStateSnapshot()
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("expected 1 completed dispatch, got %d", len(snap.DispatchHistory))
	}
	assertConsumedTokenName(t, snap.DispatchHistory[0].ConsumedTokens, "my-task-name", "CompletedDispatch")
	if got := snap.DispatchHistory[0].ConsumedTokens[0].Color.WorkID; got != "work-1" {
		t.Errorf("expected work ID on CompletedDispatch = work-1, got %q", got)
	}
}

func TestDispatchRecordsAlwaysTracked(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: work.WorkDispatch{DispatchID: "d1", TransitionID: "t1", WorkerType: "test-worker"},
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

	var dispatched []string
	eng := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(d work.WorkDispatch) {
			dispatched = append(dispatched, d.TransitionID)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if len(dispatched) != 1 || dispatched[0] != "t1" {
		t.Errorf("expected 1 dispatch for t1, got %v", dispatched)
	}

	running := eng.RunningDispatches()
	if len(running) != 1 {
		t.Errorf("expected 1 running dispatch, got %d", len(running))
	}
	if _, ok := running["d1"]; !ok {
		t.Fatal("expected running dispatch for d1")
	}
}

func assertSingleDispatchRecord(t *testing.T, records []interfaces.FactoryDispatchRecord, wantID string) {
	t.Helper()
	if len(records) != 1 {
		t.Fatalf("expected 1 recorded dispatch, got %d", len(records))
	}
	record := records[0]
	if record.Dispatch.Execution.DispatchCreatedTick != 1 {
		t.Fatalf("dispatch execution created tick = %d, want 1", record.Dispatch.Execution.DispatchCreatedTick)
	}
	if record.Dispatch.Execution.CurrentTick != 1 {
		t.Fatalf("dispatch execution current tick = %d, want 1", record.Dispatch.Execution.CurrentTick)
	}
	if record.Dispatch.Execution.ReplayKey != "transition-1/trace-1/work-1" {
		t.Fatalf("dispatch execution replay key = %q, want transition-1/trace-1/work-1", record.Dispatch.Execution.ReplayKey)
	}
	if record.Dispatch.DispatchID != wantID {
		t.Fatalf("unexpected dispatch record: %#v", record)
	}
	if len(record.ConsumedTokens) != 1 || record.ConsumedTokens[0] != "token-1" {
		t.Fatalf("consumed tokens = %#v, want [token-1]", record.ConsumedTokens)
	}
}

func assertConsumedTokenName(t *testing.T, tokens []factorytoken.Token, wantName, label string) {
	t.Helper()
	if len(tokens) != 1 {
		t.Fatalf("expected 1 consumed token on %s, got %d", label, len(tokens))
	}
	if got := tokens[0].Color.Name; got != wantName {
		t.Errorf("expected token name on %s = %s, got %q", label, wantName, got)
	}
}

func TestResultWhileAutomaticTicksPaused_BuffersUntilResume(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: work.WorkDispatch{
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
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(work.WorkDispatch) {}),
		WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	)

	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{
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
	engine.GetResultBuffer().Write(context.Background(), workerexecution.WorkResult{
		DispatchID:   "dispatch-paused-result",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeAccepted,
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
					Dispatch: work.WorkDispatch{
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
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(work.WorkDispatch) {}),
		WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	)

	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{
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
	engine.GetResultBuffer().Write(context.Background(), workerexecution.WorkResult{
		DispatchID:   "dispatch-paused-wake",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeAccepted,
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
					Dispatch: work.WorkDispatch{
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
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchResultHook(hook),
		WithAutomaticTicksPaused(func() bool {
			return paused
		}),
	)

	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{
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
	hook.results = append(hook.results, workerexecution.WorkResult{
		DispatchID:   "dispatch-hook-paused",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeAccepted,
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
