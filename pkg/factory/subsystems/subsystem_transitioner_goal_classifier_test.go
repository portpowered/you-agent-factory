package subsystems

import (
	"context"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestTransitioner_PackagedGoalReviewClassifierRoutesPlainDecisionLabels(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	mapper := &factoryconfig.ConfigMapper{}
	net, err := mapper.Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}

	reviewTransition := findTransitionByWorkstationName(t, net, goal.PackagedReviewWorkstationName)
	workstations := workstationLookupFromConfig(cfg.Workstations)

	cases := []struct {
		label     string
		wantPlace string
	}{
		{label: "accepted", wantPlace: "goal:complete"},
		{label: "needs_changes", wantPlace: "goal:plan"},
		{label: "tests_failed", wantPlace: "goal:plan"},
		{label: "needs_human", wantPlace: "goal:needs-human"},
		{label: "blocked", wantPlace: "goal:blocked"},
		{label: "interrupted", wantPlace: "goal:interrupted"},
		{label: "failed", wantPlace: "goal:failed"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			now := time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC)
			transitioner := NewTransitioner(
				net,
				nil,
				WithTransitionerClock(func() time.Time { return now }),
				WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
					Workstations: workstations,
				}),
			)
			snapshot := packagedGoalReviewClassifierSnapshot(now, reviewTransition.ID, tc.label)

			result, err := transitioner.Execute(context.Background(), snapshot)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result == nil || len(result.Mutations) != 1 {
				t.Fatalf("mutations = %#v, want one routed classifier mutation", result)
			}
			if result.Mutations[0].ToPlace != tc.wantPlace {
				t.Fatalf("routed place = %q, want %q", result.Mutations[0].ToPlace, tc.wantPlace)
			}
			if len(result.CompletedDispatches) != 1 {
				t.Fatalf("completed dispatches = %#v, want 1", result.CompletedDispatches)
			}
			completed := result.CompletedDispatches[0]
			if completed.Outcome != interfaces.OutcomeAccepted {
				t.Fatalf("completed outcome = %s, want ACCEPTED", completed.Outcome)
			}
			if completed.SelectedClassificationLabel != tc.label {
				t.Fatalf("selected classification label = %q, want %q", completed.SelectedClassificationLabel, tc.label)
			}
		})
	}
}

func TestTransitioner_PackagedGoalReviewClassifierUnknownLabelRoutesToFailed(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	mapper := &factoryconfig.ConfigMapper{}
	net, err := mapper.Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}

	reviewTransition := findTransitionByWorkstationName(t, net, goal.PackagedReviewWorkstationName)
	workstations := workstationLookupFromConfig(cfg.Workstations)

	now := time.Date(2026, time.June, 20, 15, 0, 0, 0, time.UTC)
	transitioner := NewTransitioner(
		net,
		nil,
		WithTransitionerClock(func() time.Time { return now }),
		WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
			Workstations: workstations,
		}),
	)
	snapshot := packagedGoalReviewClassifierSnapshot(now, reviewTransition.ID, "MAYBE")

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want one failure mutation", result)
	}
	if result.Mutations[0].ToPlace != "goal:failed" {
		t.Fatalf("routed place = %q, want goal:failed", result.Mutations[0].ToPlace)
	}
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1", result.CompletedDispatches)
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("completed outcome = %s, want FAILED", completed.Outcome)
	}
	if completed.SelectedClassificationLabel != "" {
		t.Fatalf("selected classification label = %q, want empty on unknown classifier label", completed.SelectedClassificationLabel)
	}
	if !strings.Contains(completed.Reason, `classifier label "MAYBE" did not match any authored classification route`) {
		t.Fatalf("completed reason = %q, want unknown classifier label explanation", completed.Reason)
	}
}

func findTransitionByWorkstationName(t *testing.T, net *state.Net, workstationName string) *petri.Transition {
	t.Helper()

	for _, transition := range net.Transitions {
		if transition != nil && transition.Name == workstationName {
			return transition
		}
	}
	t.Fatalf("missing transition for workstation %q", workstationName)
	return nil
}

func workstationLookupFromConfig(workstations []interfaces.FactoryWorkstationConfig) map[string]*interfaces.FactoryWorkstationConfig {
	lookup := make(map[string]*interfaces.FactoryWorkstationConfig, len(workstations))
	for i := range workstations {
		workstation := workstations[i]
		lookup[workstation.Name] = &workstation
	}
	return lookup
}

func packagedGoalReviewClassifierSnapshot(now time.Time, transitionID, label string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-review": {
				DispatchID:      "d-review",
				TransitionID:    transitionID,
				WorkstationName: goal.PackagedReviewWorkstationName,
				StartTime:       now.Add(-time.Second),
				ConsumedTokens: []interfaces.Token{{
					ID:        "tok-review",
					PlaceID:   "goal:review",
					CreatedAt: now.Add(-time.Hour),
					EnteredAt: now.Add(-time.Hour),
					Color: interfaces.TokenColor{
						WorkID:     "work-goal-1",
						WorkTypeID: goal.PackagedGoalWorkTypeName,
					},
					History: interfaces.TokenHistory{
						TotalVisits:         map[string]int{},
						ConsecutiveFailures: map[string]int{},
						PlaceVisits:         map[string]int{},
					},
				}},
			},
		},
		Results: []interfaces.WorkResult{{
			DispatchID:   "d-review",
			TransitionID: transitionID,
			Outcome:      interfaces.OutcomeAccepted,
			Output:       label,
		}},
	}
}
