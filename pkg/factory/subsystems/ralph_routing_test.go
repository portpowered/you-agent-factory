package subsystems_test

import (
	"context"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/ralph"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestBuiltInRalphFactory_PlannerOutputFeedsRepeatingExecutor(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(ralph.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	net, err := (&factoryconfig.ConfigMapper{}).Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}
	planner := ralphWorkstation(t, cfg.Workstations, ralph.PackagedPlanWorkstationName)
	executor := ralphWorkstation(t, cfg.Workstations, ralph.PackagedExecuteWorkstationName)
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)

	planned := executeRalphResult(t, net, planner, "ralph:init", interfaces.OutcomeAccepted, "1. Implement the requested change.", now)
	if got := planned.Mutations[0].NewToken.PlaceID; got != "ralph:execute" {
		t.Fatalf("planner destination = %q, want ralph:execute", got)
	}
	if got := string(planned.Mutations[0].NewToken.Color.Payload); got != "1. Implement the requested change." {
		t.Fatalf("planner payload = %q, want planned output", got)
	}

	continued := executeRalphResult(t, net, executor, "ralph:execute", interfaces.OutcomeContinue, "2. Continue execution.", now)
	if got := continued.Mutations[0].NewToken.PlaceID; got != "ralph:execute" {
		t.Fatalf("executor continuation destination = %q, want ralph:execute", got)
	}
	if got := string(continued.Mutations[0].NewToken.Color.Payload); got != "2. Continue execution." {
		t.Fatalf("executor continuation payload = %q, want latest execution output", got)
	}
}

func executeRalphResult(t *testing.T, net *state.Net, workstation interfaces.FactoryWorkstationConfig, place string, outcome interfaces.WorkOutcome, output string, now time.Time) *interfaces.TickResult {
	t.Helper()
	transition := ralphTransition(t, net, workstation.Name)
	transitioner := subsystems.NewTransitioner(net, nil,
		subsystems.WithTransitionerClock(func() time.Time { return now }),
		subsystems.WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{Workstations: map[string]*interfaces.FactoryWorkstationConfig{workstation.Name: &workstation}}),
	)
	result, err := transitioner.Execute(context.Background(), &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch": {
				DispatchID:      "dispatch",
				TransitionID:    transition.ID,
				WorkstationName: workstation.Name,
				ConsumedTokens: []interfaces.Token{{
					ID:      "ralph-work",
					PlaceID: place,
					Color: interfaces.TokenColor{
						WorkID:     "ralph-work",
						WorkTypeID: ralph.PackagedWorkTypeName,
						Payload:    []byte("customer request"),
					},
					History: interfaces.TokenHistory{
						TotalVisits:         map[string]int{},
						ConsecutiveFailures: map[string]int{},
						PlaceVisits:         map[string]int{},
					},
				}},
			},
		},
		Results: []interfaces.WorkResult{{DispatchID: "dispatch", TransitionID: transition.ID, Outcome: outcome, Output: output}},
	})
	if err != nil {
		t.Fatalf("Execute(%s): %v", outcome, err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want one mutation", result)
	}
	return result
}

func ralphTransition(t *testing.T, net *state.Net, name string) *petri.Transition {
	t.Helper()
	for _, transition := range net.Transitions {
		if transition.Name == name {
			return transition
		}
	}
	t.Fatalf("missing transition %q", name)
	return nil
}

func ralphWorkstation(t *testing.T, workstations []interfaces.FactoryWorkstationConfig, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation
		}
	}
	t.Fatalf("missing workstation %q", name)
	return interfaces.FactoryWorkstationConfig{}
}
