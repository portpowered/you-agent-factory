package workers_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCloneProviderInferenceRequestDeeplyDetachesInputTokens(t *testing.T) {
	t.Parallel()

	source := workerexecution.ProviderInferenceRequest{
		InputTokens: []any{
			[]any{
				map[string]any{
					"strings": []string{"alpha"},
					"bytes":   []byte("beta"),
					"labels":  map[string]string{"kind": "original"},
					"groups":  map[string][]string{"items": {"first"}},
					"scalar":  "unchanged",
				},
			},
		},
	}

	cloned := workerexecution.CloneProviderInferenceRequest(source)
	clonedMap := cloned.InputTokens[0].([]any)[0].(map[string]any)
	clonedMap["strings"].([]string)[0] = "changed"
	clonedMap["bytes"].([]byte)[0] = 'X'
	clonedMap["labels"].(map[string]string)["kind"] = "changed"
	clonedMap["groups"].(map[string][]string)["items"][0] = "changed"
	clonedMap["scalar"] = "changed"

	sourceMap := source.InputTokens[0].([]any)[0].(map[string]any)
	if got := sourceMap["strings"].([]string)[0]; got != "alpha" {
		t.Fatalf("source strings = %q, want detached original", got)
	}
	if got := string(sourceMap["bytes"].([]byte)); got != "beta" {
		t.Fatalf("source bytes = %q, want detached original", got)
	}
	if got := sourceMap["labels"].(map[string]string)["kind"]; got != "original" {
		t.Fatalf("source label = %q, want detached original", got)
	}
	if got := sourceMap["groups"].(map[string][]string)["items"][0]; got != "first" {
		t.Fatalf("source group = %q, want detached original", got)
	}
	if got := sourceMap["scalar"]; got != "unchanged" {
		t.Fatalf("source scalar = %#v, want original", got)
	}
}

func TestRequestClonesNormalizeEmptyInputTokensToNil(t *testing.T) {
	t.Parallel()

	if got := workerexecution.CloneWorkstationExecutionRequest(workerexecution.WorkstationExecutionRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("workstation input tokens = %#v, want nil", got)
	}
	if got := workerexecution.CloneProviderInferenceRequest(workerexecution.ProviderInferenceRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("provider input tokens = %#v, want nil", got)
	}
	if got := workerexecution.CloneSubprocessExecutionRequest(workerexecution.SubprocessExecutionRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("subprocess input tokens = %#v, want nil", got)
	}
}

func TestCloneProviderInferenceRequestPreservesScalarInputTokenValues(t *testing.T) {
	t.Parallel()

	source := workerexecution.ProviderInferenceRequest{
		InputTokens: []any{"text", float64(3), true, nil},
	}
	cloned := workerexecution.CloneProviderInferenceRequest(source)

	if !reflect.DeepEqual(cloned.InputTokens, source.InputTokens) {
		t.Fatalf("cloned input tokens = %#v, want %#v", cloned.InputTokens, source.InputTokens)
	}
}

func TestExecuteRequestValidateRequiresCorrelationAndTarget(t *testing.T) {
	t.Parallel()

	err := (workerexecution.ExecuteRequest{}).Validate()
	if !errors.Is(err, workerexecution.ErrInvalidExecuteRequest) {
		t.Fatalf("Validate() error = %v, want ErrInvalidExecuteRequest", err)
	}

	valid := workerexecution.ExecuteRequest{
		Correlation: workerexecution.ExecutionCorrelation{
			DispatchID: "dispatch-1",
			AttemptID:  "attempt-1",
		},
		Target: workerexecution.ExecutionTarget{RunnerID: workerexecution.RunnerIDCodex},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestExecuteRequestCloneDetachesNestedMutableValues(t *testing.T) {
	t.Parallel()

	original := workerexecution.ExecuteRequest{
		Correlation: workerexecution.ExecutionCorrelation{
			DispatchID: "dispatch-1",
			AttemptID:  "attempt-1",
		},
		Target: workerexecution.ExecutionTarget{
			RunnerID: workerexecution.RunnerIDCodex,
			Environment: workerexecution.EnvironmentPolicy{
				Vars:               map[string]string{"SECRET": "value"},
				ProcessEnvironment: []string{"TOKEN=raw"},
			},
			Tools: workerexecution.ToolPolicy{
				RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
					workerexecution.RunnerOptionalCapabilityWorkingDirectory,
				},
			},
		},
		Input: workerexecution.ExecutionInput{
			Work: []workerexecution.WorkInput{{
				WorkID: "work-1",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "payload",
				}},
				Tags: map[string]string{"k": "v"},
			}},
			Resume: &workerexecution.ProviderContinuationRef{
				Provider:          "codex",
				ProviderSessionID: "session-1",
			},
		},
	}

	clone := original.Clone()
	clone.Target.Environment.Vars["SECRET"] = "mutated"
	clone.Target.Environment.ProcessEnvironment[0] = "mutated"
	clone.Target.Tools.RequiredOptionalCapabilities[0] = workerexecution.RunnerOptionalCapabilityWorktree
	clone.Input.Work[0].Content[0].Text = "mutated"
	clone.Input.Work[0].Tags["k"] = "mutated"
	clone.Input.Resume.ProviderSessionID = "mutated"

	if original.Target.Environment.Vars["SECRET"] != "value" {
		t.Fatalf("original env mutated: %#v", original.Target.Environment.Vars)
	}
	if original.Target.Environment.ProcessEnvironment[0] != "TOKEN=raw" {
		t.Fatalf("original process environment mutated: %#v", original.Target.Environment.ProcessEnvironment)
	}
	if original.Target.Tools.RequiredOptionalCapabilities[0] !=
		workerexecution.RunnerOptionalCapabilityWorkingDirectory {
		t.Fatalf("original capabilities mutated: %#v", original.Target.Tools.RequiredOptionalCapabilities)
	}
	if original.Input.Work[0].Content[0].Text != "payload" {
		t.Fatalf("original work content mutated: %#v", original.Input.Work[0].Content)
	}
	if original.Input.Work[0].Tags["k"] != "v" {
		t.Fatalf("original work tags mutated: %#v", original.Input.Work[0].Tags)
	}
	if original.Input.Resume.ProviderSessionID != "session-1" {
		t.Fatalf("original resume mutated: %#v", original.Input.Resume)
	}
}

func TestExecuteResultCloneDetachesOutputAndDiagnostics(t *testing.T) {
	t.Parallel()

	original := workerexecution.ExecuteResult{
		Correlation: workerexecution.ExecutionCorrelation{
			DispatchID: "dispatch-1",
			AttemptID:  "attempt-1",
		},
		Outcome: workerexecution.ExecutionOutcomeAccepted,
		ArtifactVerification: &workerexecution.ExpectedArtifactVerification{
			Code: workerexecution.WorkFailureTypeExpectedArtifactsUnsatisfied,
			Entries: []workerexecution.ExpectedArtifactVerificationEntry{{
				Name:    "summary",
				Pattern: "reports/summary.md",
				Reason:  workerexecution.ExpectedArtifactVerificationReasonMissing,
			}},
		},
		Output: workerexecution.ProposedOutput{
			Primary: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "output",
			}},
			ProposedWork: []workerexecution.ProposedWork{{
				WorkTypeID: "follow-up",
				Tags:       map[string]string{"kind": "next"},
			}},
		},
		StructuredResult: map[string]any{
			"nested": []any{"original"},
		},
		StructuredResultPresent: true,
		Diagnostics: &workerexecution.SafeDiagnostics{
			Command: &workerexecution.SafeCommandDiagnostic{
				Command: "runner",
				Args:    []string{"--safe"},
			},
			Metadata: map[string]string{"duration_ms": "12"},
		},
		Continuation: &workerexecution.ProviderContinuationRef{
			Provider:          "codex",
			ProviderSessionID: "session-1",
		},
	}

	clone := original.Clone()
	clone.Output.Primary[0].Text = "mutated"
	clone.Output.ProposedWork[0].Tags["kind"] = "mutated"
	clone.StructuredResult.(map[string]any)["nested"].([]any)[0] = "mutated"
	clone.Diagnostics.Command.Args[0] = "--mutated"
	clone.Diagnostics.Metadata["duration_ms"] = "99"
	clone.Continuation.ProviderSessionID = "mutated"
	clone.ArtifactVerification.Entries[0].Name = "mutated"

	if original.Output.Primary[0].Text != "output" {
		t.Fatalf("original output mutated: %#v", original.Output.Primary)
	}
	if original.Output.ProposedWork[0].Tags["kind"] != "next" {
		t.Fatalf("original proposed work mutated: %#v", original.Output.ProposedWork)
	}
	if original.StructuredResult.(map[string]any)["nested"].([]any)[0] != "original" ||
		!original.StructuredResultPresent {
		t.Fatalf("original structured result mutated: %#v (present=%t)", original.StructuredResult, original.StructuredResultPresent)
	}
	if original.Diagnostics.Command.Args[0] != "--safe" {
		t.Fatalf("original diagnostics args mutated: %#v", original.Diagnostics.Command.Args)
	}
	if original.Diagnostics.Metadata["duration_ms"] != "12" {
		t.Fatalf("original diagnostics metadata mutated: %#v", original.Diagnostics.Metadata)
	}
	if original.Continuation.ProviderSessionID != "session-1" {
		t.Fatalf("original continuation mutated: %#v", original.Continuation)
	}
	if original.ArtifactVerification.Entries[0].Name != "summary" {
		t.Fatalf("original artifact verification mutated: %#v", original.ArtifactVerification)
	}
	nullClone := (workerexecution.ExecuteResult{
		StructuredResultPresent: true,
	}).Clone()
	if !nullClone.StructuredResultPresent || nullClone.StructuredResult != nil {
		t.Fatalf("explicit null structured result = %#v (present=%t), want nil/present", nullClone.StructuredResult, nullClone.StructuredResultPresent)
	}
}
