package replay

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestReplayModelInvocationFailureDetailAvoidsNestedWorkerClassification(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "worker scoped failure",
			input: `inference failed for worker "tts-executor" model "tts" operation "TTS": model inference failed: backend unavailable`,
			want:  "model inference failed: backend unavailable",
		},
		{
			name:  "model scoped failure",
			input: `inference failed for model "tts" operation "TTS": model inference failed: backend unavailable`,
			want:  "model inference failed: backend unavailable",
		},
		{
			name:  "already detached failure",
			input: `model inference failed: backend unavailable`,
			want:  "model inference failed: backend unavailable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := replayModelInvocationFailureDetail(test.input); got != test.want {
				t.Fatalf("replayModelInvocationFailureDetail(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

// FND-12 captured replay success baseline: matched recorded provider
// inference returns the recorded response. Invoked by
// `make fnd-12-replay-behavior-baselines`.
func TestSideEffects_InferReturnsRecordedProviderResponse(t *testing.T) {
	artifact := replaySideEffectArtifact(t)
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	resp, err := sideEffects.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			WorkerType: "worker-a",
			Execution: work.ExecutionMetadata{
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
	if resp.Diagnostics.Command != nil || resp.Diagnostics.Panic != nil ||
		len(resp.Diagnostics.Metadata) != 1 ||
		resp.Diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] != "provider_response" {
		t.Fatalf("response diagnostics leaked unsafe fields: %#v", resp.Diagnostics)
	}
}

func TestSideEffects_RunReturnsRecordedCommandResult(t *testing.T) {
	artifact := replaySideEffectArtifact(t)
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	result, err := sideEffects.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo",
		Args:    []string{"ok"},
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

func TestSideEffects_CommandMismatchDoesNotConsumeRecordedResult(t *testing.T) {
	sideEffects := &SideEffects{records: []sideEffectRecord{{
		dispatch: replayDispatch{dispatchID: "dispatch-command", dispatch: work.WorkDispatch{}},
		completion: &replayCompletion{
			result: workerexecution.WorkResult{Output: "recorded output"},
			diagnostics: &workerexecution.WorkDiagnostics{Command: &workerexecution.CommandDiagnostic{
				Command: "echo", Args: []string{"ok"}, Stdout: "recorded output",
			}},
		},
		hasCompletion: true,
	}}}

	if _, err := sideEffects.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo",
		Args:    []string{"unexpected"},
	}); err == nil {
		t.Fatal("Run(mismatched command) = nil, want replay divergence")
	}

	result, err := sideEffects.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo",
		Args:    []string{"ok"},
	})
	if err != nil {
		t.Fatalf("Run(after mismatch): %v", err)
	}
	if string(result.Stdout) != "recorded output" {
		t.Fatalf("stdout after mismatch = %q, want retained recorded result", result.Stdout)
	}
}

func TestSideEffects_InferDiagnosticsStayDetachedFromRecordedMutation(t *testing.T) {
	artifact, providerDiagnostics, _ := replaySideEffectArtifactWithDiagnostics(t)
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	providerDiagnostics.Provider.ResponseMetadata["request_id"] = "mutated-request"
	providerDiagnostics.Metadata["phase"] = "mutated"
	providerDiagnostics.Panic.Message = "mutated panic"

	resp, err := sideEffects.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			WorkerType: "worker-a",
			Execution: work.ExecutionMetadata{
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

	if got := resp.Diagnostics.Provider.ResponseMetadata["request_id"]; got != "req-1" {
		t.Fatalf("request_id = %q, want req-1", got)
	}
	if len(resp.Diagnostics.Metadata) != 1 ||
		resp.Diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] != "provider_response" {
		t.Fatalf("metadata = %#v, want only safe completion evidence", resp.Diagnostics.Metadata)
	}
	if resp.Diagnostics.Panic != nil {
		t.Fatalf("panic = %#v, want nil safe replay panic", resp.Diagnostics.Panic)
	}
}

func TestSideEffects_RunResultStaysDetachedFromRecordedCommandMutation(t *testing.T) {
	artifact, _, commandDiagnostics := replaySideEffectArtifactWithDiagnostics(t)
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	commandDiagnostics.Command.Args[0] = "mutated"
	commandDiagnostics.Command.Stdout = "mutated stdout\n"
	commandDiagnostics.Command.Stderr = "mutated stderr\n"

	result, err := sideEffects.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo",
		Args:    []string{"ok"},
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
}

// FND-12 captured replay typed-failure baseline: unmatched replay key fails
// with a stable visible error. Invoked by `make fnd-12-replay-behavior-baselines`.
func TestSideEffects_UnmatchedRequestFailsClearly(t *testing.T) {
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, replaySideEffectArtifact(t))
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	_, err = sideEffects.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			WorkerType: "worker-a",
			Execution: work.ExecutionMetadata{
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

func TestSideEffects_InferProvidesCompletionEvidenceWhenReplayArtifactOmitsDiagnostics(t *testing.T) {
	artifact := testReplayArtifact(
		t,
		replayDispatchCreatedEvent(t, work.WorkDispatch{
			DispatchID:      "dispatch-no-diagnostics",
			TransitionID:    "process",
			WorkerType:      "worker-a",
			WorkstationName: "process",
			Execution: work.ExecutionMetadata{
				ReplayKey: "process/trace-3/work-3",
				TraceID:   "trace-3",
				WorkIDs:   []string{"work-3"},
			},
		}, 2),
		replayInferenceResponseEvent(
			t,
			work.WorkDispatch{
				DispatchID: "dispatch-no-diagnostics",
				Execution: work.ExecutionMetadata{
					ReplayKey: "process/trace-3/work-3",
					TraceID:   "trace-3",
					WorkIDs:   []string{"work-3"},
				},
			},
			"dispatch-no-diagnostics/inference-request/1",
			1,
			3,
			"recorded provider output",
			&providers.SessionMetadata{
				Provider: "claude",
				Kind:     "response_id",
				ID:       "resp-no-diagnostics",
			},
			nil,
			"",
		),
		replayDispatchCompletedEvent(t, "completion-no-diagnostics", workerexecution.WorkResult{
			DispatchID:   "dispatch-no-diagnostics",
			TransitionID: "process",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "recorded provider output",
		}, 4),
	)
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	resp, err := sideEffects.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			WorkerType: "worker-a",
			Execution: work.ExecutionMetadata{
				ReplayKey: "process/trace-3/work-3",
				TraceID:   "trace-3",
				WorkIDs:   []string{"work-3"},
			},
		},
		WorkstationType: "process",
		SystemPrompt:    "system prompt",
		UserMessage:     "user prompt",
	})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if resp.Diagnostics == nil ||
		resp.Diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] != "provider_response" {
		t.Fatalf("diagnostics = %#v, want safe completion evidence", resp.Diagnostics)
	}
	providerSession := (resp.Continuation).SessionMetadata()
	if providerSession == nil || providerSession.ID != "resp-no-diagnostics" {
		t.Fatalf("provider session = %#v, want resp-no-diagnostics", providerSession)
	}
}

func TestSideEffects_InferResolvesFailureMetadataOnlyRecordedFailure(t *testing.T) {
	artifact := testReplayArtifact(
		t,
		replayDispatchCreatedEvent(t, work.WorkDispatch{
			DispatchID:      "dispatch-failure-metadata",
			TransitionID:    "process",
			WorkerType:      "worker-a",
			WorkstationName: "process",
			Execution: work.ExecutionMetadata{
				ReplayKey: "process/trace-failure/work-failure",
				TraceID:   "trace-failure",
				WorkIDs:   []string{"work-failure"},
			},
		}, 2),
		replayDispatchCompletedEvent(t, "completion-failure-metadata", workerexecution.WorkResult{
			DispatchID:   "dispatch-failure-metadata",
			TransitionID: "process",
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "provider throttled",
			FailureMetadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyThrottle,
				Type:   workerexecution.WorkFailureTypeThrottled,
			},
		}, 3),
	)
	assignEventSequences(artifact.Events)
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	_, err = sideEffects.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			WorkerType: "worker-a",
			Execution: work.ExecutionMetadata{
				ReplayKey: "process/trace-failure/work-failure",
				TraceID:   "trace-failure",
				WorkIDs:   []string{"work-failure"},
			},
		},
		WorkstationType: "process",
		SystemPrompt:    "system prompt",
		UserMessage:     "user prompt",
	})
	if err == nil {
		t.Fatal("expected recorded failure_metadata-only completion to surface provider error")
	}
	if !strings.Contains(err.Error(), "throttled") {
		t.Fatalf("Infer error = %v, want throttled failure from canonical metadata", err)
	}
}

func TestSideEffects_DispatchWithoutCompletionFailsExplicitly(t *testing.T) {
	artifact := replaySideEffectArtifact(t)
	dispatchEvent, err := interfaces.NewFactoryEvent(replayDispatchCreatedEvent(t, work.WorkDispatch{
		DispatchID:      "dispatch-no-completion",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: work.ExecutionMetadata{
			ReplayKey: "process/trace-3/work-3",
			TraceID:   "trace-3",
			WorkIDs:   []string{"work-3"},
		},
	}, 6))
	if err != nil {
		t.Fatalf("convert dispatch event: %v", err)
	}
	artifact.Events = append(artifact.Events, dispatchEvent)
	assignEventSequences(artifact.Events)
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}

	_, err = sideEffects.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			WorkerType: "worker-a",
			Execution: work.ExecutionMetadata{
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
	artifact, _, _ := replaySideEffectArtifactWithDiagnostics(t)
	return artifact
}

func replaySideEffectArtifactWithDiagnostics(t *testing.T) (*interfaces.ReplayArtifact, *workerexecution.WorkDiagnostics, *workerexecution.WorkDiagnostics) {
	t.Helper()
	dispatchProvider := work.WorkDispatch{
		DispatchID:      "dispatch-provider",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: work.ExecutionMetadata{
			ReplayKey: "process/trace-1/work-1",
			TraceID:   "trace-1",
			WorkIDs:   []string{"work-1"},
		},
	}
	dispatchCommand := work.WorkDispatch{
		DispatchID:      "dispatch-command",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: work.ExecutionMetadata{
			ReplayKey: "process/trace-2/work-2",
			TraceID:   "trace-2",
			WorkIDs:   []string{"work-2"},
		},
	}
	providerDiagnostics := &workerexecution.WorkDiagnostics{
		RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
		},
		Provider: &workerexecution.ProviderDiagnostic{
			Provider: "claude",
			Model:    "claude-3-5-haiku-20241022",
			ResponseMetadata: map[string]string{
				"request_id": "req-1",
			},
		},
		Panic:    &workerexecution.PanicDiagnostic{Message: "unsafe panic", Stack: "unsafe stack"},
		Metadata: map[string]string{"phase": "unsafe"},
	}
	commandDiagnostics := &workerexecution.WorkDiagnostics{
		Command: &workerexecution.CommandDiagnostic{
			Command:  "echo",
			Args:     []string{"ok"},
			Stdout:   "recorded script output\n",
			Stderr:   "recorded script details\n",
			ExitCode: 0,
		},
		Metadata: map[string]string{"phase": "unsafe"},
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
		replayDispatchCompletedEvent(t, "completion-provider", workerexecution.WorkResult{
			DispatchID:   "dispatch-provider",
			TransitionID: "process",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "recorded provider output",
			Diagnostics:  providerDiagnostics,
		}, 4),
		replayDispatchCompletedEvent(t, "completion-command", workerexecution.WorkResult{
			DispatchID:   "dispatch-command",
			TransitionID: "process",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "recorded script output\n",
			Error:        "recorded script details\n",
			Diagnostics:  commandDiagnostics,
		}, 5),
	), providerDiagnostics, commandDiagnostics
}

func TestSideEffects_CancellationDoesNotConsumeRecordedProviderResponse(t *testing.T) {
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, replaySideEffectArtifact(t))
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}
	request := workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			WorkerType: "worker-a",
			Execution: work.ExecutionMetadata{
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
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sideEffects.Infer(canceled, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("Infer canceled error = %v, want context.Canceled", err)
	}

	response, err := sideEffects.Infer(context.Background(), request)
	if err != nil {
		t.Fatalf("Infer after cancellation: %v", err)
	}
	if response.Content != "recorded provider output" {
		t.Fatalf("content after cancellation = %q, want retained recorded response", response.Content)
	}
}

func TestSideEffects_CancellationDoesNotConsumeRecordedCommandResult(t *testing.T) {
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, replaySideEffectArtifact(t))
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}
	request := platformprocess.CommandRequest{
		Command: "echo",
		Args:    []string{"ok"},
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sideEffects.Run(canceled, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run canceled error = %v, want context.Canceled", err)
	}

	result, err := sideEffects.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run after cancellation: %v", err)
	}
	if string(result.Stdout) != "recorded script output\n" {
		t.Fatalf("stdout after cancellation = %q, want retained recorded result", result.Stdout)
	}
}
