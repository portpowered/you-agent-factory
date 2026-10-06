package recordingreplay

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/mohae/deepcopy"
	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Historical reads never lend mutable projection storage to callers. Copies
// preserve native JSON values and nil/empty collections without re-encoding.
func copySession(value fse.SessionReadResult) fse.SessionReadResult {
	value.ResolvedSource.ResolutionOrder = slices.Clone(value.ResolvedSource.ResolutionOrder)
	value.ResolvedSource.Metadata = maps.Clone(value.ResolvedSource.Metadata)
	value.ResolvedSource.Agents = maps.Clone(value.ResolvedSource.Agents)
	value.ResolvedSource.ArgsSchema = slices.Clone(value.ResolvedSource.ArgsSchema)
	value.ResolvedSource.DefaultPolicy = slices.Clone(value.ResolvedSource.DefaultPolicy)
	value.Policy.Requested = copyPolicy(value.Policy.Requested)
	value.Policy.Effective = copyPolicy(value.Policy.Effective)
	value.PhaseSummaries = slices.Clone(value.PhaseSummaries)
	value.LatestCheckpoint = copyPointer(value.LatestCheckpoint)
	value.Progress = copyPointer(value.Progress)
	value.Budgets = copyPointer(value.Budgets)
	value.Usage.Resources = slices.Clone(value.Usage.Resources)
	value.ResultSummary = copyPointer(value.ResultSummary)
	value.ArtifactRefs = slices.Clone(value.ArtifactRefs)
	value.Failure = copyPointer(value.Failure)
	if value.Lifecycle != nil {
		lifecycle := *value.Lifecycle
		lifecycle.QueuedAt = copyPointer(lifecycle.QueuedAt)
		lifecycle.AwaitingApprovalAt = copyPointer(lifecycle.AwaitingApprovalAt)
		lifecycle.StartedAt = copyPointer(lifecycle.StartedAt)
		lifecycle.PausedAt = copyPointer(lifecycle.PausedAt)
		lifecycle.ResumedAt = copyPointer(lifecycle.ResumedAt)
		lifecycle.FinishedAt = copyPointer(lifecycle.FinishedAt)
		lifecycle.InterruptedAt = copyPointer(lifecycle.InterruptedAt)
		lifecycle.TerminatedAt = copyPointer(lifecycle.TerminatedAt)
		lifecycle.UpdatedAt = copyPointer(lifecycle.UpdatedAt)
		value.Lifecycle = &lifecycle
	}
	return value
}

func copyPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyPolicy(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	return jsonvalue.Clone(value).(map[string]any)
}

func copyResult(value fse.ResultReadResult) fse.ResultReadResult {
	value.PrimaryResult = slices.Clone(value.PrimaryResult)
	value.ArtifactIDs = slices.Clone(value.ArtifactIDs)
	value.ArtifactRefs = slices.Clone(value.ArtifactRefs)
	value.Failure = copyPointer(value.Failure)
	value.Availability = copyPointer(value.Availability)
	return value
}

func copyArtifact(value fse.ArtifactSummary) fse.ArtifactSummary {
	value.CreatedAt = copyPointer(value.CreatedAt)
	value.RedactionCounts = copyPointer(value.RedactionCounts)
	value.RetrievalRef = copyPointer(value.RetrievalRef)
	return value
}

func copyArtifacts(value fse.ListArtifactsResult) fse.ListArtifactsResult {
	value.Artifacts = slices.Clone(value.Artifacts)
	for i := range value.Artifacts {
		value.Artifacts[i] = copyArtifact(value.Artifacts[i])
	}
	return value
}

func copyEvents(events []json.RawMessage) []json.RawMessage {
	copy := slices.Clone(events)
	for i := range copy {
		copy[i] = slices.Clone(copy[i])
	}
	return copy
}

func copyWorkerHistory(value recording.PortableRecordingWorkerHistory) recording.PortableRecordingWorkerHistory {
	// Recordings owns copying its portable Worker contract. V3 selects copying
	// the supplied facts without applying the legacy availability projection.
	return recording.NormalizePortableRecordingWorkerHistory(recording.PortableRecording{
		SchemaVersion: recording.PortableRecordingSchemaV3,
		WorkerHistory: &value,
	})
}

func copyProjection(value RecordingReplayProjection) RecordingReplayProjection {
	value.Session = copySession(value.Session)
	value.Result = copyResult(value.Result)
	value.Events.Events = copyEvents(value.Events.Events)
	value.Artifacts = copyArtifacts(value.Artifacts)
	value.Checkpoint = copyPointer(value.Checkpoint)
	value.WorkerHistory = copyWorkerHistory(value.WorkerHistory)
	value.FactoryProjection = copyFactoryProjection(value.FactoryProjection)
	return value
}

func copyFactoryProjection(value *recording.FactoryWorldState) *recording.FactoryWorldState {
	if value == nil {
		return nil
	}
	// This read-only contract contains exported facts, native JSON values and
	// time.Time values, without resource handles. Copy it without serialization
	// so typed numbers, unknown snapshot fields and nil/empty shapes survive.
	return deepcopy.Copy(value).(*recording.FactoryWorldState)
}
