package checkpointsummary_test

import (
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/checkpointsummary"
)

func TestServiceBuildProjectsStableDispatchAndArtifactState(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.FixedZone("test", -7*60*60))
	summary := checkpointsummary.New().Build(factoryruntime.JavaScriptCheckpointSummaryInput{
		SessionID:    " session-1 ",
		CheckpointID: " checkpoint-1 ",
		CreatedAt:    createdAt,
		Records: []factoryruntime.JavaScriptRuntimeRecord{
			{
				Kind: factoryruntime.JavaScriptRecordKindChildDispatch,
				ChildDispatch: &factoryruntime.JavaScriptChildDispatchRecord{
					DispatchID: "dispatch-b",
					Status:     factoryruntime.JavaScriptChildDispatchStatusRunning,
				},
			},
			{
				Kind: factoryruntime.JavaScriptRecordKindChildDispatch,
				ChildDispatch: &factoryruntime.JavaScriptChildDispatchRecord{
					DispatchID:  "dispatch-a",
					Status:      factoryruntime.JavaScriptChildDispatchStatusCompleted,
					ArtifactRef: "you-artifact://sessions/session-1/artifacts/artifact-b",
				},
			},
			{
				Kind:     factoryruntime.JavaScriptRecordKindArtifact,
				Artifact: &factoryruntime.JavaScriptArtifactRecord{ID: "artifact-a"},
			},
		},
	})

	if summary == nil {
		t.Fatal("Build = nil, want checkpoint summary")
	}
	if summary.SessionID != "session-1" || summary.CheckpointID != "checkpoint-1" {
		t.Fatalf("identity = (%q, %q), want trimmed values", summary.SessionID, summary.CheckpointID)
	}
	if len(summary.CompletedDispatchIDs) != 1 || summary.CompletedDispatchIDs[0] != "dispatch-a" {
		t.Fatalf("completed dispatches = %#v", summary.CompletedDispatchIDs)
	}
	if len(summary.PendingDispatchIDs) != 1 || summary.PendingDispatchIDs[0] != "dispatch-b" {
		t.Fatalf("pending dispatches = %#v", summary.PendingDispatchIDs)
	}
	if len(summary.ArtifactIDs) != 2 ||
		summary.ArtifactIDs[0] != "artifact-a" ||
		summary.ArtifactIDs[1] != "artifact-b" {
		t.Fatalf("artifacts = %#v, want stable order", summary.ArtifactIDs)
	}
	if !summary.CreatedAt.Equal(createdAt.UTC()) || summary.CreatedAt.Location() != time.UTC {
		t.Fatalf("createdAt = %v, want UTC", summary.CreatedAt)
	}
}

func TestServiceLatestUsesLastCheckpointRecord(t *testing.T) {
	t.Parallel()

	summary := checkpointsummary.New().Latest(factoryruntime.JavaScriptCheckpointSummaryInput{
		SessionID: "session-1",
		Records: []factoryruntime.JavaScriptRuntimeRecord{
			{
				Kind:       factoryruntime.JavaScriptRecordKindCheckpoint,
				Checkpoint: &factoryruntime.JavaScriptCheckpointRecord{ID: "checkpoint-1"},
			},
			{
				Kind: factoryruntime.JavaScriptRecordKindCheckpoint,
				Checkpoint: &factoryruntime.JavaScriptCheckpointRecord{
					ID: "checkpoint-2", Label: "latest", State: map[string]any{"phase": "review"},
				},
			},
		},
	})

	if summary == nil || summary.CheckpointID != "checkpoint-2" || summary.Label != "latest" {
		t.Fatalf("Latest = %#v, want final checkpoint", summary)
	}
}
