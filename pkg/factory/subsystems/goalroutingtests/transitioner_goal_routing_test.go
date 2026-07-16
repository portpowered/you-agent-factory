package subsystems_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestBuiltInGoalFactoryJSON_ExecuteRepeaterConsumesLoopInput(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(goal.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	execute, ok := findPackagedGoalWorkstation(cfg.Workstations, goal.PackagedExecuteWorkstationName)
	if !ok {
		t.Fatalf("missing workstation %q", goal.PackagedExecuteWorkstationName)
	}
	if execute.Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("execute workstation kind = %q, want %q", execute.Kind, interfaces.WorkstationKindRepeater)
	}

	if len(execute.Inputs) != 1 {
		t.Fatalf("execute inputs = %#v, want one loop input", execute.Inputs)
	}
	if input := execute.Inputs[0]; input.WorkTypeName != goal.PackagedGoalWorkTypeName || input.StateName != "init" {
		t.Fatalf("execute input = %#v, want goal:init", input)
	}
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

	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(goal.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	net, err := (&factoryconfig.ConfigMapper{}).Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}
	workstation, ok := findPackagedGoalWorkstation(cfg.Workstations, goal.PackagedExecuteWorkstationName)
	if !ok {
		t.Fatalf("missing workstation %q", goal.PackagedExecuteWorkstationName)
	}
	return net, &workstation, findTransitionByName(t, net, goal.PackagedExecuteWorkstationName)
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
		subsystems.WithTransitionerClock(func() time.Time { return now }),
		subsystems.WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{workstation.Name: workstation},
		}),
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
				WorkstationName: goal.PackagedExecuteWorkstationName,
				StartTime:       now.Add(-time.Second),
				ConsumedTokens: []factorytoken.Token{{
					ID:        "goal-token",
					PlaceID:   inputPlace,
					CreatedAt: now.Add(-time.Hour),
					EnteredAt: now.Add(-time.Hour),
					Color: factorytoken.Color{
						WorkID:     "goal-work",
						WorkTypeID: goal.PackagedGoalWorkTypeName,
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

func findTransitionByName(t *testing.T, net *state.Net, name string) *petri.Transition {
	t.Helper()

	for _, transition := range net.Transitions {
		if transition != nil && transition.Name == name {
			return transition
		}
	}
	t.Fatalf("missing transition %q", name)
	return nil
}

func findPackagedGoalWorkstation(workstations []interfaces.FactoryWorkstationConfig, name string) (interfaces.FactoryWorkstationConfig, bool) {
	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation, true
		}
	}
	return interfaces.FactoryWorkstationConfig{}, false
}
