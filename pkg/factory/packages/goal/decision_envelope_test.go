package goal

import (
	"fmt"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestUsesDecisionEnvelopeOutcome_IdentifiesConfiguredWorkstation(t *testing.T) {
	if !UsesDecisionEnvelopeOutcome(&interfaces.FactoryWorkstationConfig{
		OutcomeFormat: DecisionEnvelopeOutcomeFormat,
	}) {
		t.Fatal("decision-envelope outcomeFormat should enable envelope parsing")
	}
	if UsesDecisionEnvelopeOutcome(&interfaces.FactoryWorkstationConfig{Name: "review"}) {
		t.Fatal("review workstation without outcomeFormat should not use envelope parsing")
	}
	if UsesDecisionEnvelopeOutcome(nil) {
		t.Fatal("nil workstation should not use envelope parsing")
	}
}

func TestUsesGoalRoutingDecisionEnvelope_RequiresClassificationRoutes(t *testing.T) {
	if !UsesGoalRoutingDecisionEnvelope(&interfaces.FactoryWorkstationConfig{
		OutcomeFormat: DecisionEnvelopeOutcomeFormat,
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{
			{Label: "accepted", Outputs: []interfaces.IOConfig{{WorkTypeName: "goal", StateName: "complete"}}},
		},
	}) {
		t.Fatal("decision-envelope workstation with classificationRoutes should use goal routing")
	}
	if UsesGoalRoutingDecisionEnvelope(&interfaces.FactoryWorkstationConfig{
		OutcomeFormat: DecisionEnvelopeOutcomeFormat,
	}) {
		t.Fatal("decision-envelope workstation without classificationRoutes should not use goal routing")
	}
}

func TestNormalizeGoalRoutingDecision_AcceptsHyphenAndUnderscoreForms(t *testing.T) {
	cases := []struct {
		decision string
		want     string
	}{
		{"accepted", GoalRoutingDecisionAccepted},
		{"needs-changes", GoalRoutingDecisionNeedsChanges},
		{"needs_changes", GoalRoutingDecisionNeedsChanges},
		{"tests-failed", GoalRoutingDecisionTestsFailed},
		{"needs-human", GoalRoutingDecisionNeedsHuman},
	}
	for _, tc := range cases {
		got, err := NormalizeGoalRoutingDecision(tc.decision)
		if err != nil {
			t.Fatalf("NormalizeGoalRoutingDecision(%q): %v", tc.decision, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeGoalRoutingDecision(%q) = %q, want %q", tc.decision, got, tc.want)
		}
	}
}

func TestWorkResultFromGoalRoutingDecisionEnvelopeJSON_PreservesMappedFields(t *testing.T) {
	raw := `{
		"decision": "needs-changes",
		"feedback": "Tighten acceptance criteria.",
		"output": "Rework summary.",
		"recorded_output_work": [
			{
				"id": "work-rework-1",
				"workTypeId": "goal",
				"state": "plan",
				"traceId": "trace-rework-1"
			}
		]
	}`

	result, err := WorkResultFromGoalRoutingDecisionEnvelopeJSON("dispatch-structured", "structured-review-goal", raw)
	if err != nil {
		t.Fatalf("WorkResultFromGoalRoutingDecisionEnvelopeJSON: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %q, want ACCEPTED for routing envelope", result.Outcome)
	}
	if result.SelectedClassificationLabel != GoalRoutingDecisionNeedsChanges {
		t.Fatalf("SelectedClassificationLabel = %q, want %q", result.SelectedClassificationLabel, GoalRoutingDecisionNeedsChanges)
	}
	if result.Feedback != "Tighten acceptance criteria." {
		t.Fatalf("Feedback = %q, want reviewer feedback preserved", result.Feedback)
	}
	if result.Output != "Rework summary." {
		t.Fatalf("Output = %q, want optional envelope output preserved", result.Output)
	}
	if len(result.RecordedOutputWork) != 1 || result.RecordedOutputWork[0].ID != "work-rework-1" {
		t.Fatalf("RecordedOutputWork = %#v, want mapped work item preserved", result.RecordedOutputWork)
	}
}

func TestSupportedDecisions_MatchesWorkOutcomeVocabulary(t *testing.T) {
	want := []workerexecution.WorkOutcome{
		workerexecution.OutcomeAccepted,
		workerexecution.OutcomeContinue,
		workerexecution.OutcomeRejected,
		workerexecution.OutcomeFailed,
	}
	got := SupportedDecisions()
	if len(got) != len(want) {
		t.Fatalf("SupportedDecisions() = %#v, want %d values", got, len(want))
	}
	for i, outcome := range want {
		if got[i] != string(outcome) {
			t.Fatalf("SupportedDecisions()[%d] = %q, want %q", i, got[i], outcome)
		}
	}
}

func TestOutcomeFromDecision_MapsAcceptedVocabulary(t *testing.T) {
	cases := []struct {
		decision string
		want     workerexecution.WorkOutcome
	}{
		{DecisionAccepted, workerexecution.OutcomeAccepted},
		{DecisionContinue, workerexecution.OutcomeContinue},
		{DecisionRejected, workerexecution.OutcomeRejected},
		{DecisionFailed, workerexecution.OutcomeFailed},
	}
	for _, tc := range cases {
		got, err := OutcomeFromDecision(tc.decision)
		if err != nil {
			t.Fatalf("OutcomeFromDecision(%q): %v", tc.decision, err)
		}
		if got != tc.want {
			t.Fatalf("OutcomeFromDecision(%q) = %q, want %q", tc.decision, got, tc.want)
		}
	}
}

func TestWorkResultFromDecisionEnvelopeJSON_MapsRequiredAndOptionalFields(t *testing.T) {
	raw := `{
		"decision": "ACCEPTED",
		"feedback": "Looks good.",
		"output": "Final summary text.",
		"recorded_output_work": [
			{
				"id": "work-1",
				"workTypeId": "task",
				"state": "complete",
				"traceId": "trace-1"
			}
		]
	}`

	result, err := WorkResultFromDecisionEnvelopeJSON("dispatch-1", "review", raw)
	if err != nil {
		t.Fatalf("WorkResultFromDecisionEnvelopeJSON: %v", err)
	}
	if result.DispatchID != "dispatch-1" || result.TransitionID != "review" {
		t.Fatalf("dispatch metadata = %#v, want dispatch-1/review", result)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Feedback != "Looks good." {
		t.Fatalf("Feedback = %q, want reviewer feedback", result.Feedback)
	}
	if result.Output != "Final summary text." {
		t.Fatalf("Output = %q, want final summary text", result.Output)
	}
	if len(result.RecordedOutputWork) != 1 {
		t.Fatalf("RecordedOutputWork = %#v, want one item", result.RecordedOutputWork)
	}
	if result.RecordedOutputWork[0].ID != "work-1" || result.RecordedOutputWork[0].WorkTypeID != "task" {
		t.Fatalf("RecordedOutputWork[0] = %#v, want mapped work item", result.RecordedOutputWork[0])
	}
}

func TestWorkResultFromDecisionEnvelopeJSON_MapsEachDecisionOutcome(t *testing.T) {
	cases := []struct {
		decision string
		want     workerexecution.WorkOutcome
	}{
		{DecisionAccepted, workerexecution.OutcomeAccepted},
		{DecisionContinue, workerexecution.OutcomeContinue},
		{DecisionRejected, workerexecution.OutcomeRejected},
		{DecisionFailed, workerexecution.OutcomeFailed},
	}
	for _, tc := range cases {
		raw := `{"decision":"` + tc.decision + `","feedback":"notes"}`
		result, err := WorkResultFromDecisionEnvelopeJSON("dispatch-2", "check", raw)
		if err != nil {
			t.Fatalf("WorkResultFromDecisionEnvelopeJSON(%q): %v", tc.decision, err)
		}
		if result.Outcome != tc.want {
			t.Fatalf("decision %q => Outcome %q, want %q", tc.decision, result.Outcome, tc.want)
		}
		if result.Feedback != "notes" {
			t.Fatalf("decision %q => Feedback %q, want notes", tc.decision, result.Feedback)
		}
	}
}

func TestWorkResultFromDecisionEnvelopeJSON_RejectsInvalidJSON(t *testing.T) {
	_, err := WorkResultFromDecisionEnvelopeJSON("dispatch-3", "review", `{decision: broken`)
	if err == nil {
		t.Fatal("WorkResultFromDecisionEnvelopeJSON: error = nil, want invalid JSON error")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("error = %v, want invalid JSON detail", err)
	}
}

func TestWorkResultFromDecisionEnvelopeJSON_RejectsUnknownDecision(t *testing.T) {
	raw := `{"decision":"MAYBE","feedback":"needs another pass"}`
	_, err := WorkResultFromDecisionEnvelopeJSON("dispatch-3", "review", raw)
	if err == nil {
		t.Fatal("WorkResultFromDecisionEnvelopeJSON: error = nil, want unknown decision error")
	}
	if !strings.Contains(err.Error(), `unknown decision "MAYBE"`) {
		t.Fatalf("error = %v, want unknown decision detail", err)
	}
}

func TestWorkResultFromDecisionEnvelopeJSONOrFailed_InvalidJSONUsesFailedOutcome(t *testing.T) {
	result := WorkResultFromDecisionEnvelopeJSONOrFailed("dispatch-4", "review", `not-json`)
	if result.Outcome != MalformedEnvelopeFailureOutcome {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, MalformedEnvelopeFailureOutcome)
	}
	if result.Error == "" {
		t.Fatal("Error is empty, want actionable malformed-envelope text")
	}
	if !strings.Contains(result.Error, "invalid JSON") {
		t.Fatalf("Error = %q, want invalid JSON detail", result.Error)
	}
	if result.Feedback != "" {
		t.Fatalf("Feedback = %q, want empty when JSON did not parse", result.Feedback)
	}
}

func TestWorkResultFromDecisionEnvelopeJSONOrFailed_UnknownDecisionUsesFailedOutcome(t *testing.T) {
	raw := `{"decision":"MAYBE","feedback":"needs another pass"}`
	result := WorkResultFromDecisionEnvelopeJSONOrFailed("dispatch-4", "review", raw)
	if result.Outcome != MalformedEnvelopeFailureOutcome {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, MalformedEnvelopeFailureOutcome)
	}
	if result.Error == "" {
		t.Fatal("Error is empty, want actionable malformed-envelope text")
	}
	if !strings.Contains(result.Error, `unknown decision "MAYBE"`) {
		t.Fatalf("Error = %q, want unknown decision detail", result.Error)
	}
	if result.Feedback != "needs another pass" {
		t.Fatalf("Feedback = %q, want reviewer feedback preserved", result.Feedback)
	}
}

func TestWorkResultFromDecisionEnvelopeJSONOrFailed_MissingDecisionUsesFailedOutcome(t *testing.T) {
	raw := `{"feedback":"missing decision field"}`
	result := WorkResultFromDecisionEnvelopeJSONOrFailed("dispatch-4", "review", raw)
	if result.Outcome != MalformedEnvelopeFailureOutcome {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, MalformedEnvelopeFailureOutcome)
	}
	if !strings.Contains(result.Error, "decision is required") {
		t.Fatalf("Error = %q, want missing decision detail", result.Error)
	}
	if result.Feedback != "missing decision field" {
		t.Fatalf("Feedback = %q, want reviewer feedback preserved", result.Feedback)
	}
}

func TestWorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed_InvalidJSONUsesFailedOutcome(t *testing.T) {
	result := WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed("dispatch-structured", "structured-review-goal", `not-json`)
	if result.Outcome != MalformedEnvelopeFailureOutcome {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, MalformedEnvelopeFailureOutcome)
	}
	if result.Error == "" {
		t.Fatal("Error is empty, want actionable malformed-envelope text")
	}
	if !strings.Contains(result.Error, "invalid JSON") {
		t.Fatalf("Error = %q, want invalid JSON detail", result.Error)
	}
	if result.SelectedClassificationLabel != "" {
		t.Fatalf("SelectedClassificationLabel = %q, want empty on malformed envelope", result.SelectedClassificationLabel)
	}
}

func TestWorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed_UnknownDecisionUsesFailedOutcome(t *testing.T) {
	raw := `{"decision":"MAYBE","feedback":"needs another pass"}`
	result := WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed("dispatch-structured", "structured-review-goal", raw)
	if result.Outcome != MalformedEnvelopeFailureOutcome {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, MalformedEnvelopeFailureOutcome)
	}
	if result.Error == "" {
		t.Fatal("Error is empty, want actionable malformed-envelope text")
	}
	if !strings.Contains(result.Error, `unknown decision "MAYBE"`) {
		t.Fatalf("Error = %q, want unknown decision detail", result.Error)
	}
	if result.Feedback != "needs another pass" {
		t.Fatalf("Feedback = %q, want reviewer feedback preserved", result.Feedback)
	}
	if result.SelectedClassificationLabel != "" {
		t.Fatalf("SelectedClassificationLabel = %q, want empty on unknown decision", result.SelectedClassificationLabel)
	}
}

func TestFailedWorkResultFromDecisionEnvelopeError_MapsToWorkResultFailedOutcome(t *testing.T) {
	parseErr := fmt.Errorf("decision envelope: invalid JSON: unexpected EOF")
	result := FailedWorkResultFromDecisionEnvelopeError("dispatch-5", "check", parseErr, DecisionEnvelope{})
	if result.DispatchID != "dispatch-5" || result.TransitionID != "check" {
		t.Fatalf("dispatch metadata = %#v, want dispatch-5/check", result)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %q, want FAILED", result.Outcome)
	}
	if !strings.Contains(result.Error, "invalid JSON") {
		t.Fatalf("Error = %q, want parse error detail", result.Error)
	}
}
