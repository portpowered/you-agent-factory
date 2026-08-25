package replay

import (
	"context"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestSideEffects_InvokeModelReconstructsRecordedOutputsAndArtifacts(t *testing.T) {
	artifactRef, err := (models.InferenceArtifactRef{}).Parse("models-inference:artifact:replay-audio")
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	content := []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "recorded text"},
		{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"ok":true}`), ContentType: "application/json"},
		{
			Type: work.WorkContentPartTypeAudio, URL: "data:audio/wav;base64,YXVkaW8=", ContentType: "audio/wav",
			ArtifactID: artifactRef.String(), Label: "speech.wav", Metadata: map[string]any{"digest": "abc", " ": "ignored", "nil": nil},
		},
		{Type: work.WorkContentPartTypeImage, File: "file://recorded.png", ContentType: "image/png"},
		{Type: work.WorkContentPartTypeBinary, Text: "binary-reference", ContentType: "application/octet-stream"},
		{Type: work.WorkContentPartType("unsupported"), Text: "ignored"},
		{Type: work.WorkContentPartTypeAudio},
	}
	artifact := replayModelSideEffectArtifact(t, workerexecution.WorkResult{
		DispatchID:   "dispatch-model",
		TransitionID: "tts",
		Outcome:      workerexecution.OutcomeAccepted,
		RecordedOutputWork: []work.FactoryWorkItem{{
			ID: "output-work", WorkTypeID: "task", State: "complete", Content: content,
		}},
	})
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects() error = %v", err)
	}
	result, err := sideEffects.InvokeModel(context.Background(), models.InvokeModelRequest{
		Model:     models.ModelReference{NameOrURI: "tts"},
		Operation: models.OperationTTS,
	})
	if err != nil {
		t.Fatalf("InvokeModel() error = %v", err)
	}
	if result.Status != models.ModelInvocationStatusCompleted || result.ModelName != "tts" || result.Operation != models.OperationTTS {
		t.Fatalf("InvokeModel() result identity = %#v", result)
	}
	if len(result.Outputs) != 5 || len(result.Content) != 5 {
		t.Fatalf("InvokeModel() outputs/content = %d/%d, want five each", len(result.Outputs), len(result.Content))
	}
	if result.Outputs[0].Name != "output-0" || result.Outputs[0].Modality != models.ModalityText || result.Outputs[0].Content != "recorded text" {
		t.Fatalf("recorded text output = %#v", result.Outputs[0])
	}
	audio := result.Outputs[2]
	if audio.Artifact == nil || audio.Artifact.Artifact.String() != artifactRef.String() || audio.Artifact.Name != "speech.wav" || audio.Artifact.Properties["digest"] != "abc" {
		t.Fatalf("recorded audio artifact = %#v", audio.Artifact)
	}
	if result.Content[2].Content != audio.Content || result.Content[2].MediaType != audio.MediaType {
		t.Fatalf("content projection = %#v, want output projection %#v", result.Content[2], audio)
	}
}

func TestSideEffects_InvokeModelUsesOutputContentAndReportsRecordedFailures(t *testing.T) {
	artifact := replayModelSideEffectArtifact(t, workerexecution.WorkResult{
		DispatchID:   "dispatch-output-content",
		TransitionID: "tts",
		Outcome:      workerexecution.OutcomeAccepted,
		RecordedOutputWork: []work.FactoryWorkItem{{Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeAudio, Text: "audio-from-output-content", ContentType: "audio/wav",
		}}}},
	})
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects() error = %v", err)
	}
	result, err := sideEffects.InvokeModel(context.Background(), models.InvokeModelRequest{Model: models.ModelReference{NameOrURI: "tts"}, Operation: models.OperationTTS})
	if err != nil || len(result.Outputs) != 1 || result.Outputs[0].Modality != models.ModalityAudio {
		t.Fatalf("OutputContent InvokeModel() = %#v, %v", result, err)
	}

	noOutputArtifact := replayModelSideEffectArtifact(t, workerexecution.WorkResult{
		DispatchID:   "dispatch-no-output",
		TransitionID: "tts",
		Outcome:      workerexecution.OutcomeAccepted,
	})
	noOutput, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, noOutputArtifact)
	if err != nil {
		t.Fatalf("NewSideEffects(no output) error = %v", err)
	}
	if _, err := noOutput.InvokeModel(context.Background(), models.InvokeModelRequest{Model: models.ModelReference{NameOrURI: "tts"}, Operation: models.OperationTTS}); err == nil || !strings.Contains(err.Error(), "has no output") {
		t.Fatalf("no-output InvokeModel() error = %v, want recorded no-output error", err)
	}

	failureArtifact := replayModelSideEffectArtifact(t, workerexecution.WorkResult{
		DispatchID:   "dispatch-failure",
		TransitionID: "tts",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        " ",
	})
	failureEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, failureArtifact)
	if err != nil {
		t.Fatalf("NewSideEffects(failure) error = %v", err)
	}
	failure, err := failureEffects.InvokeModel(context.Background(), models.InvokeModelRequest{Model: models.ModelReference{NameOrURI: "tts"}, Operation: models.OperationTTS})
	if err == nil || !strings.Contains(err.Error(), "recorded model invocation failed") || failure.Status != models.ModelInvocationStatusFailed || failure.ModelName != "tts" || failure.Operation != models.OperationTTS {
		t.Fatalf("failure InvokeModel() = %#v, %v", failure, err)
	}
}

func TestReplayModelOutputHelpersCoverEmptyAndReferenceForms(t *testing.T) {
	var nilSideEffects *SideEffects
	if _, err := nilSideEffects.InvokeModel(context.Background(), models.InvokeModelRequest{}); err == nil || !strings.Contains(err.Error(), "replay side effects are required") {
		t.Fatalf("nil SideEffects InvokeModel() error = %v, want required-side-effects error", err)
	}
	if outputs, err := replayModelOutputs(workerexecution.WorkResult{OutputContent: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "inline"}}}); err != nil || len(outputs) != 1 {
		t.Fatalf("replayModelOutputs(OutputContent) = %#v, %v; want one output", outputs, err)
	}
	if outputs, err := replayModelOutputs(workerexecution.WorkResult{RecordedOutputWork: []work.FactoryWorkItem{{Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "recorded fallback"}}}}}); err != nil || len(outputs) != 1 {
		t.Fatalf("replayModelOutputs(RecordedOutputWork) = %#v, %v; want one output", outputs, err)
	}
	if outputs, err := replayModelOutputs(workerexecution.WorkResult{}); err != nil || outputs != nil {
		t.Fatalf("replayModelOutputs(empty) = %#v, %v; want nil", outputs, err)
	}
	if properties := replayArtifactProperties(nil); properties != nil {
		t.Fatalf("replayArtifactProperties(nil) = %#v, want nil", properties)
	}
	if properties := replayArtifactProperties(map[string]any{" ": "ignored", "nil": nil}); properties != nil {
		t.Fatalf("replayArtifactProperties(only invalid) = %#v, want nil", properties)
	}
	if got := firstReplayContentReference(work.WorkContentPart{}); got != "" {
		t.Fatalf("firstReplayContentReference(empty) = %q, want empty", got)
	}
	if got := firstReplayContentReference(work.WorkContentPart{File: "file://fallback"}); got != "file://fallback" {
		t.Fatalf("firstReplayContentReference(file) = %q", got)
	}
	if got := firstReplayContentReference(work.WorkContentPart{Text: "text-fallback"}); got != "text-fallback" {
		t.Fatalf("firstReplayContentReference(text) = %q", got)
	}
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
