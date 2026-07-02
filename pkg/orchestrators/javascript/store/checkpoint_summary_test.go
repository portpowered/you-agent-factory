package store

import (
	"testing"
	"time"

	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func TestBuildCheckpointSummary_ProjectsCompletedAndPendingDispatches(t *testing.T) {
	records := []workflowruntime.RuntimeRecord{
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID:  "dispatch-1",
				ChildIndex:  1,
				Status:      workflowruntime.ChildDispatchStatusCompleted,
				ArtifactRef: "you-artifact://sessions/dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/artifacts/child-artifact-1",
			},
		},
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID: "dispatch-2",
				ChildIndex: 2,
				Status:     workflowruntime.ChildDispatchStatusRunning,
			},
		},
		{
			Kind: workflowruntime.RecordKindCheckpoint,
			Checkpoint: &workflowruntime.CheckpointRecord{
				ID:    "checkpoint-1",
				Label: "after-step-one",
				State: map[string]any{"step": float64(1)},
			},
		},
	}

	summary := BuildCheckpointSummary(CheckpointSummaryInput{
		SessionID:    "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CheckpointID: "checkpoint-1",
		Label:        "after-step-one",
		Phase:        "execute",
		SourceHash:   "sha256:source",
		PolicyHash:   "sha256:policy",
		CreatedAt:    time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC),
		CheckpointState: map[string]any{
			"step": float64(1),
		},
		Records: records,
	})
	if summary == nil {
		t.Fatal("expected checkpoint summary")
	}
	if summary.Kind != CheckpointSummaryKind {
		t.Fatalf("kind = %q, want %q", summary.Kind, CheckpointSummaryKind)
	}
	if summary.ResumeStrategy != ResumeStrategyReplayCompletedThenContinue {
		t.Fatalf("resumeStrategy = %q", summary.ResumeStrategy)
	}
	if len(summary.CompletedDispatchIDs) != 1 || summary.CompletedDispatchIDs[0] != "dispatch-1" {
		t.Fatalf("completedDispatchIds = %#v, want [dispatch-1]", summary.CompletedDispatchIDs)
	}
	if len(summary.PendingDispatchIDs) != 1 || summary.PendingDispatchIDs[0] != "dispatch-2" {
		t.Fatalf("pendingDispatchIds = %#v, want [dispatch-2]", summary.PendingDispatchIDs)
	}
	if len(summary.ArtifactIDs) != 1 || summary.ArtifactIDs[0] != "child-artifact-1" {
		t.Fatalf("artifactIds = %#v, want [child-artifact-1]", summary.ArtifactIDs)
	}
}

func TestLatestCheckpointSummaryFromRecords_UsesLatestCheckpoint(t *testing.T) {
	records := []workflowruntime.RuntimeRecord{
		{
			Kind: workflowruntime.RecordKindCheckpoint,
			Checkpoint: &workflowruntime.CheckpointRecord{
				ID:    "checkpoint-1",
				Label: "first",
			},
		},
		{
			Kind: workflowruntime.RecordKindCheckpoint,
			Checkpoint: &workflowruntime.CheckpointRecord{
				ID:    "checkpoint-2",
				Label: "second",
				State: map[string]any{"step": float64(2)},
			},
		},
	}

	summary := LatestCheckpointSummaryFromRecords(CheckpointSummaryInput{
		SessionID: "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Records:   records,
	})
	if summary == nil {
		t.Fatal("expected checkpoint summary")
	}
	if summary.CheckpointID != "checkpoint-2" || summary.Label != "second" {
		t.Fatalf("summary = %#v, want latest checkpoint-2", summary)
	}
}
