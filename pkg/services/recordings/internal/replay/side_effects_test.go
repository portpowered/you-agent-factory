package replay

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
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

func TestSideEffects_InvokeModelReconstructsRecordedOutputsAndArtifacts(t *testing.T) {
	artifactRef := replayTestArtifactRef(t, "models-inference:artifact:replay-audio")
	content := []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "recorded text"},
		{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"ok":true}`), ContentType: "application/json"},
		{
			Type: work.WorkContentPartTypeAudio, URL: "data:audio/wav;base64,YXVkaW8=", ContentType: "audio/wav",
			ArtifactID: artifactRef.String(), Label: "speech.wav", Metadata: map[string]any{"digest": "abc", " ": "ignored", "nil": nil},
		},
		{Type: work.WorkContentPartTypeImage, File: "file://recorded.png", ContentType: "image/png"},
		{Type: work.WorkContentPartTypeBinary, Text: "binary-reference", ContentType: "application/octet-stream"},
	}
	result := invokeRecordedModelForReplayTest(t, workerexecution.WorkResult{
		DispatchID:   "dispatch-model",
		TransitionID: "tts",
		Outcome:      workerexecution.OutcomeAccepted,
		RecordedOutputWork: []work.FactoryWorkItem{{
			ID: "output-work", WorkTypeID: "task", State: "complete", Content: content,
		}},
	})
	assertReplayModelIdentity(t, result)
	assertReplayModelOutputCount(t, result)
	assertReplayTextOutput(t, result.Outputs[0])
	assertReplayAudioOutput(t, result.Outputs[2], artifactRef)
	assertReplayContentProjection(t, result)
}

func TestReplayModelOutputsUsesOutputContent(t *testing.T) {
	outputs, err := replayModelOutputs(workerexecution.WorkResult{
		OutputContent: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeAudio, Text: "audio-from-output-content", ContentType: "audio/wav",
		}},
	})
	if err != nil || len(outputs) != 1 || outputs[0].Modality != models.ModalityAudio {
		t.Fatalf("replayModelOutputs(OutputContent) = %#v, %v; want one audio output", outputs, err)
	}
}

func TestSideEffects_InvokeModelReportsRecordedNoOutput(t *testing.T) {
	_, err := invokeRecordedModelForReplayTestWithError(t, workerexecution.WorkResult{
		DispatchID:   "dispatch-no-output",
		TransitionID: "tts",
		Outcome:      workerexecution.OutcomeAccepted,
	})
	if err == nil || !strings.Contains(err.Error(), "has no output") {
		t.Fatalf("no-output InvokeModel() error = %v, want recorded no-output error", err)
	}
}

func TestSideEffects_InvokeModelReportsRecordedFailure(t *testing.T) {
	result, err := invokeRecordedModelForReplayTestWithError(t, workerexecution.WorkResult{
		DispatchID:   "dispatch-failure",
		TransitionID: "tts",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        " ",
	})
	if err == nil || !strings.Contains(err.Error(), "recorded model invocation failed") {
		t.Fatalf("failure InvokeModel() error = %v", err)
	}
	if result.Status != models.ModelInvocationStatusFailed || result.ModelName != "tts" || result.Operation != models.OperationTTS {
		t.Fatalf("failure InvokeModel() result = %#v", result)
	}
}

func TestReplayModelOutputHelpersSelectRecordedContent(t *testing.T) {
	if outputs, err := replayModelOutputs(workerexecution.WorkResult{OutputContent: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "inline"}}}); err != nil || len(outputs) != 1 {
		t.Fatalf("replayModelOutputs(OutputContent) = %#v, %v; want one output", outputs, err)
	}
	if outputs, err := replayModelOutputs(workerexecution.WorkResult{RecordedOutputWork: []work.FactoryWorkItem{{Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "recorded fallback"}}}}}); err != nil || len(outputs) != 1 {
		t.Fatalf("replayModelOutputs(RecordedOutputWork) = %#v, %v; want one output", outputs, err)
	}
	if outputs, err := replayModelOutputs(workerexecution.WorkResult{}); err != nil || outputs != nil {
		t.Fatalf("replayModelOutputs(empty) = %#v, %v; want nil", outputs, err)
	}
}

func TestReplayModelOutputHelpersHandleEmptyPropertiesAndReferences(t *testing.T) {
	var nilSideEffects *SideEffects
	if _, err := nilSideEffects.InvokeModel(context.Background(), models.InvokeModelRequest{}); err == nil || !strings.Contains(err.Error(), "replay side effects are required") {
		t.Fatalf("nil SideEffects InvokeModel() error = %v, want required-side-effects error", err)
	}
	if properties := replayArtifactProperties(nil); properties != nil {
		t.Fatalf("replayArtifactProperties(nil) = %#v, want nil", properties)
	}
	if properties := replayArtifactProperties(map[string]any{" ": "ignored", "nil": nil}); properties != nil {
		t.Fatalf("replayArtifactProperties(only invalid) = %#v, want nil", properties)
	}
	assertReplayContentReference(t, work.WorkContentPart{}, "")
	assertReplayContentReference(t, work.WorkContentPart{File: "file://fallback"}, "file://fallback")
	assertReplayContentReference(t, work.WorkContentPart{Text: "text-fallback"}, "text-fallback")
}

func TestReplayModelOutputHelpersMapSupportedValues(t *testing.T) {
	for _, part := range []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "text"},
		{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"ok":true}`)},
		{Type: work.WorkContentPartTypeAudio, URL: "audio"},
		{Type: work.WorkContentPartTypeImage, URL: "image"},
		{Type: work.WorkContentPartTypeBinary, URL: "binary"},
		{Type: work.WorkContentPartType("unknown"), Text: "unknown"},
	} {
		modality, value := replayModelOutputValue(part)
		if part.Type.Normalized() != work.WorkContentPartType("unknown") && (modality == "" || value == "") {
			t.Fatalf("replayModelOutputValue(%#v) = %q/%q", part, modality, value)
		}
	}
}

func replayTestArtifactRef(t *testing.T, value string) models.InferenceArtifactRef {
	t.Helper()
	artifact, err := (models.InferenceArtifactRef{}).Parse(value)
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	return artifact
}

func invokeRecordedModelForReplayTest(t *testing.T, result workerexecution.WorkResult) models.InvokeModelResult {
	t.Helper()
	resultValue, err := invokeRecordedModelForReplayTestWithError(t, result)
	if err != nil {
		t.Fatalf("InvokeModel() error = %v", err)
	}
	return resultValue
}

func invokeRecordedModelForReplayTestWithError(t *testing.T, result workerexecution.WorkResult) (models.InvokeModelResult, error) {
	t.Helper()
	artifact := replayModelSideEffectArtifact(t, result)
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects() error = %v", err)
	}
	return sideEffects.InvokeModel(context.Background(), models.InvokeModelRequest{
		Model: models.ModelReference{NameOrURI: "tts"}, Operation: models.OperationTTS,
	})
}

func assertReplayModelIdentity(t *testing.T, result models.InvokeModelResult) {
	t.Helper()
	if result.Status != models.ModelInvocationStatusCompleted || result.ModelName != "tts" || result.Operation != models.OperationTTS {
		t.Fatalf("InvokeModel() result identity = %#v", result)
	}
}

func assertReplayModelOutputCount(t *testing.T, result models.InvokeModelResult) {
	t.Helper()
	if len(result.Outputs) != 5 || len(result.Content) != 5 {
		t.Fatalf("InvokeModel() outputs/content = %d/%d, want five each", len(result.Outputs), len(result.Content))
	}
}

func assertReplayTextOutput(t *testing.T, output models.InferenceOutput) {
	t.Helper()
	if output.Name != "output-0" || output.Modality != models.ModalityText || output.Content != "recorded text" {
		t.Fatalf("recorded text output = %#v", output)
	}
}

func assertReplayAudioOutput(t *testing.T, output models.InferenceOutput, artifactRef models.InferenceArtifactRef) {
	t.Helper()
	if output.Artifact == nil || output.Artifact.Artifact.String() != artifactRef.String() || output.Artifact.Name != "speech.wav" || output.Artifact.Properties["digest"] != "abc" {
		t.Fatalf("recorded audio artifact = %#v", output.Artifact)
	}
}

func assertReplayContentProjection(t *testing.T, result models.InvokeModelResult) {
	t.Helper()
	audio := result.Outputs[2]
	if result.Content[2].Content != audio.Content || result.Content[2].MediaType != audio.MediaType {
		t.Fatalf("content projection = %#v, want output projection %#v", result.Content[2], audio)
	}
}

func assertReplayContentReference(t *testing.T, part work.WorkContentPart, want string) {
	t.Helper()
	if got := firstReplayContentReference(part); got != want {
		t.Fatalf("firstReplayContentReference(%#v) = %q, want %q", part, got, want)
	}
}

func replayModelSideEffectArtifact(t *testing.T, result workerexecution.WorkResult) *interfaces.ReplayArtifact {
	t.Helper()
	dispatch := work.WorkDispatch{
		DispatchID:      result.DispatchID,
		TransitionID:    result.TransitionID,
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: work.ExecutionMetadata{
			ReplayKey: "process/trace-model/" + result.DispatchID,
			TraceID:   "trace-model",
			WorkIDs:   []string{"work-model"},
		},
	}
	return testReplayArtifact(t,
		replayDispatchCreatedEvent(t, dispatch, 1),
		replayDispatchCompletedEvent(t, "completion-"+result.DispatchID, result, 2),
	)
}
