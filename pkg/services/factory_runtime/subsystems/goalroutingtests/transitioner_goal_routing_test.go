package subsystems_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/token_transformer"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestBuiltInGoalFactoryJSON_ExecuteRepeaterConsumesLoopInput(t *testing.T) {
	_, execute, transition := builtInGoalRepeaterFixture(t)
	if execute.Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("execute workstation kind = %q, want %q", execute.Kind, interfaces.WorkstationKindRepeater)
	}
	if len(execute.Inputs) != 1 {
		t.Fatalf("execute inputs = %#v, want one loop input", execute.Inputs)
	}
	if input := execute.Inputs[0]; input.WorkTypeName != "goal" || input.StateName != "init" {
		t.Fatalf("execute input = %#v, want goal:init", input)
	}
	assertTransitionConsumesPlace(t, transition, "goal:init")
}

func TestTransitioner_BuiltInGoalRepeaterContinueAndRejectRepeat(t *testing.T) {
	net, workstation, transition := builtInGoalRepeaterFixture(t)
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)

	for _, outcome := range []workerexecution.WorkOutcome{
		workerexecution.OutcomeContinue,
		workerexecution.OutcomeRejected,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			result := executeBuiltInGoalRepeaterResult(t, net, workstation, transition, now, "goal:init", outcome)
			assertSingleGoalMutationAtPlace(t, result, "goal:init")
			assertTransitionConsumesPlace(t, transition, result.Mutations[0].NewToken.PlaceID)
		})
	}
}

func TestTransitioner_BuiltInGoalRepeaterFailureRoutesToFailed(t *testing.T) {
	net, workstation, transition := builtInGoalRepeaterFixture(t)
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)

	result := executeBuiltInGoalRepeaterResult(t, net, workstation, transition, now, "goal:init", workerexecution.OutcomeFailed)
	assertSingleGoalMutationAtPlace(t, result, "goal:failed")
}

func builtInGoalRepeaterFixture(t *testing.T) (*state.Net, *interfaces.FactoryWorkstationConfig, *petri.Transition) {
	t.Helper()

	workstation := &interfaces.FactoryWorkstationConfig{
		Name: "execute-goal",
		Kind: interfaces.WorkstationKindRepeater,
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "goal",
			StateName:    "init",
		}},
	}
	transition := &petri.Transition{
		ID:   "execute-goal",
		Name: "execute-goal",
		InputArcs: []petri.Arc{{
			ID: "goal-input", Name: "work", PlaceID: "goal:init", Direction: petri.ArcInput,
		}},
		OutputArcs: []petri.Arc{{
			ID: "goal-complete", PlaceID: "goal:complete", Direction: petri.ArcOutput,
		}},
		FailureArcs: []petri.Arc{{
			ID: "goal-failed", PlaceID: "goal:failed", Direction: petri.ArcOutput,
		}},
	}
	net := &state.Net{
		Places: map[string]*petri.Place{
			"goal:init":     {ID: "goal:init", TypeID: "goal", State: "init"},
			"goal:complete": {ID: "goal:complete", TypeID: "goal", State: "complete"},
			"goal:failed":   {ID: "goal:failed", TypeID: "goal", State: "failed"},
		},
		Transitions: map[string]*petri.Transition{transition.ID: transition},
		WorkTypes: map[string]*state.WorkType{
			"goal": {
				ID: "goal",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	}
	state.NormalizeTransitionTopology(net, map[string]interfaces.WorkstationKind{
		transition.Name: interfaces.WorkstationKindRepeater,
	})
	return net, workstation, transition
}

func executeBuiltInGoalRepeaterResult(
	t *testing.T,
	net *state.Net,
	workstation *interfaces.FactoryWorkstationConfig,
	transition *petri.Transition,
	now time.Time,
	inputPlace string,
	outcome workerexecution.WorkOutcome,
) *interfaces.TickResult {
	t.Helper()

	transitioner := subsystems.NewTransitioner(
		net,
		nil,
		func() time.Time { return now },
		token_transformer.New(net.Places, net.WorkTypes, petri.NewWorkIDGenerator()),
		runtimefixtures.RuntimeWorkstationLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{workstation.Name: workstation},
		},
		nil,
		nil,
		repeaterWorkPropagationPolicy{},
	)
	result, err := transitioner.Execute(context.Background(), builtInGoalRepeaterSnapshot(now, transition.ID, inputPlace, outcome))
	if err != nil {
		t.Fatalf("Execute(%s): %v", outcome, err)
	}
	return result
}

func builtInGoalRepeaterSnapshot(
	now time.Time,
	transitionID string,
	inputPlace string,
	outcome workerexecution.WorkOutcome,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	dispatchID := "dispatch-" + string(outcome)
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			dispatchID: {
				DispatchID:      dispatchID,
				TransitionID:    transitionID,
				WorkstationName: "execute-goal",
				StartTime:       now.Add(-time.Second),
				ConsumedTokens: []factorytoken.Token{{
					ID:        "goal-token",
					PlaceID:   inputPlace,
					CreatedAt: now.Add(-time.Hour),
					EnteredAt: now.Add(-time.Hour),
					Color: factorytoken.Color{
						WorkID:     "goal-work",
						WorkTypeID: "goal",
						Payload:    []byte("finish the repository change"),
					},
					History: factorytoken.History{
						TotalVisits:         map[string]int{},
						ConsecutiveFailures: map[string]int{},
						PlaceVisits:         map[string]int{},
					},
				}},
			},
		},
		Results: []workerexecution.WorkResult{{
			DispatchID:   dispatchID,
			TransitionID: transitionID,
			Outcome:      outcome,
			Output:       "agent pass output",
			Feedback:     "another pass is required",
			Error:        "agent failed",
		}},
	}
}

func assertSingleGoalMutationAtPlace(t *testing.T, result *interfaces.TickResult, wantPlace string) {
	t.Helper()

	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want one mutation to %s", result, wantPlace)
	}
	mutation := result.Mutations[0]
	if mutation.ToPlace != wantPlace || mutation.NewToken.PlaceID != wantPlace {
		t.Fatalf("mutation = %#v, want destination %s", mutation, wantPlace)
	}
	if mutation.NewToken.Color.WorkID != "goal-work" {
		t.Fatalf("routed work ID = %q, want goal-work", mutation.NewToken.Color.WorkID)
	}
}

func assertTransitionConsumesPlace(t *testing.T, transition *petri.Transition, placeID string) {
	t.Helper()

	for _, arc := range transition.InputArcs {
		if arc.PlaceID == placeID {
			return
		}
	}
	t.Fatalf("transition %q does not consume routed place %q", transition.Name, placeID)
}

type repeaterWorkPropagationPolicy struct{}

func (repeaterWorkPropagationPolicy) Mode(*interfaces.FactoryWorkstationConfig) interfaces.WorkPropagationMode {
	return interfaces.WorkPropagationModeOutputAsPayload
}
