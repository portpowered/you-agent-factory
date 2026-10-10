package decisionenvelope

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestParseDecisionEnvelopeJSONToleratesStructuredFeedbackAndOutput(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantErr      bool
		wantFeedback string
		wantOutput   string
	}{
		{"string feedback unchanged", `{"decision":"ACCEPTED","feedback":"looks good","output":"a.md"}`, false, "looks good", "a.md"},
		{"object feedback serialised", `{"decision":"ACCEPTED","feedback":{"category": null, "limits":"x"},"output":"a.md"}`, false, `{"category":null,"limits":"x"}`, "a.md"},
		{"array feedback serialised", `{"decision":"ACCEPTED","feedback":[ "a", 1 ]}`, false, `["a",1]`, ""},
		{"object output accepted", `{"decision":"ACCEPTED","feedback":"ok","output":{"files":["a.md"]}}`, false, "ok", `{"files":["a.md"]}`},
		{"null feedback empty", `{"decision":"ACCEPTED","feedback":null}`, false, "", ""},
		{"invalid decision still fails", `{"decision":"MAYBE","feedback":{"a":1}}`, true, "", ""},
		{"missing decision still fails", `{"feedback":{"a":1}}`, true, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := WorkResultFromDecisionEnvelopeJSONOrFailed("d", "t", tc.raw)
			if tc.wantErr {
				if result.Outcome != MalformedEnvelopeFailureOutcome {
					t.Fatalf("outcome = %q, want failed", result.Outcome)
				}
				return
			}
			if result.Outcome != "ACCEPTED" {
				t.Fatalf("outcome = %q (error %q), want ACCEPTED", result.Outcome, result.Error)
			}
			if result.Feedback != tc.wantFeedback || result.Output != tc.wantOutput {
				t.Fatalf("feedback/output = %q/%q, want %q/%q", result.Feedback, result.Output, tc.wantFeedback, tc.wantOutput)
			}
		})
	}
}

func TestPolicy_DecisionEnvelopeContract(t *testing.T) {
	t.Parallel()

	decisionEnvelopes := NewService()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
	}
	if !decisionEnvelopes.UsesDecisionEnvelopeOutcome(workstation) {
		t.Fatal("UsesDecisionEnvelopeOutcome() = false, want true for decision-envelope workstation")
	}

	raw := `{"decision":"ACCEPTED","feedback":"Ship it.","output":"done"}`
	result := decisionEnvelopes.WorkResultFromDecisionEnvelopeJSONOrFailed(
		"dispatch-1",
		"transition-1",
		raw,
	)
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("WorkResultFromDecisionEnvelopeJSONOrFailed() outcome = %q, want %q", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Feedback != "Ship it." {
		t.Fatalf("WorkResultFromDecisionEnvelopeJSONOrFailed() feedback = %q, want %q", result.Feedback, "Ship it.")
	}

	malformed := decisionEnvelopes.WorkResultFromDecisionEnvelopeJSONOrFailed(
		"dispatch-incomplete",
		"transition-incomplete",
		"<COMPLETE>",
	)
	if malformed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("incomplete envelope outcome = %q, want FAILED", malformed.Outcome)
	}
	if malformed.FailureMetadata == nil || malformed.FailureMetadata.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("incomplete envelope failure metadata = %#v, want terminal unknown", malformed.FailureMetadata)
	}
	if malformed.Diagnostics == nil || malformed.Diagnostics.Provider == nil {
		t.Fatalf("incomplete envelope diagnostics = %#v, want structured completion diagnostics", malformed.Diagnostics)
	}
	metadata := malformed.Diagnostics.Provider.ResponseMetadata
	if metadata[workerexecution.ProviderResponseMetadataFailureOperation] != "completion_validation" ||
		metadata[workerexecution.ProviderResponseMetadataFailureClassification] != "missing_required_output" {
		t.Fatalf("incomplete envelope diagnostics = %#v, want bounded completion-validation facts", metadata)
	}
}

func TestPolicy_GoalRoutingDecisionEnvelope(t *testing.T) {
	t.Parallel()

	decisionEnvelopes := NewService()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{
			{Label: "accepted", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "goal", StateName: "complete"}}},
		},
	}
	if !decisionEnvelopes.UsesGoalRoutingDecisionEnvelope(workstation) {
		t.Fatal("UsesGoalRoutingDecisionEnvelope() = false, want true for goal routing workstation")
	}

	acceptedRaw := `{"decision":"accepted","feedback":"Approved.","output":"done"}`
	accepted := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		acceptedRaw,
	)
	if accepted.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("accepted envelope outcome = %q, want %q", accepted.Outcome, workerexecution.OutcomeAccepted)
	}
	if accepted.SelectedClassificationLabel != factorydefinitions.GoalRoutingDecisionAccepted {
		t.Fatalf("accepted envelope label = %q, want %q", accepted.SelectedClassificationLabel, factorydefinitions.GoalRoutingDecisionAccepted)
	}

	needsChangesRaw := `{"decision":"needs-changes","feedback":"Rework required."}`
	needsChanges := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		needsChangesRaw,
	)
	if needsChanges.SelectedClassificationLabel != factorydefinitions.GoalRoutingDecisionNeedsChanges {
		t.Fatalf("needs-changes envelope label = %q, want %q", needsChanges.SelectedClassificationLabel, factorydefinitions.GoalRoutingDecisionNeedsChanges)
	}

	malformed := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		`not-json`,
	)
	if malformed.Outcome != factorydefinitions.MalformedEnvelopeFailureOutcome {
		t.Fatalf("malformed envelope outcome = %q, want %q", malformed.Outcome, factorydefinitions.MalformedEnvelopeFailureOutcome)
	}
	if malformed.Error == "" {
		t.Fatal("malformed envelope error is empty, want actionable failure text")
	}
}
