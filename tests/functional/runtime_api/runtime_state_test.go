package runtime_api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/buffers"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestRuntimeState_FailureRouting(t *testing.T) {
	n := buildThreeStageNet()
	h := newThreeStageHarness(t, n)
	h.SetExecutor("agent", &failingExecutor{errorMsg: "stage1 processing error"})

	h.submitWork("task", "fail-item")

	err := h.eng.TickUntil(context.Background(), func(snap *petri.MarkingSnapshot) bool {
		for _, tok := range snap.Tokens {
			if tok.PlaceID == "task:failed" {
				return true
			}
		}
		return false
	}, 20)
	if err != nil {
		snap := h.eng.GetMarking()
		t.Fatalf("failure routing did not complete: %v (tokens: %v)", err, tokenPlaces(snap))
	}

	snap := h.eng.GetMarking()
	failedCount := 0
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:failed" {
			failedCount++
			if tok.History.LastError == "" {
				t.Error("token in failed place has empty LastError")
			}
			if tok.History.ConsecutiveFailures == nil || tok.History.ConsecutiveFailures["t-stage1"] == 0 {
				t.Error("token in failed place has zero ConsecutiveFailures for t-stage1")
			}
		}
	}
	if failedCount != 1 {
		t.Errorf("expected 1 token in failed place, got %d", failedCount)
	}

	rtSnap := h.eng.GetRuntimeStateSnapshot()
	for _, tok := range rtSnap.Marking.Tokens {
		if tok.PlaceID == "task:failed" && tok.History.LastError == "" {
			t.Error("RuntimeStateSnapshot: token in failed place has empty LastError")
		}
	}
}

func TestRuntimeState_SameTypeTransitionsPreserveCreatedAt(t *testing.T) {
	n := buildThreeStageNet()
	t0 := time.Date(2026, time.April, 6, 8, 0, 0, 0, time.UTC)
	tickTimes := []time.Time{t0.Add(1 * time.Minute), t0.Add(2 * time.Minute), t0.Add(3 * time.Minute)}
	clockIndex := 0
	initialMarking := petri.NewMarking(n.ID)
	initialMarking.AddToken(&interfaces.Token{
		ID: "tok-initial", PlaceID: "task:init", CreatedAt: t0, EnteredAt: t0,
		Color: interfaces.TokenColor{WorkID: "task-1", WorkTypeID: "task", DataType: interfaces.DataTypeWork},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})

	sched := scheduler.NewFIFOScheduler()
	resultBuffer := buffers.NewTypedBuffer[interfaces.WorkResult](16)
	historySubsystem := subsystems.NewHistory(nil)
	transitionerSubsystem := subsystems.NewTransitioner(n, nil, subsystems.WithTransitionerClock(func() time.Time {
		current := tickTimes[clockIndex]
		if clockIndex < len(tickTimes)-1 {
			clockIndex++
		}
		return current
	}))
	termination := subsystems.NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)
	dispatcher := subsystems.NewNoOpDispatcher(n, sched, resultBuffer)
	eng := engine.NewFactoryEngine(
		n,
		initialMarking,
		[]subsystems.Subsystem{dispatcher, historySubsystem, transitionerSubsystem, termination},
		engine.WithDispatchHandler(func(dispatch interfaces.WorkDispatch) { _ = dispatch }),
		engine.WithResultBuffer(resultBuffer),
	)

	testCases := []struct {
		placeID         string
		expectedEntered time.Time
	}{
		{placeID: "task:stage1", expectedEntered: tickTimes[0]},
		{placeID: "task:stage2", expectedEntered: tickTimes[1]},
		{placeID: "task:done", expectedEntered: tickTimes[2]},
	}
	var previousEnteredAt time.Time
	for _, tc := range testCases {
		if err := eng.TickUntil(context.Background(), func(snap *petri.MarkingSnapshot) bool {
			for _, tok := range snap.Tokens {
				if tok.PlaceID == tc.placeID {
					return true
				}
			}
			return false
		}, 10); err != nil {
			t.Fatalf("expected token to reach %s: %v", tc.placeID, err)
		}

		snap := eng.GetMarking()
		tokens := snap.TokensInPlace(tc.placeID)
		if len(tokens) != 1 {
			t.Fatalf("expected 1 token in %s, got %d", tc.placeID, len(tokens))
		}

		token := tokens[0]
		if !token.CreatedAt.Equal(t0) {
			t.Fatalf("token in %s CreatedAt = %v, want %v", tc.placeID, token.CreatedAt, t0)
		}
		if !token.EnteredAt.Equal(tc.expectedEntered) {
			t.Fatalf("token in %s EnteredAt = %v, want %v", tc.placeID, token.EnteredAt, tc.expectedEntered)
		}
		if !previousEnteredAt.IsZero() && !token.EnteredAt.After(previousEnteredAt) {
			t.Fatalf("token in %s EnteredAt = %v, want after %v", tc.placeID, token.EnteredAt, previousEnteredAt)
		}
		previousEnteredAt = token.EnteredAt
	}
}

func buildThreeStageNet() *state.Net {
	return &state.Net{
		ID: "three-stage-test",
		Places: map[string]*petri.Place{
			"task:init":   {ID: "task:init", TypeID: "task", State: "init"},
			"task:stage1": {ID: "task:stage1", TypeID: "task", State: "stage1"},
			"task:stage2": {ID: "task:stage2", TypeID: "task", State: "stage2"},
			"task:done":   {ID: "task:done", TypeID: "task", State: "done"},
			"task:failed": {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		Transitions: map[string]*petri.Transition{
			"t-stage1": {ID: "t-stage1", Name: "stage1", WorkerType: "agent", InputArcs: []petri.Arc{{ID: "a1", PlaceID: "task:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}}, OutputArcs: []petri.Arc{{ID: "a2", PlaceID: "task:stage1", Direction: petri.ArcOutput}}, FailureArcs: []petri.Arc{{ID: "a3", PlaceID: "task:failed", Direction: petri.ArcOutput}}},
			"t-stage2": {ID: "t-stage2", Name: "stage2", WorkerType: "agent", InputArcs: []petri.Arc{{ID: "a4", PlaceID: "task:stage1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}}, OutputArcs: []petri.Arc{{ID: "a5", PlaceID: "task:stage2", Direction: petri.ArcOutput}}, FailureArcs: []petri.Arc{{ID: "a6", PlaceID: "task:failed", Direction: petri.ArcOutput}}},
			"t-stage3": {ID: "t-stage3", Name: "stage3", WorkerType: "agent", InputArcs: []petri.Arc{{ID: "a7", PlaceID: "task:stage2", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}}, OutputArcs: []petri.Arc{{ID: "a8", PlaceID: "task:done", Direction: petri.ArcOutput}}, FailureArcs: []petri.Arc{{ID: "a9", PlaceID: "task:failed", Direction: petri.ArcOutput}}},
		},
		WorkTypes: map[string]*state.WorkType{"task": {ID: "task", States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "stage1", Category: state.StateCategoryProcessing},
			{Value: "stage2", Category: state.StateCategoryProcessing},
			{Value: "done", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		}}},
	}
}

type threeStageHarness struct {
	t         *testing.T
	eng       *engine.FactoryEngine
	net       *state.Net
	executors map[string]workers.WorkerExecutor
}

func newThreeStageHarness(t *testing.T, n *state.Net) *threeStageHarness {
	return newThreeStageHarnessWithMarking(t, n, petri.NewMarking(n.ID))
}

func newThreeStageHarnessWithMarking(t *testing.T, n *state.Net, marking *petri.Marking) *threeStageHarness {
	t.Helper()
	h := &threeStageHarness{t: t, net: n, executors: make(map[string]workers.WorkerExecutor)}

	historySubsystem := subsystems.NewHistory(nil)
	transitionerSubsystem := subsystems.NewTransitioner(n, nil)
	sched := scheduler.NewFIFOScheduler()

	syncDisp := &threeStgSyncDispatcher{net: n, sched: sched, harness: h}
	circuitBreaker := subsystems.NewCircuitBreaker(n, nil)
	termSub := subsystems.NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)

	h.eng = engine.NewFactoryEngine(n, marking, []subsystems.Subsystem{
		circuitBreaker,
		syncDisp,
		historySubsystem,
		transitionerSubsystem,
		termSub,
	})
	return h
}

func (h *threeStageHarness) SetExecutor(workerType string, exec workers.WorkerExecutor) {
	h.executors[workerType] = exec
}

func (h *threeStageHarness) submitWork(workTypeID, workID string) {
	h.t.Helper()
	request := workRequestFromSubmitRequests([]interfaces.SubmitRequest{{WorkTypeID: workTypeID, WorkID: workID}})
	if _, err := h.eng.SubmitWorkRequest(context.Background(), request); err != nil {
		h.t.Fatalf("failed to submit work: %v", err)
	}
}

type failingExecutor struct{ errorMsg string }

func (e *failingExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: interfaces.OutcomeFailed, Error: e.errorMsg}, nil
}

type threeStgSyncDispatcher struct {
	net     *state.Net
	sched   scheduler.Scheduler
	harness *threeStageHarness
}

var _ subsystems.Subsystem = (*threeStgSyncDispatcher)(nil)

func (sd *threeStgSyncDispatcher) TickGroup() subsystems.TickGroup { return subsystems.Dispatcher }

func (sd *threeStgSyncDispatcher) Execute(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	enabled := scheduler.FindEnabledTransitions(sd.net, &snapshot.Marking)
	if len(enabled) == 0 {
		return nil, nil
	}
	decisions := sd.sched.Select(enabled, snapshot)
	if len(decisions) == 0 {
		return nil, nil
	}

	var mutations []interfaces.MarkingMutation
	for _, decision := range decisions {
		for _, tokenID := range decision.ConsumeTokens {
			tok, ok := snapshot.Marking.Tokens[tokenID]
			if !ok {
				continue
			}
			mutations = append(mutations, interfaces.MarkingMutation{Type: interfaces.MutationConsume, TokenID: tokenID, FromPlace: tok.PlaceID, Reason: "consumed by transition " + decision.TransitionID})
		}

		inputTokens := make([]interfaces.Token, 0, len(decision.ConsumeTokens))
		for _, id := range decision.ConsumeTokens {
			if tok, ok := snapshot.Marking.Tokens[id]; ok {
				inputTokens = append(inputTokens, *tok)
			}
		}

		tr := sd.net.Transitions[decision.TransitionID]
		dispatch := interfaces.WorkDispatch{
			DispatchID:      fmt.Sprintf("%s-dispatch", decision.TransitionID),
			TransitionID:    decision.TransitionID,
			WorkerType:      tr.WorkerType,
			WorkstationName: tr.Name,
			InputTokens:     workers.InputTokens(inputTokens...),
		}

		exec, ok := sd.harness.executors[tr.WorkerType]
		if !ok {
			continue
		}
		result, err := exec.Execute(ctx, dispatch)
		if err != nil {
			continue
		}
		sd.harness.eng.GetResultBuffer().Write(ctx, result)
	}

	if len(mutations) == 0 {
		return nil, nil
	}
	return &interfaces.TickResult{Mutations: mutations}, nil
}
