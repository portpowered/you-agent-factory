package store

import (
	"sort"
	"strings"
	"time"

	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

const (
	CheckpointSummaryKind                       = "javascript_checkpoint_summary"
	CheckpointSummarySchemaVersion              = 1
	ResumeStrategyReplayCompletedThenContinue     = "replay_completed_dispatches_then_continue"
)

// CheckpointSummary is the durable resume contract for one JavaScript workflow session.
// It stores checkpoint refs, dispatch ids, and sanitized script-visible state only.
type CheckpointSummary struct {
	SchemaVersion        int            `json:"schemaVersion"`
	Kind                 string         `json:"kind"`
	SessionID            string         `json:"sessionId"`
	CheckpointID         string         `json:"checkpointId"`
	Label                string         `json:"label,omitempty"`
	Phase                string         `json:"phase,omitempty"`
	SourceHash           string         `json:"sourceHash,omitempty"`
	ArgsDigest           string         `json:"argsDigest,omitempty"`
	PolicyHash           string         `json:"policyHash,omitempty"`
	CreatedAt            time.Time      `json:"createdAt,omitempty"`
	CompletedDispatchIDs []string       `json:"completedDispatchIds"`
	PendingDispatchIDs   []string       `json:"pendingDispatchIds,omitempty"`
	ArtifactIDs          []string       `json:"artifactIds,omitempty"`
	ResumeStrategy       string         `json:"resumeStrategy"`
	CheckpointState      map[string]any `json:"checkpointState,omitempty"`
}

// BuildCheckpointSummary projects one checkpoint summary from runtime records and
// session metadata at the point a checkpoint is written or a session is interrupted.
func BuildCheckpointSummary(input CheckpointSummaryInput) *CheckpointSummary {
	checkpointID := strings.TrimSpace(input.CheckpointID)
	if checkpointID == "" {
		return nil
	}
	completed, pending, artifacts := dispatchAndArtifactIDsFromRecords(input.Records)
	summary := &CheckpointSummary{
		SchemaVersion:        CheckpointSummarySchemaVersion,
		Kind:                 CheckpointSummaryKind,
		SessionID:            strings.TrimSpace(input.SessionID),
		CheckpointID:         checkpointID,
		Label:                strings.TrimSpace(input.Label),
		Phase:                strings.TrimSpace(input.Phase),
		SourceHash:           strings.TrimSpace(input.SourceHash),
		ArgsDigest:           strings.TrimSpace(input.ArgsDigest),
		PolicyHash:           strings.TrimSpace(input.PolicyHash),
		CompletedDispatchIDs: completed,
		PendingDispatchIDs:   pending,
		ArtifactIDs:          artifacts,
		ResumeStrategy:       ResumeStrategyReplayCompletedThenContinue,
		CheckpointState:      cloneCheckpointState(input.CheckpointState),
	}
	if !input.CreatedAt.IsZero() {
		summary.CreatedAt = input.CreatedAt.UTC()
	}
	return summary
}

// CheckpointSummaryInput carries the facts needed to build one checkpoint summary.
type CheckpointSummaryInput struct {
	SessionID       string
	CheckpointID    string
	Label           string
	Phase           string
	SourceHash      string
	ArgsDigest      string
	PolicyHash      string
	CreatedAt       time.Time
	CheckpointState map[string]any
	Records         []workflowruntime.RuntimeRecord
}

// LatestCheckpointSummaryFromRecords returns the most recent checkpoint summary
// derivable from ordered runtime records and optional session metadata.
func LatestCheckpointSummaryFromRecords(input CheckpointSummaryInput) *CheckpointSummary {
	if len(input.Records) == 0 {
		return nil
	}
	var latest *workflowruntime.CheckpointRecord
	for _, record := range input.Records {
		if record.Kind != workflowruntime.RecordKindCheckpoint || record.Checkpoint == nil {
			continue
		}
		latest = record.Checkpoint
	}
	if latest == nil {
		return nil
	}
	projected := input
	projected.CheckpointID = latest.ID
	projected.Label = latest.Label
	projected.CheckpointState = latest.State
	return BuildCheckpointSummary(projected)
}

func dispatchAndArtifactIDsFromRecords(records []workflowruntime.RuntimeRecord) (completed, pending, artifacts []string) {
	dispatchStatus := make(map[string]string)
	artifactSet := make(map[string]struct{})
	for _, record := range records {
		switch record.Kind {
		case workflowruntime.RecordKindArtifact:
			if record.Artifact != nil && strings.TrimSpace(record.Artifact.ID) != "" {
				artifactSet[record.Artifact.ID] = struct{}{}
			}
		case workflowruntime.RecordKindChildDispatch:
			if record.ChildDispatch == nil {
				continue
			}
			dispatchID := strings.TrimSpace(record.ChildDispatch.DispatchID)
			if dispatchID == "" {
				continue
			}
			dispatchStatus[dispatchID] = strings.TrimSpace(record.ChildDispatch.Status)
			if ref := strings.TrimSpace(record.ChildDispatch.ArtifactRef); ref != "" {
				if parsed, issues := parseArtifactIDFromURI(ref); len(issues) == 0 && parsed != "" {
					artifactSet[parsed] = struct{}{}
				}
			}
		}
	}
	dispatchIDs := make([]string, 0, len(dispatchStatus))
	for dispatchID := range dispatchStatus {
		dispatchIDs = append(dispatchIDs, dispatchID)
	}
	sort.Strings(dispatchIDs)
	for _, dispatchID := range dispatchIDs {
		switch dispatchStatus[dispatchID] {
		case workflowruntime.ChildDispatchStatusCompleted:
			completed = append(completed, dispatchID)
		case workflowruntime.ChildDispatchStatusQueued, workflowruntime.ChildDispatchStatusRunning:
			pending = append(pending, dispatchID)
		}
	}
	if len(artifactSet) == 0 {
		return completed, pending, nil
	}
	artifacts = make([]string, 0, len(artifactSet))
	for artifactID := range artifactSet {
		artifacts = append(artifacts, artifactID)
	}
	sort.Strings(artifacts)
	return completed, pending, artifacts
}

func cloneCheckpointState(state map[string]any) map[string]any {
	if len(state) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(state))
	for key, value := range state {
		cloned[key] = value
	}
	return cloned
}

func parseArtifactIDFromURI(uri string) (string, []string) {
	const prefix = "you-artifact://sessions/"
	trimmed := strings.TrimSpace(uri)
	if !strings.HasPrefix(trimmed, prefix) {
		return "", []string{"invalid artifact uri"}
	}
	rest := strings.TrimPrefix(trimmed, prefix)
	parts := strings.Split(rest, "/artifacts/")
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", []string{"invalid artifact uri"}
	}
	return strings.TrimSpace(parts[1]), nil
}
