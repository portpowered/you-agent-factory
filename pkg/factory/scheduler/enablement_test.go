package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestEnablementEvaluator_LogsEnabledTransition(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p1": {ID: "p1"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:         "t1",
				Name:       "do-work",
				WorkerType: "agent",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p1", Color: interfaces.TokenColor{WorkID: "w1"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled transition, got %d", len(enabled))
	}

	if entry := logger.findEntry("transition enabled"); entry == nil {
		t.Fatal("expected 'transition enabled' log entry")
	}
	if summary := logger.findEntry("evaluation complete"); summary == nil {
		t.Fatal("expected 'evaluation complete' log entry")
	}
}

func TestEnablementEvaluator_LogsDisabledInsufficientTokens(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p1": {ID: "p1"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:   "t1",
				Name: "do-work",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{})
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}
	if entry := logger.findEntry("transition disabled"); entry == nil {
		t.Fatal("expected 'transition disabled' log entry")
	}
}

func TestEnablementEvaluator_LogsDisabledGuardFailed(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-work":   {ID: "p-work"},
			"p-review": {ID: "p-review"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:   "t1",
				Name: "merge",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p-work", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID: "a2", Name: "review", PlaceID: "p-review", Direction: petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard: &petri.MatchColorGuard{
							Field:        "parent_id",
							MatchBinding: "work",
							MatchField:   "work_id",
						},
					},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-work":   {ID: "tok-work", PlaceID: "p-work", Color: interfaces.TokenColor{WorkID: "w1"}},
		"tok-review": {ID: "tok-review", PlaceID: "p-review", Color: interfaces.TokenColor{WorkID: "r1", ParentID: "WRONG"}},
	})

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}
	if entry := logger.findEntry("guard failed"); entry == nil {
		t.Fatal("expected log entry containing 'guard failed'")
	}
}

func TestEnablementEvaluator_BindsMultipleNamedGuardedInputs(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-req":    {ID: "p-req"},
			"p-design": {ID: "p-design"},
			"p-code":   {ID: "p-code"},
		},
		Transitions: map[string]*petri.Transition{
			"assemble": {
				ID:   "assemble",
				Name: "assemble",
				InputArcs: []petri.Arc{
					{ID: "request-in", Name: "request", PlaceID: "p-req", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID:          "design-in",
						Name:        "design",
						PlaceID:     "p-design",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.MatchColorGuard{Field: "parent_id", MatchBinding: "request", MatchField: "work_id"},
					},
					{
						ID:          "code-in",
						Name:        "code",
						PlaceID:     "p-code",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.MatchColorGuard{Field: "parent_id", MatchBinding: "request", MatchField: "work_id"},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-req":          {ID: "tok-req", PlaceID: "p-req", Color: interfaces.TokenColor{WorkID: "w1"}},
		"tok-design-match": {ID: "tok-design-match", PlaceID: "p-design", Color: interfaces.TokenColor{WorkID: "d1", ParentID: "w1"}},
		"tok-design-other": {ID: "tok-design-other", PlaceID: "p-design", Color: interfaces.TokenColor{WorkID: "d2", ParentID: "other"}},
		"tok-code-match":   {ID: "tok-code-match", PlaceID: "p-code", Color: interfaces.TokenColor{WorkID: "c1", ParentID: "w1"}},
		"tok-code-other":   {ID: "tok-code-other", PlaceID: "p-code", Color: interfaces.TokenColor{WorkID: "c2", ParentID: "other"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if enabled[0].TransitionID != "assemble" {
		t.Fatalf("enabled transition = %q, want assemble", enabled[0].TransitionID)
	}
	wantBindings := map[string]string{
		"request": "tok-req",
		"design":  "tok-design-match",
		"code":    "tok-code-match",
	}
	for binding, want := range wantBindings {
		got := tokenIDs(enabled[0].Bindings[binding])
		if strings.Join(got, ",") != want {
			t.Fatalf("%s binding tokens = %v, want [%s]", binding, got, want)
		}
	}
}

func TestEnablementEvaluator_BindsAllTokensForMatchingParentGuard(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-parent":   {ID: "p-parent"},
			"p-children": {ID: "p-children"},
		},
		Transitions: map[string]*petri.Transition{
			"join-children": {
				ID:   "join-children",
				Name: "join-children",
				InputArcs: []petri.Arc{
					{ID: "parent-in", Name: "parent", PlaceID: "p-parent", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID:          "children-in",
						Name:        "children",
						PlaceID:     "p-children",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityAll},
						Guard:       &petri.AllWithParentGuard{MatchBinding: "parent"},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-parent":      {ID: "tok-parent", PlaceID: "p-parent", Color: interfaces.TokenColor{WorkID: "w1"}},
		"tok-child-a":     {ID: "tok-child-a", PlaceID: "p-children", Color: interfaces.TokenColor{WorkID: "c1", ParentID: "w1"}},
		"tok-child-b":     {ID: "tok-child-b", PlaceID: "p-children", Color: interfaces.TokenColor{WorkID: "c2", ParentID: "w1"}},
		"tok-child-other": {ID: "tok-child-other", PlaceID: "p-children", Color: interfaces.TokenColor{WorkID: "c3", ParentID: "other"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["parent"]); strings.Join(got, ",") != "tok-parent" {
		t.Fatalf("parent binding tokens = %v, want [tok-parent]", got)
	}
	if got := tokenIDs(enabled[0].Bindings["children"]); strings.Join(got, ",") != "tok-child-a,tok-child-b" {
		t.Fatalf("children binding tokens = %v, want [tok-child-a tok-child-b]", got)
	}
}

func TestEnablementEvaluator_SameNameGuardEnablesOnMatchingNames(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := sameNameGuardNet()
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", Color: interfaces.TokenColor{Name: "alpha"}},
		"task-alpha": {ID: "task-alpha", PlaceID: "task:ready", Color: interfaces.TokenColor{Name: "alpha"}},
		"task-beta":  {ID: "task-beta", PlaceID: "task:ready", Color: interfaces.TokenColor{Name: "beta"}},
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

func TestEnablementEvaluator_SameNameGuardBlocksNonMatchingNames(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := sameNameGuardNet()
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", Color: interfaces.TokenColor{Name: "alpha"}},
		"task-beta":  {ID: "task-beta", PlaceID: "task:ready", Color: interfaces.TokenColor{Name: "beta"}},
	})

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled transitions = %d, want 0", len(enabled))
	}
}

func TestEnablementEvaluator_SameNameGuardFindsLaterMatchingBinding(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"idea:to-complete": {ID: "idea:to-complete"},
			"task:to-complete": {ID: "task:to-complete"},
		},
		Transitions: map[string]*petri.Transition{
			"consume": {
				ID:   "consume",
				Name: "consume",
				InputArcs: []petri.Arc{
					{ID: "task-in", Name: "task", PlaceID: "task:to-complete", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID:          "idea-in",
						Name:        "idea",
						PlaceID:     "idea:to-complete",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.SameNameGuard{MatchBinding: "task"},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"task-alpha": {ID: "task-alpha", PlaceID: "task:to-complete", Color: interfaces.TokenColor{Name: "alpha"}},
		"task-zeta":  {ID: "task-zeta", PlaceID: "task:to-complete", Color: interfaces.TokenColor{Name: "zeta"}},
		"idea-zeta":  {ID: "idea-zeta", PlaceID: "idea:to-complete", Color: interfaces.TokenColor{Name: "zeta"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["task"]); strings.Join(got, ",") != "task-zeta" {
		t.Fatalf("task binding tokens = %v, want [task-zeta]", got)
	}
	if got := tokenIDs(enabled[0].Bindings["idea"]); strings.Join(got, ",") != "idea-zeta" {
		t.Fatalf("idea binding tokens = %v, want [idea-zeta]", got)
	}
}

func TestEnablementEvaluator_VisitCountGuardEnablesAtThreshold(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-init":   {ID: "p-init"},
			"p-failed": {ID: "p-failed"},
		},
		Transitions: map[string]*petri.Transition{
			"exhaust-review": {
				ID:   "exhaust-review",
				Name: "exhaust-review",
				Type: petri.TransitionExhaustion,
				InputArcs: []petri.Arc{
					{
						ID:          "work-in",
						Name:        "work",
						PlaceID:     "p-init",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.VisitCountGuard{TransitionID: "review", MaxVisits: 3},
					},
				},
				OutputArcs: []petri.Arc{
					{ID: "failed-out", Name: "failed", PlaceID: "p-failed", Direction: petri.ArcOutput},
				},
			},
		},
	}

	belowThreshold := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-work": {
			ID:      "tok-work",
			PlaceID: "p-init",
			History: interfaces.TokenHistory{TotalVisits: map[string]int{"review": 2}},
		},
	})
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &belowThreshold); len(enabled) != 0 {
		t.Fatalf("enabled transitions below threshold = %d, want 0", len(enabled))
	}

	atThreshold := makeTestSnapshot(map[string]*interfaces.Token{
		"tok-work": {
			ID:      "tok-work",
			PlaceID: "p-init",
			History: interfaces.TokenHistory{TotalVisits: map[string]int{"review": 3}},
		},
	})
	enabled := eval.FindEnabledTransitions(context.Background(), n, &atThreshold)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions at threshold = %d, want 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["work"]); strings.Join(got, ",") != "tok-work" {
		t.Fatalf("work binding tokens = %v, want [tok-work]", got)
	}
}

func TestEnablementEvaluator_LogsNoInputArcs(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Transitions: map[string]*petri.Transition{
			"t1": {ID: "t1", Name: "empty", InputArcs: nil},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{})
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}
	if entry := logger.findEntry("no input arcs"); entry == nil {
		t.Fatal("expected log entry containing 'no input arcs'")
	}
}

func TestEnablementEvaluator_NilLoggerDoesNotPanic(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p1": {ID: "p1"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID: "t1",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p1"},
	})
	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled transition, got %d", len(enabled))
	}
}

func TestEnablementEvaluator_MultipleTransitions_LogsEach(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger)

	n := &state.Net{
		Places: map[string]*petri.Place{
			"p1": {ID: "p1"},
			"p2": {ID: "p2"},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:   "t1",
				Name: "first",
				InputArcs: []petri.Arc{
					{ID: "a1", Name: "work", PlaceID: "p1", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
			"t2": {
				ID:   "t2",
				Name: "second",
				InputArcs: []petri.Arc{
					{ID: "a2", Name: "input", PlaceID: "p2", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
			},
		},
	}

	marking := makeTestSnapshot(map[string]*interfaces.Token{
		"tok1": {ID: "tok1", PlaceID: "p1"},
	})
	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled transition, got %d", len(enabled))
	}
	if enabledCount := logger.countEntries("transition enabled"); enabledCount != 1 {
		t.Errorf("expected 1 'transition enabled' log, got %d", enabledCount)
	}
	if disabledCount := logger.countEntries("transition disabled"); disabledCount != 1 {
		t.Errorf("expected 1 'transition disabled' log, got %d", disabledCount)
	}
}

func sameNameGuardNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"plan:ready": {ID: "plan:ready"},
			"task:ready": {ID: "task:ready"},
		},
		Transitions: map[string]*petri.Transition{
			"match-items": {
				ID:   "match-items",
				Name: "match-items",
				InputArcs: []petri.Arc{
					{ID: "plan-in", Name: "plan", PlaceID: "plan:ready", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{
						ID:          "task-in",
						Name:        "task",
						PlaceID:     "task:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.SameNameGuard{MatchBinding: "plan"},
					},
				},
			},
		},
	}
}
