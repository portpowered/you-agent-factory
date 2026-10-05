package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestEnablementEvaluator_LogsEnabledTransition(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger, testNow, nil)

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

	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"tok1": {ID: "tok1", PlaceID: "p1", Color: factorytoken.Color{WorkID: "w1"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled transition, got %d", len(enabled))
	}

	if entry := logger.findEntry("transition enabled"); entry == nil || entry.level != "info" || !reflect.DeepEqual(entry.args, []any{"transitionID", "t1", "transitionName", "do-work", "workerType", "agent", "bindingCount", 1}) {
		t.Fatal("expected 'transition enabled' log entry")
	}
	if summary := logger.findEntry("evaluation complete"); summary == nil || summary.level != "debug" || !reflect.DeepEqual(summary.args, []any{"totalTransitions", 1, "enabledCount", 1}) {
		t.Fatal("expected 'evaluation complete' log entry")
	}
}

func TestEnablementEvaluator_LogsDisabledInsufficientTokens(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger, testNow, nil)

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

	marking := makeTestSnapshot(map[string]*factorytoken.Token{})
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}
	if entry := logger.findEntry("transition disabled"); entry == nil || entry.level != "debug" || !strings.Contains(fmt.Sprint(entry.args), "t1") {
		t.Fatal("expected 'transition disabled' log entry")
	}
}

func TestEnablementEvaluator_LogsDisabledGuardFailed(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger, testNow, nil)

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

	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"tok-work":   {ID: "tok-work", PlaceID: "p-work", Color: factorytoken.Color{WorkID: "w1"}},
		"tok-review": {ID: "tok-review", PlaceID: "p-review", Color: factorytoken.Color{WorkID: "r1", ParentID: "WRONG"}},
	})

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}
	if entry := logger.findEntry("guard failed"); entry == nil || entry.level != "debug" || !strings.Contains(fmt.Sprint(entry.args), "t1") {
		t.Fatal("expected log entry containing 'guard failed'")
	}
}

func TestEnablementEvaluator_BindsMultipleNamedGuardedInputs(t *testing.T) {
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)

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
	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"tok-req":          {ID: "tok-req", PlaceID: "p-req", Color: factorytoken.Color{WorkID: "w1"}},
		"tok-design-match": {ID: "tok-design-match", PlaceID: "p-design", Color: factorytoken.Color{WorkID: "d1", ParentID: "w1"}},
		"tok-design-other": {ID: "tok-design-other", PlaceID: "p-design", Color: factorytoken.Color{WorkID: "d2", ParentID: "other"}},
		"tok-code-match":   {ID: "tok-code-match", PlaceID: "p-code", Color: factorytoken.Color{WorkID: "c1", ParentID: "w1"}},
		"tok-code-other":   {ID: "tok-code-other", PlaceID: "p-code", Color: factorytoken.Color{WorkID: "c2", ParentID: "other"}},
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
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)

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
	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"tok-parent":      {ID: "tok-parent", PlaceID: "p-parent", Color: factorytoken.Color{WorkID: "w1"}},
		"tok-child-a":     {ID: "tok-child-a", PlaceID: "p-children", Color: factorytoken.Color{WorkID: "c1", ParentID: "w1"}},
		"tok-child-b":     {ID: "tok-child-b", PlaceID: "p-children", Color: factorytoken.Color{WorkID: "c2", ParentID: "w1"}},
		"tok-child-other": {ID: "tok-child-other", PlaceID: "p-children", Color: factorytoken.Color{WorkID: "c3", ParentID: "other"}},
	})
	marking.ParentChildRegistrations = petri.ParentChildRegistrationProjection{
		"w1": {Children: []factorytoken.Token{*marking.Tokens["tok-child-a"], *marking.Tokens["tok-child-b"]}, Complete: true},
	}

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

func TestEnablementEvaluator_AllChildrenCompleteWaitsForProcessingAndLateChild(t *testing.T) {
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)
	n := &state.Net{
		Places: map[string]*petri.Place{
			"parent:waiting":   {ID: "parent:waiting", TypeID: "parent", State: "waiting"},
			"child:complete":   {ID: "child:complete", TypeID: "child", State: "complete"},
			"child:processing": {ID: "child:processing", TypeID: "child", State: "processing"},
		},
		WorkTypes: map[string]*state.WorkType{
			"parent": {ID: "parent", States: []state.StateDefinition{{Value: "waiting", Category: state.StateCategoryProcessing}}},
			"child":  {ID: "child", States: []state.StateDefinition{{Value: "processing", Category: state.StateCategoryProcessing}, {Value: "complete", Category: state.StateCategoryTerminal}}},
		},
		Transitions: map[string]*petri.Transition{
			"join": {
				ID:   "join",
				Name: "join",
				InputArcs: []petri.Arc{
					{ID: "parent-in", Name: "parent", PlaceID: "parent:waiting", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "children-in", Name: "children", PlaceID: "child:complete", Direction: petri.ArcInput, Mode: interfaces.ArcModeObserve, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityAll}, Guard: &petri.AllWithParentGuard{MatchBinding: "parent"}},
				},
			},
		},
	}

	parent := &factorytoken.Token{ID: "tok-parent", PlaceID: "parent:waiting", Color: factorytoken.Color{WorkID: "parent-1", WorkTypeID: "parent", DataType: factorytoken.DataTypeWork}}
	first := &factorytoken.Token{ID: "tok-child-1", PlaceID: "child:complete", Color: factorytoken.Color{WorkID: "child-1", WorkTypeID: "child", ParentID: "parent-1", DataType: factorytoken.DataTypeWork}}
	processing := &factorytoken.Token{ID: "tok-child-2", PlaceID: "child:processing", Color: factorytoken.Color{WorkID: "child-2", WorkTypeID: "child", ParentID: "parent-1", DataType: factorytoken.DataTypeWork}}

	initial := makeTestSnapshot(map[string]*factorytoken.Token{parent.ID: parent, first.ID: first})
	initial.ParentChildRegistrations = petri.ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{*first}, Complete: false},
	}
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &initial); len(enabled) != 0 {
		t.Fatalf("fan-in enabled before the registered set was complete: %#v", enabled)
	}

	late := makeTestSnapshot(map[string]*factorytoken.Token{parent.ID: parent, first.ID: first, processing.ID: processing})
	late.ParentChildRegistrations = petri.ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{*first, *processing}, Complete: false},
	}
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &late); len(enabled) != 0 {
		t.Fatalf("fan-in enabled with a late registration still open: %#v", enabled)
	}

	processing.PlaceID = "child:complete"
	terminal := makeTestSnapshot(map[string]*factorytoken.Token{parent.ID: parent, first.ID: first, processing.ID: processing})
	terminal.ParentChildRegistrations = petri.ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{*first, *processing}, Complete: true},
	}
	enabled := eval.FindEnabledTransitions(context.Background(), n, &terminal)
	if len(enabled) != 1 {
		t.Fatalf("fan-in enabled transitions = %d, want exactly 1 after final child terminal", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["children"]); strings.Join(got, ",") != "tok-child-1,tok-child-2" {
		t.Fatalf("fan-in child binding = %v, want both registered children", got)
	}
}

func TestEnablementEvaluator_AllChildrenCompleteAcceptsDistinctTerminalPlaces(t *testing.T) {
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)
	n := &state.Net{
		Places: map[string]*petri.Place{
			"parent:waiting": {ID: "parent:waiting", TypeID: "parent", State: "waiting"},
			"child:complete": {ID: "child:complete", TypeID: "child", State: "complete"},
			"child:skipped":  {ID: "child:skipped", TypeID: "child", State: "skipped"},
		},
		WorkTypes: map[string]*state.WorkType{
			"parent": {ID: "parent", States: []state.StateDefinition{{Value: "waiting", Category: state.StateCategoryProcessing}}},
			"child": {ID: "child", States: []state.StateDefinition{
				{Value: "complete", Category: state.StateCategoryTerminal},
				{Value: "skipped", Category: state.StateCategoryTerminal},
			}},
		},
		Transitions: map[string]*petri.Transition{
			"join": {
				ID:   "join",
				Name: "join",
				InputArcs: []petri.Arc{
					{ID: "parent-in", Name: "parent", PlaceID: "parent:waiting", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "children-in", Name: "children", PlaceID: "child:complete", Direction: petri.ArcInput, Mode: interfaces.ArcModeObserve, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityAll}, Guard: &petri.AllWithParentGuard{MatchBinding: "parent"}},
				},
			},
		},
	}

	parent := &factorytoken.Token{ID: "tok-parent", PlaceID: "parent:waiting", Color: factorytoken.Color{WorkID: "parent-1", WorkTypeID: "parent", DataType: factorytoken.DataTypeWork}}
	complete := &factorytoken.Token{ID: "tok-child-complete", PlaceID: "child:complete", Color: factorytoken.Color{WorkID: "child-1", WorkTypeID: "child", ParentID: "parent-1", DataType: factorytoken.DataTypeWork}}
	skipped := &factorytoken.Token{ID: "tok-child-skipped", PlaceID: "child:skipped", Color: factorytoken.Color{WorkID: "child-2", WorkTypeID: "child", ParentID: "parent-1", DataType: factorytoken.DataTypeWork}}
	marking := makeTestSnapshot(map[string]*factorytoken.Token{parent.ID: parent, complete.ID: complete, skipped.ID: skipped})
	marking.ParentChildRegistrations = petri.ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{*complete, *skipped}, Complete: true},
	}

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("fan-in enabled transitions = %d, want exactly 1", len(enabled))
	}
	if got := tokenIDs(enabled[0].Bindings["children"]); strings.Join(got, ",") != complete.ID {
		t.Fatalf("fan-in binding surface = %v, want [%s] while validating both terminal places", got, complete.ID)
	}
}

func TestEnablementEvaluator_SameNameGuardEnablesOnMatchingNames(t *testing.T) {
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)
	n := sameNameGuardNet()
	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", Color: factorytoken.Color{Name: "alpha"}},
		"task-alpha": {ID: "task-alpha", PlaceID: "task:ready", Color: factorytoken.Color{Name: "alpha"}},
		"task-beta":  {ID: "task-beta", PlaceID: "task:ready", Color: factorytoken.Color{Name: "beta"}},
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
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)
	n := sameNameGuardNet()
	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", Color: factorytoken.Color{Name: "alpha"}},
		"task-beta":  {ID: "task-beta", PlaceID: "task:ready", Color: factorytoken.Color{Name: "beta"}},
	})

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled transitions = %d, want 0", len(enabled))
	}
}

func TestEnablementEvaluator_SameNameGuardFindsLaterMatchingBinding(t *testing.T) {
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)

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
	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"task-alpha": {ID: "task-alpha", PlaceID: "task:to-complete", Color: factorytoken.Color{Name: "alpha"}},
		"task-zeta":  {ID: "task-zeta", PlaceID: "task:to-complete", Color: factorytoken.Color{Name: "zeta"}},
		"idea-zeta":  {ID: "idea-zeta", PlaceID: "idea:to-complete", Color: factorytoken.Color{Name: "zeta"}},
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

func TestEnablementEvaluator_SameNameGuardFactoryConsumeInputOrder(t *testing.T) {
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)

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
					{
						ID:          "idea-in",
						Name:        "idea:to-complete:to:consume",
						PlaceID:     "idea:to-complete",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.SameNameGuard{MatchBinding: "task:to-complete:to:consume"},
					},
					{
						ID:          "task-in",
						Name:        "task:to-complete:to:consume",
						PlaceID:     "task:to-complete",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.DependencyGuard{},
					},
				},
			},
		},
	}
	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"idea-cell": {ID: "idea-cell", PlaceID: "idea:to-complete", Color: factorytoken.Color{Name: "dynamic-workflows-cell-cli-validate-list"}},
		"task-cell": {ID: "task-cell", PlaceID: "task:to-complete", Color: factorytoken.Color{Name: "dynamic-workflows-cell-cli-validate-list"}},
	})

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1; marking=%#v", len(enabled), marking.PlaceTokens)
	}
}

func TestEnablementEvaluator_VisitCountGuardEnablesAtThreshold(t *testing.T) {
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)

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

	belowThreshold := makeTestSnapshot(map[string]*factorytoken.Token{
		"tok-work": {
			ID:      "tok-work",
			PlaceID: "p-init",
			History: factorytoken.History{TotalVisits: map[string]int{"review": 2}},
		},
	})
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &belowThreshold); len(enabled) != 0 {
		t.Fatalf("enabled transitions below threshold = %d, want 0", len(enabled))
	}

	atThreshold := makeTestSnapshot(map[string]*factorytoken.Token{
		"tok-work": {
			ID:      "tok-work",
			PlaceID: "p-init",
			History: factorytoken.History{TotalVisits: map[string]int{"review": 3}},
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
	eval := NewEnablementEvaluator(logger, testNow, nil)

	n := &state.Net{
		Transitions: map[string]*petri.Transition{
			"t1": {ID: "t1", Name: "empty", InputArcs: nil},
		},
	}

	marking := makeTestSnapshot(map[string]*factorytoken.Token{})
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}
	if entry := logger.findEntry("no input arcs"); entry == nil || entry.level != "debug" || !strings.Contains(fmt.Sprint(entry.args), "t1") {
		t.Fatal("expected log entry containing 'no input arcs'")
	}
}

func TestEnablementEvaluator_ExplicitNoopPreservesEnablement(t *testing.T) {
	eval := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)

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

	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"tok1": {ID: "tok1", PlaceID: "p1"},
	})
	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled transition, got %d", len(enabled))
	}
}

func TestEnablementEvaluator_MultipleTransitions_LogsEach(t *testing.T) {
	logger := &capturingLogger{}
	eval := NewEnablementEvaluator(logger, testNow, nil)

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

	marking := makeTestSnapshot(map[string]*factorytoken.Token{
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

func TestEnablementEvaluator_SameNameJoinGatesSecondaryDependency(t *testing.T) {
	for _, tc := range []struct {
		name      string
		peerFirst bool
	}{
		{name: "peer input declared first", peerFirst: true},
		{name: "dependency input declared first", peerFirst: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evaluator := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)
			net := sameNameDependencyNet(tc.peerFirst)

			blocked := sameNameDependencySnapshot("prerequisite:pending")
			if enabled := evaluator.FindEnabledTransitions(context.Background(), net, &blocked); len(enabled) != 0 {
				t.Fatalf("enabled transitions before secondary prerequisite completion = %d, want 0", len(enabled))
			}

			completed := sameNameDependencySnapshot("prerequisite:complete")
			enabled := evaluator.FindEnabledTransitions(context.Background(), net, &completed)
			if len(enabled) != 1 {
				t.Fatalf("enabled transitions after secondary prerequisite completion = %d, want 1", len(enabled))
			}
			assertJoinedBindingToken(t, enabled[0].Bindings, "idea", "idea-token")
			assertJoinedBindingToken(t, enabled[0].Bindings, "task", "task-token")
		})
	}
}

func TestEnablementEvaluator_DependencyBindingPreflightHandlesNestedGuards(t *testing.T) {
	evaluator := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil)
	transition := &petri.Transition{
		InputArcs: []petri.Arc{{
			Guard: &petri.AllGuard{Guards: []petri.Guard{
				&petri.SameNameGuard{MatchBinding: "peer"},
				&petri.DependencyGuard{},
			}},
		}},
	}

	if !transitionUsesDependencyGuard(transition) {
		t.Fatal("expected nested dependency guard to be detected")
	}
	if transitionUsesDependencyGuard(nil) {
		t.Fatal("nil transition must not report a dependency guard")
	}
	if guardUsesDependencyGuard(&petri.AllGuard{Guards: []petri.Guard{&petri.SameNameGuard{}}}) {
		t.Fatal("unrelated nested guard must not report a dependency guard")
	}
	if evaluator.bindingDependenciesMet(transition, nil, nil) {
		t.Fatal("dependency binding must fail closed when its marking snapshot is missing")
	}
}

func sameNameDependencyNet(peerFirst bool) *state.Net {
	peerArc := petri.Arc{
		ID:          "idea-in",
		Name:        "idea",
		PlaceID:     "idea:init",
		Direction:   petri.ArcInput,
		Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		Guard:       &petri.SameNameGuard{MatchBinding: "task"},
	}
	dependencyArc := petri.Arc{
		ID:          "task-in",
		Name:        "task",
		PlaceID:     "task:init",
		Direction:   petri.ArcInput,
		Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		Guard:       &petri.DependencyGuard{},
	}
	inputArcs := []petri.Arc{dependencyArc, peerArc}
	if peerFirst {
		inputArcs = []petri.Arc{peerArc, dependencyArc}
	}
	return &state.Net{
		Transitions: map[string]*petri.Transition{
			"consume": {
				ID:        "consume",
				Name:      "consume",
				InputArcs: inputArcs,
			},
		},
	}
}

func sameNameDependencySnapshot(prerequisitePlace string) petri.MarkingSnapshot {
	tokens := map[string]*factorytoken.Token{
		"idea-token": {
			ID:      "idea-token",
			PlaceID: "idea:init",
			Color: factorytoken.Color{
				Name:       "joined-work",
				WorkID:     "work-idea",
				WorkTypeID: "idea",
				DataType:   factorytoken.DataTypeWork,
				Relations: []work.Relation{{
					Type:          work.RelationDependsOn,
					TargetWorkID:  "work-prerequisite",
					RequiredState: "complete",
				}},
			},
		},
		"task-token": {
			ID:      "task-token",
			PlaceID: "task:init",
			Color: factorytoken.Color{
				Name:       "joined-work",
				WorkID:     "work-task",
				WorkTypeID: "task",
				DataType:   factorytoken.DataTypeWork,
			},
		},
		"prerequisite-token": {
			ID:      "prerequisite-token",
			PlaceID: prerequisitePlace,
			Color: factorytoken.Color{
				WorkID:     "work-prerequisite",
				WorkTypeID: "prerequisite",
				DataType:   factorytoken.DataTypeWork,
			},
		},
	}
	placeTokens := make(map[string][]string, len(tokens))
	for id, token := range tokens {
		placeTokens[token.PlaceID] = append(placeTokens[token.PlaceID], id)
	}
	return petri.MarkingSnapshot{Tokens: tokens, PlaceTokens: placeTokens}
}

func assertJoinedBindingToken(t *testing.T, bindings any, name, wantID string) {
	t.Helper()
	ids := tokenIDs(bindingTokens(bindings, name))
	if ids == nil {
		t.Fatalf("missing binding %q", name)
	}
	if len(ids) != 1 || ids[0] != wantID {
		t.Fatalf("binding %q = %#v, want token %q", name, ids, wantID)
	}
}

func bindingTokens(bindings any, name string) any {
	switch values := bindings.(type) {
	case map[string][]factorytoken.Token:
		return values[name]
	case map[string][]workerexecution.Token:
		return values[name]
	default:
		return nil
	}
}

func TestEnablementSelectedLoggerParity(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"enabled", "insufficient", "guard", "no-input", "empty", "nil-snapshot"} {
		t.Run(name, func(t *testing.T) { t.Parallel(); checkEnablementLoggerParity(t, name) })
	}
}

func selectedEnablementFixture(name string) (*state.Net, petri.MarkingSnapshot, string) {
	n := sameNameGuardNet()
	marking := makeTestSnapshot(map[string]*factorytoken.Token{
		"plan-alpha": {ID: "plan-alpha", PlaceID: "plan:ready", CreatedAt: testNow(), EnteredAt: testNow(), Color: factorytoken.Color{Name: "alpha"}},
		"task-alpha": {ID: "task-alpha", PlaceID: "task:ready", CreatedAt: testNow(), EnteredAt: testNow(), Color: factorytoken.Color{Name: "alpha"}},
	})
	reason := ""
	switch name {
	case "enabled":
		for _, tr := range n.Transitions {
			second := *tr
			second.ID = "zz-second"
			second.Name = "second"
			second.WorkerType = "script"
			n.Transitions[second.ID] = &second
			break
		}
	case "insufficient":
		marking = makeTestSnapshot(nil)
		reason = "insufficient tokens"
	case "guard":
		marking.Tokens["task-alpha"].Color.Name = "beta"
		reason = "guard failed"
	case "no-input":
		for _, tr := range n.Transitions {
			tr.InputArcs = nil
		}
		reason = "no input arcs"
	case "empty":
		n.Transitions = nil
	}
	return n, marking, reason
}

func checkEnablementLoggerParity(t *testing.T, name string) {
	t.Helper()
	n, marking, reason := selectedEnablementFixture(name)
	before, err := json.Marshal(marking)
	if err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zapcore.DebugLevel)
	capture := logging.NewZapLogger(zap.New(core).With(zap.String("session_id", "enablement")), false)
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: marking, Topology: n}
	if name == "nil-snapshot" {
		snapshot = nil
	}
	got := NewEnablementEvaluator(capture, testNow, nil).FindEnabledTransitionsWithSnapshot(context.Background(), n, snapshot)
	quiet := NewEnablementEvaluator(logging.NoopLogger{}, testNow, nil).FindEnabledTransitionsWithSnapshot(context.Background(), n, snapshot)
	after, err := json.Marshal(marking)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, quiet) || string(after) != string(before) {
		t.Fatalf("decision or marking changed: capture=%+v quiet=%+v", got, quiet)
	}
	if name == "enabled" {
		assertSelectedEnabledDiagnostics(t, got, logs)
	} else if len(got) != 0 {
		t.Fatalf("unexpected enabled transitions: %+v", got)
	}
	if name == "nil-snapshot" {
		if logs.Len() != 0 {
			t.Fatal("nil snapshot emitted diagnostics")
		}
		return
	}
	assertSelectedEnablementSummary(t, logs, len(got), reason)
	for _, entry := range logs.All() {
		if entry.ContextMap()["session_id"] != "enablement" {
			t.Fatalf("lost supplied scope: %+v", entry)
		}
	}
}

func assertSelectedEnabledDiagnostics(t *testing.T, got []interfaces.EnabledTransition, logs *observer.ObservedLogs) {
	t.Helper()
	if len(got) != 2 || got[0].TransitionID >= got[1].TransitionID {
		t.Fatalf("sorted transitions = %+v", got)
	}
	entries := logs.FilterMessage("enablement: transition enabled").All()
	if len(entries) != 2 {
		t.Fatalf("enabled diagnostics = %+v", entries)
	}
	for i, entry := range entries {
		fields := entry.ContextMap()
		if entry.Level != zapcore.InfoLevel || fields["transitionID"] != got[i].TransitionID || fields["bindingCount"] != int64(2) {
			t.Fatalf("enabled diagnostic = %+v", entry)
		}
		if len(got[i].ArcModes) != 2 || len(got[i].Bindings) != 2 || got[i].Bindings["plan"][0].ID != "plan-alpha" || got[i].Bindings["task"][0].ID != "task-alpha" {
			t.Fatalf("bindings/modes = %+v", got[i])
		}
	}
}

func assertSelectedEnablementSummary(t *testing.T, logs *observer.ObservedLogs, enabled int, reason string) {
	t.Helper()
	summary := logs.FilterMessage("enablement: evaluation complete").All()
	if len(summary) != 1 || summary[0].Level != zapcore.DebugLevel || summary[0].ContextMap()["enabledCount"] != int64(enabled) {
		t.Fatalf("summary = %+v", summary)
	}
	if reason == "" {
		return
	}
	disabled := logs.FilterMessage("enablement: transition disabled").All()
	if len(disabled) == 0 {
		t.Fatal("missing disabled diagnostic")
	}
	for _, entry := range disabled {
		if entry.Level != zapcore.DebugLevel || !strings.Contains(fmt.Sprint(entry.ContextMap()["reason"]), reason) || entry.ContextMap()["transitionID"] == nil {
			t.Fatalf("disabled diagnostic = %+v", entry)
		}
	}
}

func TestEnablementSelectedLoggerScopeIsolation(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zapcore.DebugLevel)
	for _, scope := range []string{"first", "second", "quiet"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			logger := logging.NewZapLogger(zap.New(core).With(zap.String("session_id", scope), zap.String("folder_path", "/"+scope), zap.String("factory_dir", "/factory/"+scope)), false)
			if scope == "quiet" {
				logger = logging.NoopLogger{}
			}
			n, marking, _ := selectedEnablementFixture("enabled")
			enabled := NewEnablementEvaluator(logger, testNow, nil).FindEnabledTransitions(context.Background(), n, &marking)
			if len(enabled) != 2 {
				t.Fatalf("scoped decision = %+v", enabled)
			}
		})
	}
	t.Cleanup(func() {
		if logs.Len() != 6 {
			t.Fatalf("scoped logs = %+v", logs.All())
		}
		for _, entry := range logs.All() {
			fields := entry.ContextMap()
			scope := fields["session_id"]
			if scope != "first" && scope != "second" {
				t.Fatalf("unexpected scope = %+v", fields)
			}
			if fields["folder_path"] != "/"+scope.(string) || fields["factory_dir"] != "/factory/"+scope.(string) {
				t.Fatalf("crossed scope = %+v", fields)
			}
		}
	})
}
