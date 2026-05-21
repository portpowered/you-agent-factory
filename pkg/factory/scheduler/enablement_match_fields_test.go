package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestEnablementEvaluator_MatchesFieldsGuardEnablesSingleInputWhenSelectorResolves(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{"task:ready": {ID: "task:ready"}},
		Transitions: map[string]*petri.Transition{
			"match-single": {
				ID:   "match-single",
				Name: "match-single",
				InputArcs: []petri.Arc{{
					ID:          "task-in",
					Name:        "task",
					PlaceID:     "task:ready",
					Direction:   petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					Guard:       &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`},
				}},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"task-alpha": {ID: "task-alpha", PlaceID: "task:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "alpha"}}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["task"]); strings.Join(got, ",") != "task-alpha" {
		t.Fatalf("task binding tokens = %v, want [task-alpha]", got)
	}
}

func TestEnablementEvaluator_MatchesFieldsGuardEnablesOnMatchingTwoInputValues(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := matchesFieldsPairNet()
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "alpha"}}},
		"task-alpha": {ID: "task-alpha", PlaceID: "task:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "alpha"}}},
		"task-beta":  {ID: "task-beta", PlaceID: "task:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "beta"}}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["plan"]); strings.Join(got, ",") != "plan-alpha" {
		t.Fatalf("plan binding tokens = %v, want [plan-alpha]", got)
	}
	if got := tokenIDs(enabled[0].Bindings["task"]); strings.Join(got, ",") != "task-alpha" {
		t.Fatalf("task binding tokens = %v, want [task-alpha]", got)
	}
}

func TestEnablementEvaluator_MatchesFieldsGuardBlocksMismatchedTwoInputValues(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := matchesFieldsPairNet()
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "alpha"}}},
		"task-beta":  {ID: "task-beta", PlaceID: "task:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "beta"}}},
	})

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled transitions = %d, want 0", len(enabled))
	}
}

func TestEnablementEvaluator_MatchesFieldsGuardRequiresAllInputsToMatchSourceValue(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := matchesFieldsTripletNet()

	matching := matchesFieldsTripletSnapshot("alpha", "alpha", map[string]string{"asset-alpha": "alpha", "asset-beta": "beta"})
	enabled := eval.FindEnabledTransitions(context.Background(), n, &matching)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["asset"]); strings.Join(got, ",") != "asset-alpha" {
		t.Fatalf("asset binding tokens = %v, want [asset-alpha]", got)
	}

	mismatched := matchesFieldsTripletSnapshot("alpha", "alpha", map[string]string{"asset-beta": "beta"})
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &mismatched); len(enabled) != 0 {
		t.Fatalf("enabled transitions with mismatched third input = %d, want 0", len(enabled))
	}
}

func matchesFieldsPairNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"plan:ready": {ID: "plan:ready"},
			"task:ready": {ID: "task:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-pair": {
				ID:   "match-pair",
				Name: "match-pair",
				InputArcs: []petri.Arc{
					{ID: "plan-in", Name: "plan", PlaceID: "plan:ready", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`}},
					{ID: "task-in", Name: "task", PlaceID: "task:ready", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`, MatchBinding: "plan"}},
				},
			},
		},
	}
}

func matchesFieldsTripletNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"plan:ready":  {ID: "plan:ready"},
			"task:ready":  {ID: "task:ready"},
			"asset:ready": {ID: "asset:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-triplet": {
				ID:   "match-triplet",
				Name: "match-triplet",
				InputArcs: []petri.Arc{
					{ID: "plan-in", Name: "plan", PlaceID: "plan:ready", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`}},
					{ID: "task-in", Name: "task", PlaceID: "task:ready", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`, MatchBinding: "plan"}},
					{ID: "asset-in", Name: "asset", PlaceID: "asset:ready", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`, MatchBinding: "plan"}},
				},
			},
		},
	}
}

func matchesFieldsTripletSnapshot(planValue, taskValue string, assetValues map[string]string) petri.MarkingSnapshot {
	tokens := map[string]*interfaces.Token{
		"plan-" + planValue: {ID: "plan-" + planValue, PlaceID: "plan:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": planValue}}},
		"task-" + taskValue: {ID: "task-" + taskValue, PlaceID: "task:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": taskValue}}},
	}
	for assetID, assetValue := range assetValues {
		tokens[assetID] = &interfaces.Token{ID: assetID, PlaceID: "asset:ready", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": assetValue}}}
	}
	return makeTestSnapshot(tokens)
}
