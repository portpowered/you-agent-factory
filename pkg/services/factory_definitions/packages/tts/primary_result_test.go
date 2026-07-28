package tts

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestPackagedTTSInvocationPrimaryResult_ReturnsMetadataNotRawAudio(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	encodedOutput, err := json.Marshal([]work.WorkContentPart{{
		Type: work.WorkContentPartTypeAudio, File: audioPath, ContentType: "audio/wav", Slot: "audio",
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
	submitted := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: "task",
		State:      "init",
		TraceID:    requestID,
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "hi there",
		}},
	}
	terminal := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: "task",
		State:      "complete",
		TraceID:    requestID,
		PlaceID:    "task:complete",
		Content:    metadataContent,
	}

	state := factorydefinitions.FactoryWorldState{
		PayloadLineage:   work.WorkPayloadLineageProjection{},
		WorkRequestsByID: make(map[string]factorydefinitions.WorkRequestPayload),
		TerminalWorkByID: make(map[string]factorydefinitions.FactoryTerminalWork),
	}
	state.WorkRequestsByID[requestID] = factorydefinitions.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []work.FactoryWorkItem{submitted},
	}
	state.PayloadLineage.RecordWorkRequestSnapshot(1, requestID, submitted)
	state.PayloadLineage.RecordConsumedInputSnapshot("dispatch-tts", submitted)
	state.PayloadLineage.RecordDispatchOutputSnapshot(2, "dispatch-tts", []work.FactoryWorkItem{submitted}, terminal, 0)
	state.TerminalWorkByID[workID] = factorydefinitions.FactoryTerminalWork{WorkItem: terminal, Status: "TERMINAL"}

	selection, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID:  requestID,
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult: %v", err)
	}
	if len(selection.PrimaryResult) != 1 || selection.PrimaryResult[0].Type != work.WorkContentPartTypeText {
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
