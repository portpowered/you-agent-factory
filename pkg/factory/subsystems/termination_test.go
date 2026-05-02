package subsystems

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestTerminationCheck_TerminatesWhenNoWorkIsInTheSystem(t *testing.T) {
	n := buildTerminationNet()
	tc := NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*interfaces.Token{
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: interfaces.TokenColor{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("should terminate when no work is in the system")
	}
}

func TestTerminationCheck_DoesNotTerminateWithNonTerminalWork(t *testing.T) {
	n := buildTerminationNet()
	tc := NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*interfaces.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:init", Color: interfaces.TokenColor{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: interfaces.TokenColor{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("should not terminate when work remains in a non-terminal state")
	}
}

func TestTerminationCheck_TerminatesWhenAllWorkIsTerminal(t *testing.T) {
	n := buildTerminationNet()
	tc := NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*interfaces.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:done", Color: interfaces.TokenColor{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: interfaces.TokenColor{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("should terminate when all work is terminal")
	}
}

func TestTerminationCheck_TerminatesWhenAllWorkHasFailed(t *testing.T) {
	n := buildTerminationNet()
	tc := NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*interfaces.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:failed", Color: interfaces.TokenColor{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: interfaces.TokenColor{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("should terminate when all work has failed")
	}
}

func TestTerminationCheck_DoesNotTerminateWhileDispatchesAreInFlight(t *testing.T) {
	n := buildTerminationNet()
	tc := NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		InFlightCount: 1,
		Marking: makeTerminationSnapshot(map[string]*interfaces.Token{
			"tok1":     {ID: "tok1", PlaceID: "wt:done", Color: interfaces.TokenColor{WorkID: "w1"}},
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: interfaces.TokenColor{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("should not terminate while work is still in flight")
	}
}

func TestTerminationCheck_DoesNotTerminateUntilResourcesReturn(t *testing.T) {
	n := buildTerminationNet()
	tc := NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*interfaces.Token{
			"tok1": {ID: "tok1", PlaceID: "wt:done", Color: interfaces.TokenColor{WorkID: "w1"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("should not terminate until resource tokens are returned")
	}
}

func TestTerminationCheck_ResourcesOnlyTerminates(t *testing.T) {
	n := buildTerminationNetNoTransitions()
	tc := NewTerminationCheck(n, nil, interfaces.RuntimeModeBatch)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*interfaces.Token{
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: interfaces.TokenColor{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.ShouldTerminate {
		t.Fatal("should terminate when only returned resources remain")
	}
}

func TestTerminationCheck_ServiceModeDoesNotTerminateIdleRuntime(t *testing.T) {
	n := buildTerminationNetNoTransitions()
	tc := NewTerminationCheck(n, nil, interfaces.RuntimeModeService)

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeTerminationSnapshot(map[string]*interfaces.Token{
			"res-tok0": {ID: "res-tok0", PlaceID: "gpu:available", Color: interfaces.TokenColor{WorkID: "gpu:0", WorkTypeID: "gpu"}},
		}),
	}

	result, err := tc.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && result.ShouldTerminate {
		t.Fatal("service mode should stay alive while idle")
	}
}

// buildTerminationNet creates a test net with one work type and one resource.
func buildTerminationNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"wt:init":       {ID: "wt:init", TypeID: "wt", State: "init"},
			"wt:done":       {ID: "wt:done", TypeID: "wt", State: "done"},
			"wt:failed":     {ID: "wt:failed", TypeID: "wt", State: "failed"},
			"gpu:available": {ID: "gpu:available", TypeID: "gpu", State: "available"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID: "t1",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "wt:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "a2", Name: "out", PlaceID: "wt:done", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"wt": {
				ID: "wt",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "done", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
		Resources: map[string]*state.ResourceDef{
			"gpu": {ID: "gpu", Name: "GPU", Capacity: 1},
		},
	}
}

// buildTerminationNetNoTransitions creates a test net with no transitions.
func buildTerminationNetNoTransitions() *state.Net {
	net := buildTerminationNet()
	net.Transitions = map[string]*petri.Transition{}
	return net
}

func makeTerminationSnapshot(tokens map[string]*interfaces.Token) petri.MarkingSnapshot {
	placeTokens := make(map[string][]string)
	for id, tok := range tokens {
		if tok.CreatedAt.IsZero() {
			tok.CreatedAt = time.Now()
		}
		if tok.EnteredAt.IsZero() {
			tok.EnteredAt = time.Now()
		}
		placeTokens[tok.PlaceID] = append(placeTokens[tok.PlaceID], id)
	}
	return petri.MarkingSnapshot{
		Tokens:      tokens,
		PlaceTokens: placeTokens,
	}
}
