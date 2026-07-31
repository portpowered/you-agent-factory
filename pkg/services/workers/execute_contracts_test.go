package workers_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestExecuteRequestValidateRequiresCorrelationAndTarget(t *testing.T) {
	t.Parallel()

	err := workers.ExecuteRequest{}.Validate()
	if !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("Validate() error = %v, want ErrInvalidExecuteRequest", err)
	}

	valid := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-1",
			AttemptID:  "attempt-1",
		},
		Target: workers.ExecutionTarget{RunnerID: workers.RunnerIDCodex},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestExecuteRequestCloneDetachesMutableCollections(t *testing.T) {
	t.Parallel()

	original := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-1",
			AttemptID:  "attempt-1",
		},
		Target: workers.ExecutionTarget{
			RunnerID: "agent",
			Environment: workers.EnvironmentPolicy{
				Vars:               map[string]string{"SECRET": "value"},
				ProcessEnvironment: []string{"TOKEN=raw"},
			},
			Prompt: workers.PromptPolicy{
				SystemPrompt: "system secret",
				UserMessage:  "user secret",
			},
		},
		Input: workers.ExecutionInput{
			Work: []workers.WorkInput{{
				WorkID:  "work-1",
				Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "payload"}},
				Tags:    map[string]string{"k": "v"},
			}},
			Resume: &workers.ProviderContinuationRef{
				Provider:          "codex",
				ProviderSessionID: "session-1",
			},
		},
	}
	clone := original.Clone()
	clone.Target.Environment.Vars["SECRET"] = "mutated"
	clone.Target.Environment.ProcessEnvironment[0] = "mutated"
	clone.Input.Work[0].Tags["k"] = "mutated"
	clone.Input.Resume.ProviderSessionID = "mutated"

	if original.Target.Environment.Vars["SECRET"] != "value" {
		t.Fatalf("original env mutated: %#v", original.Target.Environment.Vars)
	}
	if original.Target.Environment.ProcessEnvironment[0] != "TOKEN=raw" {
		t.Fatalf("original process env mutated: %#v", original.Target.Environment.ProcessEnvironment)
	}
	if original.Input.Work[0].Tags["k"] != "v" {
		t.Fatalf("original work tags mutated: %#v", original.Input.Work[0].Tags)
	}
	if original.Input.Resume.ProviderSessionID != "session-1" {
		t.Fatalf("original resume mutated: %#v", original.Input.Resume)
	}
}

func TestCloneSafeDiagnosticsOmitsCommandStdinAndEnv(t *testing.T) {
	t.Parallel()

	diagnostics := &workers.SafeDiagnostics{
		Command: &workers.SafeCommandDiagnostic{
			Command: "tool",
			Args:    []string{"--flag"},
			Stdout:  "ok",
		},
		Metadata: map[string]string{"duration_ms": "12"},
	}
	clone := workers.CloneSafeDiagnostics(diagnostics)
	clone.Command.Args[0] = "--mutated"
	clone.Metadata["duration_ms"] = "99"
	if diagnostics.Command.Args[0] != "--flag" {
		t.Fatalf("original command args mutated: %#v", diagnostics.Command.Args)
	}
	if diagnostics.Metadata["duration_ms"] != "12" {
		t.Fatalf("original metadata mutated: %#v", diagnostics.Metadata)
	}
}
