// Package javascriptsummary implements Factory Runtime's durable JavaScript
// checkpoint-summary projection contract inside the parent-private checkpoint
// recovery layout.
package javascriptsummary

import (
	"sort"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// Service projects durable checkpoint summaries from runtime records.
type Service struct{}

var _ factoryruntime.JavaScriptCheckpointSummaries = Service{}

// New constructs the stateless checkpoint-summary projector.
func New() Service {
	return Service{}
}

// Build projects one checkpoint summary from runtime records and session
// metadata at the point a checkpoint is written or interrupted.
func (Service) Build(
	input factoryruntime.JavaScriptCheckpointSummaryInput,
) *factoryruntime.JavaScriptCheckpointSummary {
	checkpointID := strings.TrimSpace(input.CheckpointID)
	if checkpointID == "" {
		return nil
	}
	completed, pending, artifacts := dispatchAndArtifactIDsFromRecords(input.Records)
	summary := &factoryruntime.JavaScriptCheckpointSummary{
		SchemaVersion:        factoryruntime.JavaScriptCheckpointSummarySchemaVersion,
		Kind:                 factoryruntime.JavaScriptCheckpointSummaryKind,
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
		ResumeStrategy:       factoryruntime.JavaScriptResumeStrategy,
		CheckpointState:      cloneCheckpointState(input.CheckpointState),
	}
	if !input.CreatedAt.IsZero() {
		summary.CreatedAt = input.CreatedAt.UTC()
	}
	return summary
}

// Latest returns the most recent checkpoint summary derivable from ordered
// runtime records and optional session metadata.
func (s Service) Latest(
	input factoryruntime.JavaScriptCheckpointSummaryInput,
) *factoryruntime.JavaScriptCheckpointSummary {
	if len(input.Records) == 0 {
		return nil
	}
	var latest *factoryruntime.JavaScriptCheckpointRecord
	for _, record := range input.Records {
		if record.Kind != factoryruntime.JavaScriptRecordKindCheckpoint || record.Checkpoint == nil {
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
	return s.Build(projected)
}

func dispatchAndArtifactIDsFromRecords(
	records []factoryruntime.JavaScriptRuntimeRecord,
) (completed, pending, artifacts []string) {
	dispatchStatus, artifactSet := collectDispatchAndArtifactState(records)
	completed, pending = classifyDispatchIDs(dispatchStatus)
	return completed, pending, sortedStringSet(artifactSet)
}

func collectDispatchAndArtifactState(
	records []factoryruntime.JavaScriptRuntimeRecord,
) (map[string]string, map[string]struct{}) {
	dispatchStatus := make(map[string]string)
	artifactSet := make(map[string]struct{})
	for _, record := range records {
		switch record.Kind {
		case factoryruntime.JavaScriptRecordKindArtifact:
			if record.Artifact != nil {
				if artifactID := strings.TrimSpace(record.Artifact.ID); artifactID != "" {
					artifactSet[artifactID] = struct{}{}
				}
			}
		case factoryruntime.JavaScriptRecordKindChildDispatch:
			rememberChildDispatchRecord(record, dispatchStatus, artifactSet)
		}
	}
	return dispatchStatus, artifactSet
}

func rememberChildDispatchRecord(
	record factoryruntime.JavaScriptRuntimeRecord,
	dispatchStatus map[string]string,
	artifactSet map[string]struct{},
) {
	if record.ChildDispatch == nil {
		return
	}
	dispatchID := strings.TrimSpace(record.ChildDispatch.DispatchID)
	if dispatchID == "" {
		return
	}
	dispatchStatus[dispatchID] = strings.TrimSpace(record.ChildDispatch.Status)
	if ref := strings.TrimSpace(record.ChildDispatch.ArtifactRef); ref != "" {
		if parsed, ok := artifactIDFromURI(ref); ok {
			artifactSet[parsed] = struct{}{}
		}
	}
}

func classifyDispatchIDs(dispatchStatus map[string]string) (completed, pending []string) {
	dispatchIDs := make([]string, 0, len(dispatchStatus))
	for dispatchID := range dispatchStatus {
		dispatchIDs = append(dispatchIDs, dispatchID)
	}
	sort.Strings(dispatchIDs)
	for _, dispatchID := range dispatchIDs {
		switch dispatchStatus[dispatchID] {
		case factoryruntime.JavaScriptChildDispatchStatusCompleted:
			completed = append(completed, dispatchID)
		case factoryruntime.JavaScriptChildDispatchStatusQueued,
			factoryruntime.JavaScriptChildDispatchStatusRunning:
			pending = append(pending, dispatchID)
		}
	}
	return completed, pending
}

func sortedStringSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	sorted := make([]string, 0, len(values))
	for value := range values {
		sorted = append(sorted, value)
	}
	sort.Strings(sorted)
	return sorted
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

func artifactIDFromURI(uri string) (string, bool) {
	const prefix = "you-artifact://sessions/"
	trimmed := strings.TrimSpace(uri)
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(trimmed, prefix), "/artifacts/")
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}
