package events

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/projections"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryEventHistory_RecordOrchestratorProgress_EmitsReconstructablePhaseAndCheckpointSequence(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 13, 0, 0, 0, time.UTC)
	phaseStartedAt := t0.Add(2 * time.Second)
	checkpointTime := t0.Add(3 * time.Second)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	kind := interfaces.OrchestratorKindJavaScript
	history.RecordOrchestratorPhaseChanged(OrchestratorPhaseChangedInput{
		SessionID:        "session-js",
		OrchestratorKind: kind,
		PhaseID:          "phase-plan",
		PhaseName:        "plan",
		Source:           "runtime",
		Tick:             1,
		PhaseStatus:      interfaces.OrchestratorPhaseStatusActive,
		StartedAt:        &phaseStartedAt,
		ProgressSummary:  "Planning workflow",
	}, phaseStartedAt)
	history.RecordOrchestratorPhaseChanged(OrchestratorPhaseChangedInput{
		SessionID:         "session-js",
		OrchestratorKind:  kind,
		PhaseID:           "phase-execute",
		PhaseName:         "execute",
		Source:            "runtime",
		Tick:              2,
		PreviousPhaseID:   "phase-plan",
		PreviousPhaseName: "plan",
		PhaseStatus:       interfaces.OrchestratorPhaseStatusActive,
		StartedAt:         &checkpointTime,
		ProgressSummary:   "Entered execute phase",
	}, checkpointTime)
	hash := "sha256:checkpoint-body"
	size := int64(128)
	history.RecordOrchestratorCheckpointWritten(OrchestratorCheckpointWrittenInput{
		SessionID:             "session-js",
		OrchestratorKind:      kind,
		PhaseID:               "phase-execute",
		PhaseName:             "execute",
		CheckpointID:          "ckpt-1",
		Source:                "runtime",
		Tick:                  2,
		Label:                 "after-plan",
		Timestamp:             &checkpointTime,
		SourceHash:            "sha256:source",
		RuntimeSnapshotDigest: "sha256:snapshot",
		ResumabilityStatus:    interfaces.CheckpointResumabilityStatusResumable,
		ArtifactRef: &interfaces.FactoryArtifactRef{
			ID:          "artifact-ckpt-1",
			Kind:        interfaces.JavaScriptCheckpointArtifactKind,
			Visibility:  interfaces.JavaScriptCheckpointArtifactVisibility,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
		Warnings: []interfaces.FactoryDispatchWarning{{
			Code:    "checkpoint_stale_inputs",
			Message: "Some inputs were captured before the latest dispatch completed",
		}},
	}, checkpointTime)

	events := generatedHistoryEvents(t, history)
	if len(events) != 3 {
		t.Fatalf("events = %d, want phase, phase, checkpoint", len(events))
	}
	assertOrchestratorProgressEventType(t, events[0], factoryapi.FactoryEventTypeOrchestratorPhaseChanged)
	assertOrchestratorProgressEventType(t, events[1], factoryapi.FactoryEventTypeOrchestratorPhaseChanged)
	assertOrchestratorProgressEventType(t, events[2], factoryapi.FactoryEventTypeOrchestratorCheckpointWritten)

	worldState, err := projections.ReconstructCanonicalFactoryWorldState(history.CanonicalEvents(), 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.JavaScriptRuntime == nil {
		t.Fatal("javascript runtime = nil, want phase history")
	}
	if worldState.JavaScriptRuntime.Phase != "execute" || len(worldState.JavaScriptRuntime.Phases) != 2 {
		t.Fatalf("phase history = %#v, want execute with [plan execute]", worldState.JavaScriptRuntime)
	}
	if len(worldState.JavaScriptCheckpoints) != 1 {
		t.Fatalf("checkpoints = %#v, want one checkpoint ref", worldState.JavaScriptCheckpoints)
	}
	checkpoint := worldState.JavaScriptCheckpoints[0]
	if checkpoint.ID != "ckpt-1" || checkpoint.ResumabilityStatus != string(factoryapi.RESUMABLE) {
		t.Fatalf("checkpoint = %#v, want ckpt-1 RESUMABLE", checkpoint)
	}
	if len(checkpoint.Warnings) != 1 || checkpoint.Warnings[0].Code != "checkpoint_stale_inputs" {
		t.Fatalf("checkpoint warnings = %#v, want stale input warning", checkpoint.Warnings)
	}
}

func assertOrchestratorProgressEventType(
	t *testing.T,
	event factoryapi.FactoryEvent,
	wantType factoryapi.FactoryEventType,
) {
	t.Helper()
	if event.Type != wantType {
		t.Fatalf("event type = %q, want %q", event.Type, wantType)
	}
	if event.Context.SessionId == nil || *event.Context.SessionId != "session-js" {
		t.Fatalf("session id = %#v, want session-js", event.Context.SessionId)
	}
}
