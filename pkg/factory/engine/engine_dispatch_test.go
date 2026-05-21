package engine

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
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
					Dispatch: interfaces.WorkDispatch{DispatchID: "d1", TransitionID: "t1", WorkerType: "test-worker"},
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
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(d interfaces.WorkDispatch) {
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

	eng.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "d1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
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
					Dispatch: interfaces.WorkDispatch{
						DispatchID:   "dispatch-1",
						TransitionID: "transition-1",
						WorkerType:   "worker-a",
						Execution: interfaces.ExecutionMetadata{
							TraceID:   "trace-1",
							WorkIDs:   []string{"work-1"},
							ReplayKey: "transition-1/trace-1/work-1",
						},
						InputTokens: workers.InputTokens(interfaces.Token{
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
	hook.submit = func(_ context.Context, dispatch interfaces.WorkDispatch) error {
		if len(records) != 1 {
			t.Fatalf("dispatch submitted before recorder observed it; record count = %d", len(records))
		}
		if _, ok := eng.runtimeState.Dispatches[dispatch.DispatchID]; !ok {
			t.Fatalf("dispatch %q submitted before engine running-dispatch tracking", dispatch.DispatchID)
		}
		return nil
	}

	eng = NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
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
	inputToken := interfaces.Token{
		ID:      "token-raw",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
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
					Dispatch: interfaces.WorkDispatch{
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
	var handled []interfaces.WorkDispatch
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchResultHook(hook),
		WithDispatchHandler(func(dispatch interfaces.WorkDispatch) {
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

	for label, dispatch := range map[string]interfaces.WorkDispatch{"hook": hook.submits[0], "handler": handled[0]} {
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
	hook.results = []interfaces.WorkResult{{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		Outcome:      interfaces.OutcomeAccepted,
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
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{observer},
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
					Dispatch: interfaces.WorkDispatch{
						DispatchID:   "d1",
						TransitionID: "t1",
						WorkerType:   "test-worker",
						InputTokens: workers.InputTokens(interfaces.Token{
							ID:      "tok-1",
							PlaceID: "task:init",
							Color: interfaces.TokenColor{
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

	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub}, WithDispatchHandler(func(_ interfaces.WorkDispatch) {}))
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	assertConsumedTokenName(t, eng.GetRuntimeStateSnapshot().Dispatches["d1"].ConsumedTokens, "my-task-name", "DispatchEntry")

	eng.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "d1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
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
					Dispatch: interfaces.WorkDispatch{DispatchID: "d1", TransitionID: "t1", WorkerType: "test-worker"},
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
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(d interfaces.WorkDispatch) {
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

func assertConsumedTokenName(t *testing.T, tokens []interfaces.Token, wantName, label string) {
	t.Helper()
	if len(tokens) != 1 {
		t.Fatalf("expected 1 consumed token on %s, got %d", label, len(tokens))
	}
	if got := tokens[0].Color.Name; got != wantName {
		t.Errorf("expected token name on %s = %s, got %q", label, wantName, got)
	}
}
