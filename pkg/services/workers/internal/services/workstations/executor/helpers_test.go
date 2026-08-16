package executor

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCompletionValidationFailureUsesProviderEvidenceSafely(t *testing.T) {
	for _, test := range []struct {
		name     string
		response workerexecution.InferenceResponse
		want     string
		kind     string
	}{
		{
			name: "missing",
			want: "provider completion evidence was missing",
			kind: "missing_completion_evidence",
		},
		{
			name: "content without evidence",
			response: workerexecution.InferenceResponse{
				Content: "answer",
			},
			want: "provider completion evidence was incomplete",
			kind: "missing_completion_evidence",
		},
		{
			name: "top-level evidence",
			response: workerexecution.InferenceResponse{
				Content: "answer",
				Diagnostics: &workerexecution.WorkDiagnostics{Metadata: map[string]string{
					workerexecution.ProviderResponseMetadataCompletionEvidence: " task_complete ",
				}},
			},
			want: "provider completion evidence was contradictory",
			kind: "contradictory_completion",
		},
		{
			name: "provider evidence",
			response: workerexecution.InferenceResponse{
				Diagnostics: &workerexecution.WorkDiagnostics{Provider: &workerexecution.ProviderDiagnostic{
					ResponseMetadata: map[string]string{
						workerexecution.ProviderResponseMetadataCompletionEvidence: "turn_completed",
					},
				}},
			},
			want: "provider completion evidence was contradictory",
			kind: "contradictory_completion",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			message, kind := CompletionValidationFailure(test.response)
			if message != test.want || kind != test.kind {
				t.Fatalf("CompletionValidationFailure() = (%q, %q), want (%q, %q)", message, kind, test.want, test.kind)
			}
		})
	}
}

func TestOutputSchemaValidationCoversParsingAndBoundedDiagnostics(t *testing.T) {
	schema := `{"type":"object","required":["name"],"properties":{"name":{"type":"string","pattern":"^[a-z]+$"}}}`
	if value, err := ParseOutputAgainstSchema(`{"name":"alice"}`, schema); err != nil || value == nil {
		t.Fatalf("ParseOutputAgainstSchema(valid) = %#v, %v", value, err)
	}

	for _, content := range []string{"", "not-json", `{"name":1}`, `{"other":"value"}`} {
		if _, err := ParseOutputAgainstSchema(content, schema); err == nil {
			t.Fatalf("ParseOutputAgainstSchema(%q) error = nil, want validation failure", content)
		}
	}
	for _, invalidSchema := range []string{"", "not-json", `{"type":"not-a-json-type"}`} {
		if _, err := ParseOutputAgainstSchema(`{"name":"alice"}`, invalidSchema); err == nil {
			t.Fatalf("ParseOutputAgainstSchema(schema %q) error = nil, want schema failure", invalidSchema)
		}
	}

	if got := structuredOutputInstanceLocation(nil); got != "$" {
		t.Fatalf("structuredOutputInstanceLocation(nil) = %q, want $", got)
	}
	if got := structuredOutputInstanceLocation([]string{"name", "a/b", "c~d"}); got != "/name/a~1b/c~0d" {
		t.Fatalf("structuredOutputInstanceLocation() = %q, want escaped JSON pointer", got)
	}
	if got := structuredOutputValidationSummary(nil); got != "schema validation failed" {
		t.Fatalf("structuredOutputValidationSummary(nil) = %q", got)
	}
	if got := structuredOutputValidationSummary(errors.New("raw rejected value")); got != "schema validation failed" {
		t.Fatalf("structuredOutputValidationSummary(non-schema) = %q", got)
	}
	if got := boundedStructuredOutputDiagnostic(strings.Repeat("x", structuredOutputValidationDetailLimit+10)); !strings.HasSuffix(got, "...") || len([]rune(got)) != structuredOutputValidationDetailLimit {
		t.Fatalf("boundedStructuredOutputDiagnostic() = %q, want bounded diagnostic", got)
	}
	if got := boundedStructuredOutputDiagnostic(" "); got != "schema validation failed" {
		t.Fatalf("boundedStructuredOutputDiagnostic(empty) = %q", got)
	}
}

func TestStructuredResultAttachmentAndConfigurationFailures(t *testing.T) {
	request := workerexecution.WorkstationExecutionRequest{
		Dispatch:     workerDispatchFixture(),
		OutputSchema: `{"type":"object","required":["answer"]}`,
	}
	valid := attachStructuredResult(request, workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		Output:  `{"answer":"ok"}`,
	})
	if !valid.StructuredResultPresent || valid.StructuredResult == nil {
		t.Fatalf("attachStructuredResult(valid) = %#v, want structured result", valid)
	}

	invalid := attachStructuredResult(request, workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		Output:  `{"wrong":"value"}`,
	})
	if invalid.Outcome != workerexecution.OutcomeFailed || invalid.FailureMetadata == nil ||
		invalid.FailureMetadata.Type != workerexecution.WorkFailureTypeStructuredOutputSchemaViolation {
		t.Fatalf("attachStructuredResult(invalid) = %#v, want schema violation", invalid)
	}

	unchanged := workerexecution.WorkResult{Outcome: workerexecution.OutcomeRejected, Output: "raw"}
	if got := attachStructuredResult(request, unchanged); !reflect.DeepEqual(got, unchanged) {
		t.Fatalf("attachStructuredResult(rejected) = %#v, want unchanged result", got)
	}
	withStructured := workerexecution.WorkResult{
		Outcome:                 workerexecution.OutcomeAccepted,
		Output:                  "raw",
		StructuredResult:        map[string]any{"answer": "already decoded"},
		StructuredResultPresent: true,
	}
	if got := attachStructuredResult(request, withStructured); !reflect.DeepEqual(got.StructuredResult, withStructured.StructuredResult) {
		t.Fatalf("attachStructuredResult(existing structured) changed result: %#v", got)
	}

	failure := outputSchemaConfigurationFailure(request, errors.New("schema is invalid"))
	if failure.Outcome != workerexecution.OutcomeFailed || failure.FailureMetadata == nil ||
		failure.FailureMetadata.Type != workerexecution.WorkFailureTypeMisconfigured ||
		!strings.Contains(failure.Error, "schema is invalid") {
		t.Fatalf("outputSchemaConfigurationFailure() = %#v", failure)
	}
	if validateOutputSchema([]byte(`{"type":"object"}`)) != nil {
		t.Fatal("validateOutputSchema(valid) returned an error")
	}
	if validateOutputSchema(nil) == nil {
		t.Fatal("validateOutputSchema(empty) error = nil")
	}
}

func TestOutputContractsValidateSemanticResults(t *testing.T) {
	markdown := strings.Join([]string{
		"## Inspection Status", "Inspected: yes",
		"## Chronological Events", "At 0.0s the clip begins.",
		"## Temporal or Transient Defects", "none observed",
		"## Audio Content and Defects", "Audio content: silence\nnone observed",
		"## Observed Speech", "none observed",
		"## Overall Recommendation", "Recommendation: pass",
	}, "\n")
	if err := ValidateOutputContract(markdown, outputContractMarkdownObservationReportV1); err != nil {
		t.Fatalf("ValidateOutputContract(valid markdown) = %v", err)
	}
	if err := ValidateOutputContract(`{"action_completed":true}`, ""); err != nil {
		t.Fatalf("ValidateOutputContract(empty contract) = %v", err)
	}
	if err := ValidateOutputContract("answer", "unsupported"); err == nil {
		t.Fatal("ValidateOutputContract(unsupported) error = nil")
	}

	validPass := `{"action_completed":true,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"silence","unexpected_speech":false,"verdict":"pass","confidence":0.9}`
	if err := ValidateOutputContract(validPass, outputContractStructuredClipQAVerdictV1); err != nil {
		t.Fatalf("ValidateOutputContract(valid pass) = %v", err)
	}
	validReroll := `{"action_completed":false,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"speech","unexpected_speech":true,"verdict":"reroll","confidence":0.4}`
	if err := ValidateOutputContract(validReroll, outputContractStructuredClipQAVerdictV1); err != nil {
		t.Fatalf("ValidateOutputContract(valid reroll) = %v", err)
	}

	for _, content := range []string{
		"not-json",
		`{"action_completed":true}`,
		`{"action_completed":null,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"silence","unexpected_speech":false,"verdict":"pass","confidence":0.9}`,
		`{"action_completed":true,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"silence","unexpected_speech":false,"verdict":"pass","confidence":2}`,
		`{"action_completed":false,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"silence","unexpected_speech":false,"verdict":"pass","confidence":0.9}`,
		`{"action_completed":true,"spec_deviations":["bad"],"temporal_artifacts":[],"audio_content":"silence","unexpected_speech":false,"verdict":"pass","confidence":0.9}`,
		`{"action_completed":true,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"silence","unexpected_speech":false,"verdict":"reroll","confidence":0.9}`,
		`{"action_completed":true,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"silence","unexpected_speech":false,"verdict":"unknown","confidence":0.9}`,
	} {
		if err := ValidateOutputContract(content, outputContractStructuredClipQAVerdictV1); err == nil {
			t.Fatalf("ValidateOutputContract(%s) error = nil", content)
		}
	}

	for _, content := range []string{"", "## Inspection Status\nno", markdown + "\nRecommendation: reroll"} {
		if err := ValidateOutputContract(content, outputContractMarkdownObservationReportV1); err == nil {
			t.Fatalf("ValidateOutputContract(invalid markdown) error = nil for %q", content)
		}
	}

	for _, test := range []struct {
		content  string
		contract string
		want     string
	}{
		{content: `{"verdict":"failed"}`, want: "verdict=failed"},
		{content: `{"success":false}`, want: "success=false"},
		{content: `{"action_completed":false}`, contract: "other", want: "action_completed=false"},
		{content: `{"verdict":"reroll"}`, contract: "other", want: "verdict=reroll without an explicit structured QA output contract"},
	} {
		if got := structuredOutputFailure(test.content, test.contract); got != test.want {
			t.Fatalf("structuredOutputFailure(%s, %q) = %q, want %q", test.content, test.contract, got, test.want)
		}
	}
	if got := structuredOutputFailure(`{"verdict":"pass","success":true}`, outputContractStructuredClipQAVerdictV1); got != "" {
		t.Fatalf("structuredOutputFailure(success) = %q, want empty", got)
	}
}

func TestWorkstationExecutorRequestHelpersAndNoop(t *testing.T) {
	if got := cloneEnvVars(nil); got != nil {
		t.Fatalf("cloneEnvVars(nil) = %#v, want nil", got)
	}
	if got := cloneEnvVars(map[string]string{"A": "B"}); got["A"] != "B" {
		t.Fatalf("cloneEnvVars() = %#v", got)
	}
	if got := cloneContinuation(nil); got != nil {
		t.Fatalf("cloneContinuation(nil) = %#v, want nil", got)
	}
	if _, err := (&NoopExecutor{}).Execute(context.Background(), workerDispatchFixture()); err != nil {
		t.Fatalf("NoopExecutor.Execute() error = %v", err)
	}
}

func workerDispatchFixture() work.WorkDispatch {
	return work.WorkDispatch{DispatchID: "dispatch-1", TransitionID: "transition-1"}
}
