package goal

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSupportedDecisions_MatchesWorkOutcomeVocabulary(t *testing.T) {
	want := []interfaces.WorkOutcome{
		interfaces.OutcomeAccepted,
		interfaces.OutcomeContinue,
		interfaces.OutcomeRejected,
		interfaces.OutcomeFailed,
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
		want     interfaces.WorkOutcome
	}{
		{DecisionAccepted, interfaces.OutcomeAccepted},
		{DecisionContinue, interfaces.OutcomeContinue},
		{DecisionRejected, interfaces.OutcomeRejected},
		{DecisionFailed, interfaces.OutcomeFailed},
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
	if result.Outcome != interfaces.OutcomeAccepted {
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
		want     interfaces.WorkOutcome
	}{
		{DecisionAccepted, interfaces.OutcomeAccepted},
		{DecisionContinue, interfaces.OutcomeContinue},
		{DecisionRejected, interfaces.OutcomeRejected},
		{DecisionFailed, interfaces.OutcomeFailed},
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
