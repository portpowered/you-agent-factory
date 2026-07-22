package recordingreplay

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// RecordingReplayProjection is the complete public inspection surface restored
// from a portable recording. It intentionally has no live execution controls.
type RecordingReplayProjection struct {
	Session    fse.SessionReadResult
	Events     fse.EventReadResult
	Artifacts  fse.ListArtifactsResult
	Result     fse.ResultReadResult
	Checkpoint *CheckpointReadModel
}

type CheckpointReadModel struct {
	ID, Label, Summary, ArtifactID string
	Timestamp                      time.Time
}

type ControlOutcome struct {
	Outcome, Detail string
}

// ApplyLifecycleControl reports the stable non-live outcome for recordings.
// Historical projections are inspection-only and cannot be resumed or paused.
func (RecordingReplayProjection) ApplyLifecycleControl(_ fse.LifecycleControlKind) ControlOutcome {
	return ControlOutcome{Outcome: "NON_LIVE_REPLAY", Detail: "recorded Factory Sessions are historical and do not support live lifecycle controls"}
}

// ReplayRecording validates and maps a privacy-bounded recording without
// constructing an executor, provider, network client, or JavaScript runtime.
func ReplayRecording(value recording.PortableRecording) (RecordingReplayProjection, error) {
	if err := recording.ValidatePortableRecording(value); err != nil {
		return RecordingReplayProjection{}, err
	}
	if value.Result == nil {
		return RecordingReplayProjection{}, &recording.PortableRecordingDiagnostic{
			Code: recording.PortableRecordingCodeInvalidSummary, Area: "result", Path: "result",
			Message: "recording has no referenced public result data; migrate or re-record the session",
		}
	}

	artifacts := replayArtifactSummaries(value.Artifacts)
	result := replayResultProjection(value, artifacts)
	session := replaySessionRead(value, result, artifacts)
	events, err := replayEventSummaries(value.Session.ID, value.Events, value.Checkpoint, value.Artifacts)
	if err != nil {
		return RecordingReplayProjection{}, err
	}
	return RecordingReplayProjection{
		Session:    session,
		Events:     fse.EventReadResult{SessionID: value.Session.ID, Events: events},
		Artifacts:  fse.ListArtifactsResult{SessionID: value.Session.ID, Artifacts: artifacts},
		Result:     result,
		Checkpoint: replayCheckpoint(value.Checkpoint),
	}, nil
}

func replaySessionRead(value recording.PortableRecording, result fse.ResultReadResult, artifacts []fse.ArtifactSummary) fse.SessionReadResult {
	status := fse.LifecycleStatus(value.Session.Status)
	if status == "COMPLETED" {
		status = fse.LifecycleStatusSucceeded
	}
	refs := make([]fse.ArtifactRefSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		refs = append(refs, fse.ArtifactRefSummary{ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility, ContentHash: artifact.ContentHash, SizeBytes: artifact.SizeBytes})
	}
	session := fse.SessionReadResult{
		SessionID: value.Session.ID, Status: status, OrchestratorKind: value.Session.OrchestratorKind,
		ResolvedSource: fse.ResolvedSource{SourceRef: value.Source.Ref, SourceHash: value.Source.Hash},
		SourceHash:     value.Source.Hash, Policy: fse.PolicyProjection{EffectiveHash: value.PolicyHash},
		Usage: fse.EmptySessionUsage(), ResultSummary: &fse.ResultSummary{ResultStatus: string(result.ResultStatus)},
		ArtifactRefs: refs, ArtifactCount: len(refs), Links: fse.InspectionLinksForSession(value.Session.ID, true),
	}
	session.Lifecycle = replayLifecycle(value.Events)
	if result.Failure != nil {
		failure := *result.Failure
		session.Failure = &failure
	}
	return session
}

func replayLifecycle(events []recording.PortableRecordingEventSummary) *fse.LifecycleTimestamps {
	lifecycle := &fse.LifecycleTimestamps{}
	for _, event := range events {
		timestamp := event.Timestamp
		switch event.Type {
		case "SESSION_STARTED":
			lifecycle.StartedAt = &timestamp
		case "SESSION_PAUSED":
			lifecycle.PausedAt = &timestamp
		case "SESSION_RESUMED":
			lifecycle.ResumedAt = &timestamp
		case "SESSION_COMPLETED":
			lifecycle.FinishedAt = &timestamp
		}
	}
	return lifecycle
}

func replayCheckpoint(value *recording.PortableRecordingCheckpointSummary) *CheckpointReadModel {
	if value == nil {
		return nil
	}
	return &CheckpointReadModel{ID: value.ID, Label: value.Label, Summary: value.Summary, Timestamp: value.Timestamp.UTC(), ArtifactID: value.ArtifactID}
}

func replayResultProjection(value recording.PortableRecording, artifacts []fse.ArtifactSummary) fse.ResultReadResult {
	result := value.Result
	projected := fse.ResultReadResult{
		SessionID: value.Session.ID, SessionStatus: fse.LifecycleStatus(value.Session.Status),
		ResultStatus: fse.ResultStatus(result.Status), Mode: fse.ResultMode(result.Mode),
		PrimaryResult: append(json.RawMessage(nil), result.PrimaryResult...),
		ArtifactIDs:   append([]string(nil), result.ArtifactIDs...),
	}
	if projected.SessionStatus == "COMPLETED" {
		projected.SessionStatus = fse.LifecycleStatusSucceeded
	}
	artifactByID := make(map[string]fse.ArtifactSummary, len(artifacts))
	for _, artifact := range artifacts {
		artifactByID[artifact.ID] = artifact
	}
	for _, id := range projected.ArtifactIDs {
		artifact := artifactByID[id]
		projected.ArtifactRefs = append(projected.ArtifactRefs, fse.ArtifactRefSummary{ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility, ContentHash: artifact.ContentHash, SizeBytes: artifact.SizeBytes})
	}
	if result.Failure != nil {
		projected.Failure = &fse.FailureSummary{Reason: result.Failure.Reason, Message: result.Failure.Message, PartialResultAvailable: result.Failure.PartialResultAvailable}
	}
	if result.Availability != nil {
		projected.Availability = &fse.ResultAvailabilityDetail{Reason: result.Availability.Reason, Message: result.Availability.Message, Retryable: result.Availability.Retryable}
	}
	return projected
}

func replayArtifactSummaries(values []recording.PortableRecordingArtifactSummary) []fse.ArtifactSummary {
	artifacts := make([]fse.ArtifactSummary, 0, len(values))
	for _, value := range values {
		createdAt := value.CreatedAt
		artifacts = append(artifacts, fse.ArtifactSummary{ID: value.ID, Kind: value.Kind, Visibility: value.Visibility, Label: value.Label, ContentHash: value.ContentHash, SizeBytes: value.SizeBytes, CreatedAt: &createdAt})
	}
	return artifacts
}

func replayEventSummaries(sessionID string, values []recording.PortableRecordingEventSummary, checkpoint *recording.PortableRecordingCheckpointSummary, artifacts []recording.PortableRecordingArtifactSummary) ([]json.RawMessage, error) {
	events := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		context := map[string]any{"sessionId": sessionID, "sequence": value.Sequence, "eventTime": value.Timestamp}
		payload := map[string]any{"artifactIds": value.ArtifactIDs}
		if value.CheckpointID != "" {
			context["checkpointId"] = value.CheckpointID
			if checkpoint != nil && checkpoint.ID == value.CheckpointID {
				payload = replayCheckpointEventPayload(*checkpoint, artifacts)
			}
		}
		event := map[string]any{
			"id": value.ID, "type": value.Type,
			"context": context,
			"payload": payload,
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("replay event summary %q: %w", strings.TrimSpace(value.ID), err)
		}
		events = append(events, encoded)
	}
	return events, nil
}

func replayCheckpointEventPayload(checkpoint recording.PortableRecordingCheckpointSummary, artifacts []recording.PortableRecordingArtifactSummary) map[string]any {
	payload := map[string]any{
		"checkpointId": checkpoint.ID,
		"label":        checkpoint.Label,
		"summary":      checkpoint.Summary,
		"timestamp":    checkpoint.Timestamp,
	}
	for _, artifact := range artifacts {
		if artifact.ID == checkpoint.ArtifactID {
			payload["artifactRef"] = map[string]any{
				"id": artifact.ID, "kind": artifact.Kind, "visibility": artifact.Visibility,
				"contentHash": artifact.ContentHash, "sizeBytes": artifact.SizeBytes,
			}
			break
		}
	}
	return payload
}
