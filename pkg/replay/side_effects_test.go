package replay

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestSideEffects_InferReturnsRecordedProviderResponse(t *testing.T) {
	artifact := replaySideEffectArtifact(t)
	sideEffects, err := NewSideEffects(artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	resp, err := sideEffects.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch: interfaces.WorkDispatch{
			WorkerType: "worker-a",
			Execution: interfaces.ExecutionMetadata{
				ReplayKey: "process/trace-1/work-1",
				TraceID:   "trace-1",
				WorkIDs:   []string{"work-1"},
			},
		},
		WorkstationType: "process",
		Model:           "claude-3-5-haiku-20241022",
		ModelProvider:   "claude",
		SystemPrompt:    "system prompt",
		UserMessage:     "user prompt",
	})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if resp.Content != "recorded provider output" {
		t.Fatalf("content = %q, want recorded provider output", resp.Content)
	}
	if resp.Diagnostics == nil || resp.Diagnostics.Provider == nil {
		t.Fatal("expected recorded provider diagnostics")
	}
	if resp.Diagnostics.Provider.ResponseMetadata["request_id"] != "req-1" {
		t.Fatalf("response metadata = %#v", resp.Diagnostics.Provider.ResponseMetadata)
	}
}

func TestSideEffects_RunReturnsRecordedCommandResult(t *testing.T) {
	artifact := replaySideEffectArtifact(t)
	sideEffects, err := NewSideEffects(artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	result, err := sideEffects.Run(context.Background(), workers.CommandRequest{
		Command: "echo",
		Args:    []string{"ok"},
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-2/work-2",
			TraceID:   "trace-2",
			WorkIDs:   []string{"work-2"},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(result.Stdout) != "recorded script output\n" {
		t.Fatalf("stdout = %q, want recorded script output", result.Stdout)
	}
	if string(result.Stderr) != "recorded script details\n" {
		t.Fatalf("stderr = %q, want recorded script details", result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", result.ExitCode)
	}
}

func TestSideEffects_UnmatchedRequestFailsClearly(t *testing.T) {
	sideEffects, err := NewSideEffects(replaySideEffectArtifact(t))
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	_, err = sideEffects.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch: interfaces.WorkDispatch{
			WorkerType: "worker-a",
			Execution: interfaces.ExecutionMetadata{
				ReplayKey: "unexpected",
			},
		},
		WorkstationType: "process",
		SystemPrompt:    "system prompt",
		UserMessage:     "user prompt",
	})
	if err == nil {
		t.Fatal("expected unmatched provider request to fail")
	}
	if !strings.Contains(err.Error(), "replay provider request did not match") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSideEffects_DispatchWithoutCompletionFailsExplicitly(t *testing.T) {
	artifact := replaySideEffectArtifact(t)
	artifact.Events = append(artifact.Events, replayDispatchCreatedEvent(t, interfaces.WorkDispatch{
		DispatchID:      "dispatch-no-completion",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-3/work-3",
			TraceID:   "trace-3",
			WorkIDs:   []string{"work-3"},
		},
	}, 6))
	assignEventSequences(artifact.Events)
	sideEffects, err := NewSideEffects(artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	_, err = sideEffects.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch: interfaces.WorkDispatch{
			WorkerType: "worker-a",
			Execution: interfaces.ExecutionMetadata{
				ReplayKey: "process/trace-3/work-3",
				TraceID:   "trace-3",
				WorkIDs:   []string{"work-3"},
			},
		},
		WorkstationType: "process",
		SystemPrompt:    "system prompt",
		UserMessage:     "user prompt",
	})
	if err == nil {
		t.Fatal("expected dispatch without completion to fail")
	}
	if !strings.Contains(err.Error(), "has no completion") {
		t.Fatalf("Infer error = %v, want missing completion diagnostic", err)
	}
}

func replaySideEffectArtifact(t *testing.T) *interfaces.ReplayArtifact {
	t.Helper()
	dispatchProvider := interfaces.WorkDispatch{
		DispatchID:      "dispatch-provider",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-1/work-1",
			TraceID:   "trace-1",
			WorkIDs:   []string{"work-1"},
		},
	}
	dispatchCommand := interfaces.WorkDispatch{
		DispatchID:      "dispatch-command",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-2/work-2",
			TraceID:   "trace-2",
			WorkIDs:   []string{"work-2"},
		},
	}
	providerDiagnostics := &interfaces.WorkDiagnostics{
		Provider: &interfaces.ProviderDiagnostic{
			Provider: "claude",
			Model:    "claude-3-5-haiku-20241022",
			ResponseMetadata: map[string]string{
				"request_id": "req-1",
			},
		},
	}
	commandDiagnostics := &interfaces.WorkDiagnostics{
		Command: &interfaces.CommandDiagnostic{
			Command:  "echo",
			Args:     []string{"ok"},
			Stdout:   "recorded script output\n",
			Stderr:   "recorded script details\n",
			ExitCode: 0,
		},
	}
	return testReplayArtifact(t,
		replayDispatchCreatedEvent(t, dispatchProvider, 2),
		replayDispatchCreatedEvent(t, dispatchCommand, 3),
		replayInferenceResponseEvent(
			t,
			dispatchProvider,
			"dispatch-provider/inference-request/1",
			1,
			4,
			"recorded provider output",
			nil,
			providerDiagnostics,
			"",
		),
		replayDispatchCompletedEvent(t, "completion-provider", interfaces.WorkResult{
			DispatchID:   "dispatch-provider",
			TransitionID: "process",
			Outcome:      interfaces.OutcomeAccepted,
			Output:       "recorded provider output",
			Diagnostics:  providerDiagnostics,
		}, 4),
		replayDispatchCompletedEvent(t, "completion-command", interfaces.WorkResult{
			DispatchID:   "dispatch-command",
			TransitionID: "process",
			Outcome:      interfaces.OutcomeAccepted,
			Output:       "recorded script output\n",
			Error:        "recorded script details\n",
			Diagnostics:  commandDiagnostics,
		}, 5),
	)
}
