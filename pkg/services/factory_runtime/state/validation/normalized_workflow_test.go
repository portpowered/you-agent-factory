package validation

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
)

func TestCompositeValidator_AcceptsNormalizedMultiStageWorkflow(t *testing.T) {
	t.Parallel()

	net := &state.Net{
		ID: "modified-code-factory",
		Places: map[string]*petri.Place{
			"task:init":       {ID: "task:init", TypeID: "task", State: "init"},
			"task:processing": {ID: "task:processing", TypeID: "task", State: "processing"},
			"task:complete":   {ID: "task:complete", TypeID: "task", State: "complete"},
			"task:failed":     {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		Transitions: map[string]*petri.Transition{
			"execute-task": multiStageValidationTransition(
				"execute-task",
				"task:init",
				"task:processing",
			),
			"finish-task": multiStageValidationTransition(
				"finish-task",
				"task:processing",
				"task:complete",
			),
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "processing", Category: state.StateCategoryProcessing},
					{Value: "complete", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
		Resources: map[string]*state.ResourceDef{},
	}
	state.NormalizeTransitionTopology(net, nil)

	validator := NewCompositeValidator(
		&ReachabilityValidator{},
		&CompletenessValidator{},
		&BoundednessValidator{},
		&TypeSafetyValidator{},
	)
	for _, violation := range validator.Validate(net) {
		if violation.Level == ViolationError {
			t.Fatalf(
				"normalized workflow has error violation %s: %s (at %s)",
				violation.Code,
				violation.Message,
				violation.Location,
			)
		}
	}
}

func multiStageValidationTransition(
	id string,
	inputPlace string,
	outputPlace string,
) *petri.Transition {
	return &petri.Transition{
		ID:         id,
		Name:       id,
		Type:       petri.TransitionNormal,
		WorkerType: id + "-worker",
		InputArcs: []petri.Arc{{
			Name:        "work",
			PlaceID:     inputPlace,
			Direction:   petri.ArcInput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
		OutputArcs: []petri.Arc{{
			PlaceID:     outputPlace,
			Direction:   petri.ArcOutput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
		FailureArcs: []petri.Arc{{
			PlaceID:     "task:failed",
			Direction:   petri.ArcOutput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
	}
}
