package subsystems_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
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
			transitioner := subsystems.NewTransitioner(
				net,
				nil,
				subsystems.WithTransitionerClock(func() time.Time { return now }),
				subsystems.WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
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
	transitioner := subsystems.NewTransitioner(
		net,
		nil,
		subsystems.WithTransitionerClock(func() time.Time { return now }),
		subsystems.WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
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
		decision  string
		wantPlace string
		wantLabel string
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
			transitioner := subsystems.NewTransitioner(
				net,
				nil,
				subsystems.WithTransitionerClock(func() time.Time { return now }),
				subsystems.WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
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

func TestTransitioner_PackagedGoalStructuredReviewEnvelopeRejectsMalformedAndUnknownDecisions(t *testing.T) {
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
		name       string
		raw        string
		wantErr    string
		wantFeedbk string
	}{
		{
			name:    "malformed json",
			raw:     `not-json`,
			wantErr: "invalid JSON",
		},
		{
			name:       "unknown decision",
			raw:        `{"decision":"MAYBE","feedback":"needs another pass"}`,
			wantErr:    `unknown decision "MAYBE"`,
			wantFeedbk: "needs another pass",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, time.June, 20, 15, 30, 0, 0, time.UTC)
			transitioner := subsystems.NewTransitioner(
				net,
				nil,
				subsystems.WithTransitionerClock(func() time.Time { return now }),
				subsystems.WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
					Workstations: workstations,
				}),
			)
			snapshot := packagedGoalStructuredReviewMalformedEnvelopeSnapshot(t, now, reviewTransition.ID, tc.raw)
			if tc.wantFeedbk != "" {
				if len(snapshot.Results) != 1 || snapshot.Results[0].Feedback != tc.wantFeedbk {
					t.Fatalf("result feedback = %q, want %q preserved for inspection", snapshot.Results[0].Feedback, tc.wantFeedbk)
				}
			}

			tickResult, err := transitioner.Execute(context.Background(), snapshot)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if tickResult == nil || len(tickResult.Mutations) != 1 {
				t.Fatalf("mutations = %#v, want one failure mutation", tickResult)
			}
			if tickResult.Mutations[0].ToPlace != "goal:failed" {
				t.Fatalf("routed place = %q, want goal:failed", tickResult.Mutations[0].ToPlace)
			}
			if len(tickResult.CompletedDispatches) != 1 {
				t.Fatalf("completed dispatches = %#v, want 1", tickResult.CompletedDispatches)
			}
			completed := tickResult.CompletedDispatches[0]
			if completed.Outcome != interfaces.OutcomeFailed {
				t.Fatalf("completed outcome = %s, want FAILED", completed.Outcome)
			}
			if completed.SelectedClassificationLabel != "" {
				t.Fatalf("selected classification label = %q, want empty on malformed envelope", completed.SelectedClassificationLabel)
			}
			if !strings.Contains(completed.Reason, tc.wantErr) {
				t.Fatalf("completed reason = %q, want %q", completed.Reason, tc.wantErr)
			}
		})
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

func packagedGoalStructuredReviewMalformedEnvelopeSnapshot(
	t *testing.T,
	now time.Time,
	transitionID string,
	raw string,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	result := goal.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed("d-structured-review", transitionID, raw)

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
