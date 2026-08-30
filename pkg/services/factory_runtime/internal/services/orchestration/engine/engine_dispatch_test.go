package engine

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestEngine_SameWorkObserveAndConsumeDispatchesInOneTick(t *testing.T) {
	net := sameWorkObserveConsumeNet()
	marking := petri.NewMarking("test-wf")
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	marking.AddToken(testDispatchToken("work-token", "task:init", factorytoken.DataTypeWork, now))
	marking.AddToken(testDispatchToken("slot-token", "slot:available", factorytoken.DataTypeResource, now))

	dispatcher := subsystems.NewDispatcher(
		net,
		scheduler.NewWorkInQueueScheduler(2, nil),
		nil, nil, nil, func() time.Time { return now }, nextTestDispatchID(),
	)
	var forwarded []work.WorkDispatch
	var records []interfaces.FactoryDispatchRecord
	engine := newTestFactoryEngine(net, marking, []subsystems.Subsystem{dispatcher},
		WithDispatchHandler(func(dispatch work.WorkDispatch) { forwarded = append(forwarded, dispatch) }),
		WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) { records = append(records, record) }),
	)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if len(records) != 2 || len(forwarded) != 2 {
		t.Fatalf("same-tick dispatch records/forwards = %d/%d, want 2/2", len(records), len(forwarded))
	}

	recordedByTransition := make(map[string]interfaces.FactoryDispatchRecord, len(records))
	for _, record := range records {
		recordedByTransition[record.Dispatch.TransitionID] = record
		if record.CreatedTick != 1 {
			t.Fatalf("record %q created tick = %d, want 1", record.Dispatch.TransitionID, record.CreatedTick)
		}
	}
	assertRecordedMutation(t, recordedByTransition, "consume-work", "work-token")
	assertRecordedMutation(t, recordedByTransition, "observe-work", "slot-token")

	byTransition := make(map[string]work.WorkDispatch, len(forwarded))
	for _, dispatch := range forwarded {
		byTransition[dispatch.TransitionID] = dispatch
		if dispatch.Execution.DispatchCreatedTick != 1 || dispatch.Execution.CurrentTick != 1 {
			t.Fatalf("dispatch %q tick metadata = %+v, want tick 1", dispatch.TransitionID, dispatch.Execution)
		}
		if !dispatchHasInputToken(dispatch, "work-token") {
			t.Fatalf("dispatch %q inputs = %#v, want shared Work token", dispatch.TransitionID, dispatch.InputTokens)
		}
	}
	if _, ok := byTransition["consume-work"]; !ok {
		t.Fatalf("forwarded transitions = %#v, want consume-work", byTransition)
	}
	if observe, ok := byTransition["observe-work"]; !ok || !dispatchHasInputToken(observe, "slot-token") {
		t.Fatalf("observe-work dispatch = %#v, want shared Work and slot inputs", observe)
	}

	running := engine.RunningDispatches()
	if len(running) != 2 {
		t.Fatalf("running dispatches = %d, want 2", len(running))
	}
	assertHeldMutationTokens(t, recordedByTransition, running, "consume-work", "work-token")
	assertHeldMutationTokens(t, recordedByTransition, running, "observe-work", "slot-token")
}

func TestEngine_SameTickCancellationResultRestoresResources(t *testing.T) {
	net := sameWorkObserveConsumeNet()
	marking := petri.NewMarking("test-wf")
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	marking.AddToken(testDispatchToken("work-token", "task:init", factorytoken.DataTypeWork, now))
	marking.AddToken(testDispatchToken("slot-token", "slot:available", factorytoken.DataTypeResource, now))

	dispatcher := subsystems.NewDispatcher(
		net,
		scheduler.NewWorkInQueueScheduler(2, nil),
		nil, nil, nil, func() time.Time { return now }, nextTestDispatchID(),
	)
	transitioner := subsystems.NewTransitioner(
		net,
		nil,
		func() time.Time { return now },
		token_transformer.New(net.Places, net.WorkTypes, petri.NewWorkIDGenerator()),
		nil, nil, nil,
		factorydefinitions.WorkPropagationPolicyFunc(func(*factorydefinitions.FactoryWorkstationConfig) factorydefinitions.WorkPropagationMode {
			return factorydefinitions.WorkPropagationModeOutputAsPayload
		}),
	)
	hook := newTestDispatchResultHook()
	var forwarded []work.WorkDispatch
	engine := newTestFactoryEngine(net, marking, []subsystems.Subsystem{dispatcher, transitioner},
		WithDispatchResultHook(hook),
		WithDispatchHandler(func(dispatch work.WorkDispatch) { forwarded = append(forwarded, dispatch) }),
	)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("initial Tick() error: %v", err)
	}
	if len(forwarded) != 2 {
		t.Fatalf("same-tick dispatches = %d, want winner and loser", len(forwarded))
	}

	deliverSameWorkTerminalResults(t, hook, forwarded)
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("result Tick() error: %v", err)
	}
	assertSameWorkTerminalMarking(t, engine.GetMarking())
	assertSameWorkDispatchHistory(t, engine.runtimeState.DispatchHistory)
}

func deliverSameWorkTerminalResults(
	t *testing.T,
	hook *testDispatchResultHook,
	forwarded []work.WorkDispatch,
) {
	t.Helper()
	for _, dispatch := range forwarded {
		result := workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
		}
		switch dispatch.TransitionID {
		case "consume-work":
			result.Outcome = workerexecution.OutcomeAccepted
		case "observe-work":
			result.Outcome = workerexecution.OutcomeCanceled
			result.Cancellation = &workerexecution.DispatchCancellation{Reason: workerexecution.DispatchCancellationReasonSuperseded}
		default:
			t.Fatalf("unexpected transition %q", dispatch.TransitionID)
		}
		hook.results = append(hook.results, result)
	}
}

func assertSameWorkTerminalMarking(t *testing.T, marking petri.MarkingSnapshot) {
	t.Helper()
	if got := len(marking.TokensInPlace("task:failed")); got != 0 {
		t.Fatalf("failed Work tokens = %d, want none after superseded loser cleanup", got)
	}
	if got := len(marking.TokensInPlace("task:complete")); got != 1 {
		t.Fatalf("completed Work tokens = %d, want winner completion", got)
	}
	if got := len(marking.TokensInPlace("slot:available")); got != 1 {
		t.Fatalf("restored resource tokens = %d, want canceled loser resource claim restored", got)
	}
}

func assertSameWorkDispatchHistory(t *testing.T, history []interfaces.CompletedDispatch) {
	t.Helper()
	if len(history) != 2 {
		t.Fatalf("dispatch history = %#v, want winner and superseded loser", history)
	}
	var sawWinner, sawLoser bool
	for _, completed := range history {
		switch completed.TransitionID {
		case "consume-work":
			sawWinner = completed.Outcome == workerexecution.OutcomeAccepted
		case "observe-work":
			sawLoser = completed.Outcome == workerexecution.OutcomeCanceled &&
				completed.Cancellation != nil &&
				completed.Cancellation.Reason == workerexecution.DispatchCancellationReasonSuperseded
		}
	}
	if !sawWinner || !sawLoser {
		t.Fatalf("dispatch history = %#v, want accepted winner and SUPERSEDED loser", history)
	}
}

func sameWorkObserveConsumeNet() *state.Net {
	net := buildTestNet()
	resource := &state.ResourceDef{ID: "slot", Name: "Slot", Capacity: 1}
	resourcePlace, _ := state.GenerateResourcePlaces(resource, time.Time{})
	net.Resources[resource.ID] = resource
	net.Places[resourcePlace.ID] = resourcePlace
	net.Transitions["consume-work"] = &petri.Transition{
		ID: "consume-work", Name: "Consume Work", WorkerType: "worker-a",
		InputArcs:  []petri.Arc{{ID: "consume-work-input", Name: "work", PlaceID: "task:init", Direction: petri.ArcInput, Mode: interfaces.ArcModeConsume, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}},
		OutputArcs: []petri.Arc{{ID: "consume-work-output", Name: "complete", PlaceID: "task:complete", Direction: petri.ArcOutput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}},
	}
	net.Transitions["observe-work"] = &petri.Transition{
		ID: "observe-work", Name: "Observe Work", WorkerType: "worker-b",
		InputArcs: []petri.Arc{
			{ID: "observe-work-input", Name: "work", PlaceID: "task:init", Direction: petri.ArcInput, Mode: interfaces.ArcModeObserve, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
			{ID: "consume-slot-input", Name: "slot", PlaceID: resourcePlace.ID, Direction: petri.ArcInput, Mode: interfaces.ArcModeConsume, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
		},
	}
	return net
}

func testDispatchToken(id, placeID string, dataType factorytoken.DataType, now time.Time) *factorytoken.Token {
	return &factorytoken.Token{ID: id, PlaceID: placeID, CreatedAt: now, EnteredAt: now, Color: factorytoken.Color{WorkID: "work-1", TraceID: "trace-1", WorkTypeID: "task", DataType: dataType}}
}

func nextTestDispatchID() func() string {
	ids := []string{"dispatch-same-work-1", "dispatch-same-work-2"}
	next := 0
	return func() string {
		id := ids[next]
		next++
		return id
	}
}

func dispatchHasInputToken(dispatch work.WorkDispatch, tokenID string) bool {
	for _, token := range workers.WorkDispatchInputTokens(dispatch) {
		if token.ID == tokenID {
			return true
		}
	}
	return false
}

func assertHeldMutationTokens(t *testing.T, records map[string]interfaces.FactoryDispatchRecord, running map[string][]interfaces.MarkingMutation, transitionID string, tokenID string) {
	t.Helper()
	record, ok := records[transitionID]
	if !ok {
		t.Fatalf("dispatch record %q missing from %#v", transitionID, records)
	}
	mutations, ok := running[record.DispatchID]
	if !ok || len(mutations) != 1 || mutations[0].TokenID != tokenID || mutations[0].Type != interfaces.MutationConsume {
		t.Fatalf("running dispatch %q = %#v, want one consume for %s", record.DispatchID, mutations, tokenID)
	}
}

func assertRecordedMutation(t *testing.T, records map[string]interfaces.FactoryDispatchRecord, transitionID string, tokenID string) {
	t.Helper()
	record, ok := records[transitionID]
	if !ok || len(record.HeldMutations) != 1 || record.HeldMutations[0].TokenID != tokenID {
		t.Fatalf("record %q = %#v, want one held mutation for %s", transitionID, record, tokenID)
	}
}

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

	snapshot := engine.GetRuntimeStateSnapshot()
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
	if snapshot.DispatchHistory[0].Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("dispatch completion outcome = %q, want FAILED", snapshot.DispatchHistory[0].Outcome)
	}
	ignored := snapshot.DispatchHistory[0].IgnoredResult
	if ignored == nil || ignored.Reason != interfaces.DispatchResultIgnoredReasonWorkAlreadyTerminal ||
		ignored.ResultOutcome != workerexecution.OutcomeFailed ||
		ignored.ObservedState.Name != "complete" || ignored.ObservedState.Type != interfaces.StateTypeTerminal ||
		snapshot.DispatchHistory[0].IgnoredWorkID != workID {
		t.Fatalf("ignored result marker = %#v, work id = %q, want terminal %s marker", ignored, snapshot.DispatchHistory[0].IgnoredWorkID, workID)
	}
}

func TestHumanApprovalDispatchIsReservedWithoutWorkerForwarding(t *testing.T) {
	n := buildTestNet()
	n.Transitions["approval"] = &petri.Transition{
		ID:   "approval",
		Name: "Approval",
		Type: petri.TransitionHumanApproval,
	}
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
						DispatchID:      "approval-dispatch",
						TransitionID:    "approval",
						WorkerType:      "",
						WorkstationName: "Approval",
						Execution: work.ExecutionMetadata{
							WorkIDs: []string{"work-1"},
						},
					},
					Mutations: []interfaces.MarkingMutation{{
						Type:      interfaces.MutationConsume,
						TokenID:   "tok-1",
						FromPlace: "task:init",
						Reason:    "consumed by human approval",
					}},
				}},
			}, nil
		},
	}

	hook := newTestDispatchResultHook()
	var handlerCalls int
	var records []interfaces.FactoryDispatchRecord
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchResultHook(hook),
		WithDispatchHandler(func(work.WorkDispatch) { handlerCalls++ }),
		WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
			records = append(records, record)
		}),
	)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if handlerCalls != 0 {
		t.Fatalf("human approval dispatch reached worker handler %d times", handlerCalls)
	}
	if len(hook.submits) != 0 {
		t.Fatalf("human approval dispatch reached result hook: %#v", hook.submits)
	}
	if len(records) != 1 || !records[0].HumanApproval {
		t.Fatalf("dispatch records = %#v, want one human-approval record", records)
	}
	if len(engine.RunningDispatches()) != 1 {
		t.Fatalf("running dispatches = %d, want reserved approval dispatch", len(engine.RunningDispatches()))
	}

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("human approval dispatch was retried or duplicated: %#v", records)
	}
}

func TestWorkResultForCompletedDispatchPreservesResolvedClassificationLabel(t *testing.T) {
	result := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		Output:  "needs_review",
	}
	completed := interfaces.CompletedDispatch{
		Outcome:                     workerexecution.OutcomeAccepted,
		SelectedClassificationLabel: "needs_review",
	}

	got := workResultForCompletedDispatch(result, completed)

	if got.SelectedClassificationLabel != completed.SelectedClassificationLabel {
		t.Fatalf("selected classification label = %q, want %q", got.SelectedClassificationLabel, completed.SelectedClassificationLabel)
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
						InputTokens: workers.InputTokens(factorytoken.ToWorker(factorytoken.Token{
							ID:      "token-1",
							PlaceID: "task:init",
						})),
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
						InputTokens:     workers.InputTokens(factorytoken.ToWorker(inputToken)),
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
						InputTokens: workers.InputTokens(factorytoken.ToWorker(factorytoken.Token{
							ID:      "tok-1",
							PlaceID: "task:init",
							Color: factorytoken.Color{
								Name:       "my-task-name",
								WorkID:     "work-1",
								WorkTypeID: "task",
							},
						})),
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

func assertConsumedTokenName(t *testing.T, tokens []workerexecution.Token, wantName, label string) {
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
