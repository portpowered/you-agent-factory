package subsystems

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestNoOpDispatcher_NoEnabledTransitions(t *testing.T) {
	n := &state.Net{
		Places:      map[string]*petri.Place{"p1": {ID: "p1"}},
		Transitions: map[string]*petri.Transition{},
	}

	sched := &mockScheduler{}
	tp := newTestPipeline(n)
	noopDisp := NewNoOpDispatcher(n, sched, tp.results)

	markingSnap := makeDispatcherSnapshot(map[string]*interfaces.Token{})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}

	result, err := noopDisp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when no transitions enabled, got %+v", result)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-noop-dispatcher-fixture review=2026-07-18 removal=split-net-setup-and-multi-transition-assertions-before-next-noop-dispatcher-change
func TestNoOpDispatcher_MultipleTransitions(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init-a": {ID: "p-init-a"},
			"p-init-b": {ID: "p-init-b"},
			"p-done-a": {ID: "p-done-a"},
			"p-done-b": {ID: "p-done-b"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "work-a",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "in", PlaceID: "p-init-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a2", Name: "out", PlaceID: "p-done-a", Direction: petri.ArcOutput},
				},
			},
			"t2": {
				ID:         "t2",
				Name:       "work-b",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "a3", Name: "in", PlaceID: "p-init-b", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a4", Name: "out", PlaceID: "p-done-b", Direction: petri.ArcOutput},
				},
			},
		},
	}

	sched := &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "t1", ConsumeTokens: []string{"tok1"}, WorkerType: "script"},
			{TransitionID: "t2", ConsumeTokens: []string{"tok2"}, WorkerType: "script"},
		},
	}

	tp := newTestPipeline(n)
	noopDisp := NewNoOpDispatcher(n, sched, tp.results)

	markingSnap := makeDispatcherSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p-init-a", Color: interfaces.TokenColor{WorkID: "w1"}},
		"tok2": {ID: "tok2", PlaceID: "p-init-b", Color: interfaces.TokenColor{WorkID: "w2"}},
	})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}

	result, err := noopDisp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	snapshot.Dispatches = make(map[string]*interfaces.DispatchEntry, len(result.Dispatches))
	for _, rec := range result.Dispatches {
		snapshot.Dispatches[rec.Dispatch.DispatchID] = &interfaces.DispatchEntry{
			DispatchID:     rec.Dispatch.DispatchID,
			TransitionID:   rec.Dispatch.TransitionID,
			ConsumedTokens: workers.WorkDispatchInputTokens(rec.Dispatch),
		}
	}

	// Should have 2 CONSUME mutations.
	if len(result.Mutations) != 2 {
		t.Fatalf("expected 2 mutations, got %d", len(result.Mutations))
	}

	// Pipeline should produce 2 CREATE mutations.
	collResult, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("pipeline error: %v", err)
	}
	if collResult == nil {
		t.Fatal("expected pipeline output")
	}
	if len(collResult.Mutations) != 2 {
		t.Fatalf("expected 2 pipeline mutations, got %d", len(collResult.Mutations))
	}

	// Verify output places.
	places := map[string]bool{}
	for _, cm := range collResult.Mutations {
		if cm.Type != interfaces.MutationCreate {
			t.Errorf("expected CREATE, got %s", cm.Type)
		}
		places[cm.ToPlace] = true
	}
	if !places["p-done-a"] || !places["p-done-b"] {
		t.Errorf("expected tokens in p-done-a and p-done-b, got %v", places)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-noop-history-fixture review=2026-07-18 removal=split-history-setup-and-output-token-assertions-before-next-noop-dispatcher-change
func TestNoOpDispatcher_PreservesInputHistory(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init": {ID: "p-init"},
			"p-done": {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "do-work",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "in", PlaceID: "p-init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a2", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}

	sched := &mockScheduler{
		decisions: []interfaces.FiringDecision{
			{TransitionID: "t1", ConsumeTokens: []string{"tok1"}, WorkerType: "script"},
		},
	}

	tp := newTestPipeline(n)
	noopDisp := NewNoOpDispatcher(n, sched, tp.results)

	// Input token with existing history.
	markingSnap := makeDispatcherSnapshot(map[string]*interfaces.Token{
		"tok1": {
			ID:      "tok1",
			PlaceID: "p-init",
			Color:   interfaces.TokenColor{WorkID: "w1"},
			History: interfaces.TokenHistory{
				TotalVisits:         map[string]int{"t0": 1},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{"p-start": 1},
			},
		},
	})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}

	result, err := noopDisp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil dispatcher result")
	}
	snapshot.Dispatches = make(map[string]*interfaces.DispatchEntry, len(result.Dispatches))
	for _, rec := range result.Dispatches {
		snapshot.Dispatches[rec.Dispatch.DispatchID] = &interfaces.DispatchEntry{
			DispatchID:     rec.Dispatch.DispatchID,
			TransitionID:   rec.Dispatch.TransitionID,
			ConsumedTokens: workers.WorkDispatchInputTokens(rec.Dispatch),
		}
	}

	// Pipeline processes the result — check that history is carried forward.
	collResult, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("pipeline error: %v", err)
	}
	if collResult == nil || len(collResult.Mutations) == 0 {
		t.Fatal("expected pipeline output")
	}

	outToken := collResult.Mutations[0].NewToken
	if outToken == nil {
		t.Fatal("expected new token")
	}

	// t0 visit should be carried forward.
	if outToken.History.TotalVisits["t0"] != 1 {
		t.Errorf("expected TotalVisits[t0]=1, got %d", outToken.History.TotalVisits["t0"])
	}
	// t1 should be incremented.
	if outToken.History.TotalVisits["t1"] != 1 {
		t.Errorf("expected TotalVisits[t1]=1, got %d", outToken.History.TotalVisits["t1"])
	}

	// Outcome is ACCEPTED, so ConsecutiveFailures for t1 should be 0.
	if outToken.History.ConsecutiveFailures["t1"] != 0 {
		t.Errorf("expected ConsecutiveFailures[t1]=0, got %d", outToken.History.ConsecutiveFailures["t1"])
	}
}

// TestNoOpDispatcher_ImplementsSubsystem verifies compile-time interface compliance
// (redundant with var _ above, but explicit for documentation).
func TestNoOpDispatcher_ImplementsSubsystem(t *testing.T) {
	var _ Subsystem = (*NoOpDispatcherSubsystem)(nil)
	// Also verify the outcome type used.
	if interfaces.OutcomeAccepted != "ACCEPTED" {
		t.Errorf("unexpected OutcomeAccepted value: %s", interfaces.OutcomeAccepted)
	}
}
