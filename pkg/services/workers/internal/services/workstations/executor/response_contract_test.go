package executor

import (
	"strings"
	"testing"
)

const validObservationReport = `## Inspection status
Inspected: yes

## Chronological events
- 00:00.000 — The subject enters frame.
- 00:02.000 — The subject turns toward the light.

## Temporal or transient defects
None observed.

## Audio content and defects
Audio content: noise
None observed.

## Observed speech
None observed.

## Overall recommendation
Recommendation: pass
`

func TestValidateMarkdownObservationReportAcceptsCompleteReport(t *testing.T) {
	if err := validateOutputContract(validObservationReport, outputContractMarkdownObservationReportV1); err != nil {
		t.Fatalf("validateOutputContract() error = %v, want complete report accepted", err)
	}
}

func TestValidateMarkdownObservationReportRejectsIncompleteAndRefusalOutput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "prose without required sections",
			content: "The clip contains a woman and a ticking clock. The audio is noise.",
			want:    `missing required section "inspection status"`,
		},
		{
			name:    "provider refusal",
			content: "I could not inspect the file because it does not exist. Recommendation: pass",
			want:    `missing required section "inspection status"`,
		},
		{
			name:    "missing recommendation",
			content: strings.Replace(validObservationReport, "Recommendation: pass", "No recommendation available", 1),
			want:    "exactly one pass or reroll recommendation",
		},
		{
			name:    "duplicate recommendation",
			content: strings.Replace(validObservationReport, "Recommendation: pass", "Recommendation: pass\nRecommendation: reroll", 1),
			want:    "exactly one pass or reroll recommendation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOutputContract(test.content, outputContractMarkdownObservationReportV1)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateOutputContract() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
