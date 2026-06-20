package subsystems

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestTransitioner_PackagedGoalStructuredReviewEnvelopeRoutesParsedDecisions(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	mapper := &factoryconfig.ConfigMapper{}
	net, err := mapper.Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}

	reviewTransition := findTransitionByWorkstationName(t, net, goal.PackagedStructuredReviewWorkstationName)
	workstations := workstationLookupFromConfig(cfg.Workstations)

	cases := []struct {
		decision   string
		wantPlace  string
		wantLabel  string
	}{
		{decision: "accepted", wantPlace: "goal:complete", wantLabel: "accepted"},
		{decision: "needs-changes", wantPlace: "goal:plan", wantLabel: "needs_changes"},
		{decision: "tests_failed", wantPlace: "goal:plan", wantLabel: "tests_failed"},
		{decision: "needs-human", wantPlace: "goal:needs-human", wantLabel: "needs_human"},
		{decision: "blocked", wantPlace: "goal:blocked", wantLabel: "blocked"},
		{decision: "interrupted", wantPlace: "goal:interrupted", wantLabel: "interrupted"},
		{decision: "failed", wantPlace: "goal:failed", wantLabel: "failed"},
	}

	for _, tc := range cases {
		t.Run(tc.decision, func(t *testing.T) {
			now := time.Date(2026, time.June, 20, 14, 0, 0, 0, time.UTC)
			transitioner := NewTransitioner(
				net,
				nil,
				WithTransitionerClock(func() time.Time { return now }),
				WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
					Workstations: workstations,
				}),
			)
			snapshot, result := packagedGoalStructuredReviewEnvelopeSnapshot(t, now, reviewTransition.ID, tc.decision)

			tickResult, err := transitioner.Execute(context.Background(), snapshot)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if tickResult == nil || len(tickResult.Mutations) != 1 {
				t.Fatalf("mutations = %#v, want one routed envelope mutation", tickResult)
			}
			if tickResult.Mutations[0].ToPlace != tc.wantPlace {
				t.Fatalf("routed place = %q, want %q", tickResult.Mutations[0].ToPlace, tc.wantPlace)
			}
			if len(tickResult.CompletedDispatches) != 1 {
				t.Fatalf("completed dispatches = %#v, want 1", tickResult.CompletedDispatches)
			}
			completed := tickResult.CompletedDispatches[0]
			if completed.Outcome != interfaces.OutcomeAccepted {
				t.Fatalf("completed outcome = %s, want ACCEPTED", completed.Outcome)
			}
			if completed.SelectedClassificationLabel != tc.wantLabel {
				t.Fatalf("selected classification label = %q, want %q", completed.SelectedClassificationLabel, tc.wantLabel)
			}
			if result.Feedback != "reviewer notes for "+tc.decision {
				t.Fatalf("result feedback = %q, want reviewer notes preserved", result.Feedback)
			}
			if result.Output != "summary for "+tc.decision {
				t.Fatalf("result output = %q, want envelope output preserved", result.Output)
			}
			if len(result.RecordedOutputWork) != 1 || result.RecordedOutputWork[0].ID != "work-"+tc.decision {
				t.Fatalf("recorded output work = %#v, want mapped work item preserved", result.RecordedOutputWork)
			}
		})
	}
}

func packagedGoalStructuredReviewEnvelopeSnapshot(
	t *testing.T,
	now time.Time,
	transitionID string,
	decision string,
) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], interfaces.WorkResult) {
	t.Helper()

	raw, err := json.Marshal(map[string]any{
		"decision": decision,
		"feedback": "reviewer notes for " + decision,
		"output":   "summary for " + decision,
		"recorded_output_work": []map[string]string{
			{
				"id":         "work-" + decision,
				"workTypeId": goal.PackagedGoalWorkTypeName,
				"state":      "execute",
				"traceId":    "trace-envelope-" + decision,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	result, err := goal.WorkResultFromGoalRoutingDecisionEnvelopeJSON("d-structured-review", transitionID, string(raw))
	if err != nil {
		t.Fatalf("WorkResultFromGoalRoutingDecisionEnvelopeJSON: %v", err)
	}

	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-structured-review": {
				DispatchID:      "d-structured-review",
				TransitionID:    transitionID,
				WorkstationName: goal.PackagedStructuredReviewWorkstationName,
				StartTime:       now.Add(-time.Second),
				ConsumedTokens: []interfaces.Token{{
					ID:        "tok-structured-review",
					PlaceID:   "goal:structured-review",
					CreatedAt: now.Add(-time.Hour),
					EnteredAt: now.Add(-time.Hour),
					Color: interfaces.TokenColor{
						WorkID:     "work-goal-structured-1",
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
		Results: []interfaces.WorkResult{result},
	}, result
}
