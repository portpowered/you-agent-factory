package decisionenvelope

import "testing"

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
