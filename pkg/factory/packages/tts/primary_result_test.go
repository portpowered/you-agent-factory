package tts

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestPackagedTTSInvocationPrimaryResult_ReturnsMetadataNotRawAudio(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	encodedOutput, err := json.Marshal([]interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeAudio, File: audioPath, ContentType: "audio/wav", Slot: "audio",
	}})
	if err != nil {
		t.Fatalf("marshal worker output: %v", err)
	}
	output := string(encodedOutput)
	metadataContent, err := MetadataContentFromWorkerOutput(output, "trace-1", "session-1", "")
	if err != nil {
		t.Fatalf("MetadataContentFromWorkerOutput: %v", err)
	}

	requestID := "req-tts"
	workID := "work-tts"
	submitted := interfaces.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: "task",
		State:      "init",
		TraceID:    requestID,
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "hi there",
		}},
	}
	terminal := interfaces.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: "task",
		State:      "complete",
		TraceID:    requestID,
		PlaceID:    "task:complete",
		Content:    metadataContent,
	}

	state := interfaces.FactoryWorldState{
		PayloadLineage:   interfaces.WorkPayloadLineageProjection{},
		WorkRequestsByID: make(map[string]interfaces.WorkRequestPayload),
		TerminalWorkByID: make(map[string]interfaces.FactoryTerminalWork),
	}
	state.WorkRequestsByID[requestID] = interfaces.WorkRequestPayload{
		RequestID: requestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []interfaces.FactoryWorkItem{submitted},
	}
	state.PayloadLineage.RecordWorkRequestSnapshot(1, requestID, submitted)
	state.PayloadLineage.RecordConsumedInputSnapshot("dispatch-tts", submitted)
	state.PayloadLineage.RecordDispatchOutputSnapshot(2, "dispatch-tts", []interfaces.FactoryWorkItem{submitted}, terminal, 0)
	state.TerminalWorkByID[workID] = interfaces.FactoryTerminalWork{WorkItem: terminal, Status: "TERMINAL"}

	selection, err := invocations.ResolvePrimaryResult(invocations.PrimaryResultSelectionInput{
		RequestID:  requestID,
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult: %v", err)
	}
	if len(selection.PrimaryResult) != 1 || selection.PrimaryResult[0].Type != interfaces.WorkContentPartTypeText {
		t.Fatalf("primary result = %#v, want one text metadata part", selection.PrimaryResult)
	}

	var metadata InvocationMetadata
	if err := json.Unmarshal([]byte(selection.PrimaryResult[0].Text), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if metadata.ArtifactPath != audioPath {
		t.Fatalf("artifactPath = %q, want %q", metadata.ArtifactPath, audioPath)
	}
	if metadata.MediaType != "audio/wav" || metadata.Backend == "" {
		t.Fatalf("metadata = %#v, want media type and backend", metadata)
	}
}
