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

const validClipQAVerdict = `{"action_completed":true,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"noise","unexpected_speech":false,"verdict":"pass","confidence":0.95}`

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

func TestValidateStructuredClipQAVerdictEnforcesBoundsAndPassInvariants(t *testing.T) {
	if err := validateOutputContract(validClipQAVerdict, outputContractStructuredClipQAVerdictV1); err != nil {
		t.Fatalf("validateOutputContract() error = %v, want valid verdict accepted", err)
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "confidence below zero",
			content: strings.Replace(validClipQAVerdict, `"confidence":0.95`, `"confidence":-0.01`, 1),
			want:    "between 0 and 1",
		},
		{
			name:    "confidence above one",
			content: strings.Replace(validClipQAVerdict, `"confidence":0.95`, `"confidence":1.01`, 1),
			want:    "between 0 and 1",
		},
		{
			name:    "pass with incomplete action",
			content: strings.Replace(validClipQAVerdict, `"action_completed":true`, `"action_completed":false`, 1),
			want:    "action_completed=true",
		},
		{
			name:    "pass with specification deviation",
			content: strings.Replace(validClipQAVerdict, `"spec_deviations":[]`, `"spec_deviations":["wrong action"]`, 1),
			want:    "spec_deviations to be empty",
		},
		{
			name:    "pass with temporal artifact",
			content: strings.Replace(validClipQAVerdict, `"temporal_artifacts":[]`, `"temporal_artifacts":["flash"]`, 1),
			want:    "temporal_artifacts to be empty",
		},
		{
			name:    "pass with unexpected speech",
			content: strings.Replace(validClipQAVerdict, `"unexpected_speech":false`, `"unexpected_speech":true`, 1),
			want:    "unexpected_speech=false",
		},
		{
			name:    "reroll without reason",
			content: strings.Replace(validClipQAVerdict, `"verdict":"pass"`, `"verdict":"reroll"`, 1),
			want:    "observed failure reason",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOutputContract(test.content, outputContractStructuredClipQAVerdictV1)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateOutputContract() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateStructuredClipQAVerdictAcceptsInspectedReroll(t *testing.T) {
	content := strings.Replace(validClipQAVerdict, `"action_completed":true`, `"action_completed":false`, 1)
	content = strings.Replace(content, `"verdict":"pass"`, `"verdict":"reroll"`, 1)
	if err := validateOutputContract(content, outputContractStructuredClipQAVerdictV1); err != nil {
		t.Fatalf("validateOutputContract() error = %v, want inspected reroll accepted", err)
	}
}
