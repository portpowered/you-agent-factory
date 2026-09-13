package recordingreplay

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// RecordingReplayProjection is the complete public inspection surface restored
// from a historical recording. It intentionally has no live execution controls.
type RecordingReplayProjection struct {
	Session       fse.SessionReadResult
	Events        fse.EventReadResult
	Artifacts     fse.ListArtifactsResult
	Result        fse.ResultReadResult
	WorkerHistory recording.PortableRecordingWorkerHistory
	// FactoryProjection is populated only for legacy event recordings. Portable
	// recordings keep this nil because their privacy-bounded contract does not
	// carry canonical Factory Work facts.
	FactoryProjection *recording.FactoryWorldState
	Checkpoint        *CheckpointReadModel
	Redaction         recording.PortableRecordingRedactionMetadata
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

	artifacts := replayArtifactSummaries(value.Artifacts)
	result := replayResultProjection(value, artifacts)
	session := replaySessionRead(value, result, artifacts)
	events, err := replayEventSummaries(value.Session.ID, value.Events, value.Checkpoint, value.Artifacts)
	if err != nil {
		return RecordingReplayProjection{}, err
	}
	return RecordingReplayProjection{
		Session:           session,
		Events:            fse.EventReadResult{SessionID: value.Session.ID, Events: events},
		Artifacts:         fse.ListArtifactsResult{SessionID: value.Session.ID, Artifacts: artifacts},
		Result:            result,
		WorkerHistory:     recording.NormalizePortableRecordingWorkerHistory(value),
		FactoryProjection: nil,
		Checkpoint:        replayCheckpoint(value.Checkpoint),
		Redaction:         value.Redaction,
	}, nil
}

// ReplayLegacyRecording maps a validated legacy event artifact and its
// Recordings-owned canonical Factory projection into the same detached read
// model used by portable replay. It deliberately consumes recorded worker
// outcomes as facts; an error string is never interpreted as a live decision
// envelope or sent back through execution policy.
func ReplayLegacyRecording(
	value recording.ReplayArtifact,
	sessionID string,
	state recording.FactoryWorldState,
) (RecordingReplayProjection, error) {
	sessionID = legacySessionID(sessionID, state, value.Events)
	enrichLegacyFailureDetails(&state)
	artifacts := replayLegacyArtifactSummaries(state.Artifacts)
	result, err := replayLegacyResultProjection(sessionID, value.Events, state, artifacts)
	if err != nil {
		return RecordingReplayProjection{}, err
	}
	session := replayLegacySessionRead(sessionID, value, state, result, artifacts)
	events, err := replayLegacyEventSummaries(value.Events)
	if err != nil {
		return RecordingReplayProjection{}, err
	}
	return RecordingReplayProjection{
		Session:   session,
		Events:    fse.EventReadResult{SessionID: sessionID, Events: events},
		Artifacts: fse.ListArtifactsResult{SessionID: sessionID, Artifacts: artifacts},
		Result:    result,
		WorkerHistory: recording.PortableRecordingWorkerHistory{
			Availability: recording.PortableRecordingWorkerHistoryUnavailable,
			Reason:       recording.PortableRecordingWorkerHistoryReasonLegacySchema,
		},
		FactoryProjection: &state,
		Redaction:         replayLegacyRedaction(artifacts),
	}, nil
}

func legacySessionID(
	requested string,
	state recording.FactoryWorldState,
	events []recording.FactoryEvent,
) string {
	if state.SessionBracket != nil {
		if value := strings.TrimSpace(state.SessionBracket.SessionID); value != "" {
			return value
		}
	}
	for _, event := range events {
		if event.Context.SessionID != nil {
			if value := strings.TrimSpace(*event.Context.SessionID); value != "" {
				return value
			}
		}
	}
	if value := strings.TrimSpace(requested); value != "" {
		return value
	}
	return "~default"
}

func replayLegacySessionRead(
	sessionID string,
	value recording.ReplayArtifact,
	state recording.FactoryWorldState,
	result fse.ResultReadResult,
	artifacts []fse.ArtifactSummary,
) fse.SessionReadResult {
	status := result.SessionStatus
	if status == "" {
		status = legacyLifecycleStatus(value.Events, state)
	}
	bracket := state.SessionBracket
	sourceRef := ""
	sourceHash := ""
	policyHash := ""
	orchestratorKind := ""
	dialect := ""
	snapshotMetadata := legacyFactorySnapshotFactsFrom(value.Factory)
	if bracket != nil {
		sourceRef = bracket.SourceRef
		sourceHash = bracket.SourceHash
		policyHash = bracket.PolicyHash
		orchestratorKind = bracket.OrchestratorKind
		dialect = bracket.OrchestratorDialect
	}
	sourceRef = firstNonEmpty(sourceRef, snapshotMetadata.SourceRef, snapshotMetadata.SourceDirectory, snapshotMetadata.FactoryDirectory)
	sourceHash = firstNonEmpty(
		sourceHash,
		snapshotMetadata.SourceHash,
		snapshotMetadata.Metadata["sourceHash"],
		snapshotMetadata.Metadata["source_hash"],
		snapshotMetadata.Metadata["factory_hash"],
	)
	orchestratorKind = firstNonEmpty(orchestratorKind, snapshotMetadata.OrchestratorKind)
	dialect = firstNonEmpty(dialect, snapshotMetadata.OrchestratorDialect)
	if orchestratorKind == "" && value.Factory != nil {
		orchestratorKind = factorydefinitions.OrchestratorKindPetri
	}
	for _, event := range value.Events {
		if sourceRef == "" {
			sourceRef = legacyStringPointer(event.Context.Source)
		}
		if orchestratorKind == "" {
			orchestratorKind = legacyStringPointer(event.Context.OrchestratorKind)
		}
		if dialect == "" {
			dialect = legacyStringPointer(event.Context.OrchestratorDialect)
		}
		if event.Type == recording.FactoryEventTypeSessionStarted {
			var payload factorydefinitions.FactorySessionStartedEventPayload
			if event.DecodePayload(&payload) == nil {
				sourceRef = firstNonEmpty(sourceRef, legacyStringPointer(payload.SourceRef))
				sourceHash = firstNonEmpty(sourceHash, legacyStringPointer(payload.SourceHash))
				policyHash = firstNonEmpty(policyHash, legacyStringPointer(payload.PolicyHash))
			}
		}
	}
	lifecycle := replayLegacyLifecycle(value, state)
	refs := make([]fse.ArtifactRefSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		refs = append(refs, fse.ArtifactRefSummary{
			ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
			ContentHash: artifact.ContentHash, SizeBytes: artifact.SizeBytes,
		})
	}
	session := fse.SessionReadResult{
		SessionID:        sessionID,
		Status:           status,
		OrchestratorKind: orchestratorKind,
		Dialect:          dialect,
		ResolvedSource: fse.ResolvedSource{
			SourceRef: sourceRef, SourceHash: sourceHash, Dialect: dialect,
		},
		SourceHash:    sourceHash,
		Policy:        fse.PolicyProjection{EffectiveHash: policyHash},
		Usage:         fse.EmptySessionUsage(),
		ArtifactRefs:  refs,
		ArtifactCount: len(refs),
		Lifecycle:     lifecycle,
		Links:         fse.InspectionLinksForSession(sessionID, true),
	}
	if result.ResultStatus != "" {
		session.ResultSummary = &fse.ResultSummary{ResultStatus: string(result.ResultStatus)}
	}
	if result.Failure != nil {
		failure := *result.Failure
		session.Failure = &failure
	}
	return session
}

type legacyFactorySnapshotFacts struct {
	FactoryDirectory    string
	SourceDirectory     string
	SourceRef           string
	SourceHash          string
	Metadata            map[string]string `json:"metadata"`
	OrchestratorKind    string
	OrchestratorDialect string
}

func legacyFactorySnapshotFactsFrom(snapshot *factorydefinitions.FactorySnapshot) legacyFactorySnapshotFacts {
	if snapshot == nil {
		return legacyFactorySnapshotFacts{}
	}
	var document struct {
		FactoryDirectory string            `json:"factoryDirectory"`
		SourceDirectory  string            `json:"sourceDirectory"`
		SourceRef        string            `json:"sourceRef"`
		SourceHash       string            `json:"sourceHash"`
		Metadata         map[string]string `json:"metadata"`
		Orchestrator     *struct {
			Kind       string `json:"kind"`
			JavaScript *struct {
				Dialect    string `json:"dialect"`
				SourceRef  string `json:"sourceRef"`
				SourceHash string `json:"sourceHash"`
			} `json:"javascript"`
		} `json:"orchestrator"`
	}
	if err := snapshot.Decode(&document); err != nil {
		return legacyFactorySnapshotFacts{}
	}
	metadata := legacyFactorySnapshotFacts{
		FactoryDirectory: document.FactoryDirectory,
		SourceDirectory:  document.SourceDirectory,
		SourceRef:        document.SourceRef,
		SourceHash:       document.SourceHash,
		Metadata:         document.Metadata,
	}
	if document.Orchestrator != nil {
		metadata.OrchestratorKind = document.Orchestrator.Kind
		if document.Orchestrator.JavaScript != nil {
			metadata.OrchestratorDialect = document.Orchestrator.JavaScript.Dialect
			metadata.SourceRef = firstNonEmpty(metadata.SourceRef, document.Orchestrator.JavaScript.SourceRef)
			metadata.SourceHash = firstNonEmpty(metadata.SourceHash, document.Orchestrator.JavaScript.SourceHash)
		}
	}
	return metadata
}

func replayLegacyResultProjection(
	sessionID string,
	events []recording.FactoryEvent,
	state recording.FactoryWorldState,
	artifacts []fse.ArtifactSummary,
) (fse.ResultReadResult, error) {
	status := legacyLifecycleStatus(events, state)
	resultStatus := legacyResultStatus(events, state, status)
	result := fse.ResultReadResult{
		SessionID: sessionID, ResultStatus: resultStatus, SessionStatus: status,
		ArtifactIDs: legacyArtifactIDs(state), IncludeArtifacts: true,
	}
	if bracket := state.SessionBracket; bracket != nil {
		result.ArtifactIDs = append([]string(nil), bracket.ArtifactIDs...)
		if len(bracket.ResultSummary) > 0 {
			encoded, err := json.Marshal(bracket.ResultSummary)
			if err != nil {
				return fse.ResultReadResult{}, fmt.Errorf("encode legacy result summary: %w", err)
			}
			result.PrimaryResult = encoded
		}
	}
	artifactByID := make(map[string]fse.ArtifactSummary, len(artifacts))
	for _, artifact := range artifacts {
		artifactByID[artifact.ID] = artifact
	}
	for _, id := range result.ArtifactIDs {
		if artifact, ok := artifactByID[id]; ok {
			result.ArtifactRefs = append(result.ArtifactRefs, fse.ArtifactRefSummary{
				ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
				ContentHash: artifact.ContentHash, SizeBytes: artifact.SizeBytes,
			})
		}
	}
	if failure := legacyOverallFailure(state); failure != nil {
		result.Failure = &fse.FailureSummary{
			Reason: string(failure.Reason), Message: failure.Message,
			PartialResultAvailable: resultStatus == fse.ResultStatusFailedWithPartial,
		}
	}
	return result, nil
}

func replayLegacyEventSummaries(events []recording.FactoryEvent) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, 0, len(events))
	for index, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("encode legacy replay event %d: %w", index, err)
		}
		result = append(result, encoded)
	}
	return result, nil
}

func replayLegacyArtifactSummaries(values []factorydefinitions.FactorySessionArtifactState) []fse.ArtifactSummary {
	artifacts := make([]fse.ArtifactSummary, 0, len(values))
	for _, value := range values {
		var createdAt *time.Time
		if !value.CapturedAt.IsZero() {
			capturedAt := value.CapturedAt.UTC()
			createdAt = &capturedAt
		}
		var redactions *fse.ArtifactRedactionCounts
		if len(value.RedactionCounts) > 0 {
			redactions = &fse.ArtifactRedactionCounts{
				Paths:   int32(value.RedactionCounts["paths"]),
				Secrets: int32(value.RedactionCounts["secrets"]),
				Tokens:  int32(value.RedactionCounts["tokens"]),
			}
		}
		dispatchID := ""
		if value.CaptureMetadata != nil {
			dispatchID = value.CaptureMetadata["sourceDispatchId"]
		}
		artifacts = append(artifacts, fse.ArtifactSummary{
			ID: value.ID, Kind: value.Kind, Visibility: value.Visibility, Label: value.Label,
			ContentHash: value.ContentHash, SizeBytes: value.SizeBytes, CreatedAt: createdAt,
			DispatchID: dispatchID, AuditMode: value.AuditMode, RedactionCounts: redactions,
		})
	}
	return artifacts
}

func replayLegacyRedaction(artifacts []fse.ArtifactSummary) recording.PortableRecordingRedactionMetadata {
	var secrets int64
	for _, artifact := range artifacts {
		if artifact.RedactionCounts != nil {
			secrets += int64(artifact.RedactionCounts.Secrets)
		}
	}
	return recording.PortableRecordingRedactionMetadata{SecretsRedacted: secrets}
}

func replayLegacyLifecycle(value recording.ReplayArtifact, state recording.FactoryWorldState) *fse.LifecycleTimestamps {
	lifecycle := &fse.LifecycleTimestamps{}
	if bracket := state.SessionBracket; bracket != nil {
		setLegacyTimestamp(&lifecycle.StartedAt, bracket.StartedAt)
		setLegacyTimestamp(&lifecycle.PausedAt, bracket.PausedAt)
		setLegacyTimestamp(&lifecycle.ResumedAt, bracket.ResumedAt)
		setLegacyTimestamp(&lifecycle.FinishedAt, bracket.CompletedAt)
	}
	for _, event := range value.Events {
		timestamp := event.Context.EventTime.UTC()
		if timestamp.IsZero() {
			continue
		}
		switch event.Type {
		case recording.FactoryEventTypeSessionStarted:
			var payload factorydefinitions.FactorySessionStartedEventPayload
			if event.DecodePayload(&payload) == nil && !payload.StartedAt.IsZero() {
				timestamp = payload.StartedAt.UTC()
			}
			setLegacyTimestamp(&lifecycle.StartedAt, timestamp)
		case recording.FactoryEventTypeSessionPaused:
			var payload factorydefinitions.FactorySessionPausedEventPayload
			if event.DecodePayload(&payload) == nil && !payload.PausedAt.IsZero() {
				timestamp = payload.PausedAt.UTC()
			}
			setLegacyTimestamp(&lifecycle.PausedAt, timestamp)
		case recording.FactoryEventTypeSessionResumed:
			var payload factorydefinitions.FactorySessionResumedEventPayload
			if event.DecodePayload(&payload) == nil && !payload.ResumedAt.IsZero() {
				timestamp = payload.ResumedAt.UTC()
			}
			setLegacyTimestamp(&lifecycle.ResumedAt, timestamp)
		case recording.FactoryEventTypeSessionCompleted:
			var payload factorydefinitions.FactorySessionCompletedEventPayload
			if event.DecodePayload(&payload) == nil && !payload.CompletedAt.IsZero() {
				timestamp = payload.CompletedAt.UTC()
			}
			setLegacyTimestamp(&lifecycle.FinishedAt, timestamp)
		}
	}
	if lifecycle.StartedAt == nil && !value.RecordedAt.IsZero() {
		startedAt := value.RecordedAt.UTC()
		lifecycle.StartedAt = &startedAt
	}
	if lifecycle.StartedAt == nil && value.WallClock != nil && !value.WallClock.StartedAt.IsZero() {
		startedAt := value.WallClock.StartedAt.UTC()
		lifecycle.StartedAt = &startedAt
	}
	if lifecycle.FinishedAt == nil && value.WallClock != nil && !value.WallClock.FinishedAt.IsZero() {
		finishedAt := value.WallClock.FinishedAt.UTC()
		lifecycle.FinishedAt = &finishedAt
	}
	return lifecycle
}

func setLegacyTimestamp(destination **time.Time, value time.Time) {
	if destination == nil || value.IsZero() {
		return
	}
	timestamp := value.UTC()
	*destination = &timestamp
}

func legacyLifecycleStatus(events []recording.FactoryEvent, state recording.FactoryWorldState) fse.LifecycleStatus {
	if bracket := state.SessionBracket; bracket != nil {
		if status := normalizeLegacyLifecycleStatus(bracket.FinalStatus); status != "" && bracket.Terminal {
			return status
		}
	}
	if status := normalizeLegacyLifecycleStatus(state.FactoryState); status != "" {
		return status
	}
	status := fse.LifecycleStatusRunning
	for _, event := range events {
		switch event.Type {
		case recording.FactoryEventTypeRunResponse:
			var payload factorydefinitions.RunResponseEventPayload
			if event.DecodePayload(&payload) == nil && payload.State != nil {
				if next := normalizeLegacyLifecycleStatus(string(*payload.State)); next != "" {
					status = next
				}
			}
		case recording.FactoryEventTypeFactoryStateResponse:
			var payload factorydefinitions.FactoryStateResponseEventPayload
			if event.DecodePayload(&payload) == nil {
				if next := normalizeLegacyLifecycleStatus(string(payload.State)); next != "" {
					status = next
				}
			}
		case recording.FactoryEventTypeSessionStarted:
			status = fse.LifecycleStatusRunning
		case recording.FactoryEventTypeSessionPaused:
			status = fse.LifecycleStatusPaused
		case recording.FactoryEventTypeSessionResumed:
			status = fse.LifecycleStatusRunning
		case recording.FactoryEventTypeSessionCompleted:
			var payload factorydefinitions.FactorySessionCompletedEventPayload
			if event.DecodePayload(&payload) == nil {
				if next := normalizeLegacyLifecycleStatus(string(payload.FinalStatus)); next != "" {
					status = next
				}
			}
		}
	}
	return status
}

func normalizeLegacyLifecycleStatus(value string) fse.LifecycleStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "COMPLETED", "SUCCEEDED":
		return fse.LifecycleStatusSucceeded
	case "FAILED":
		return fse.LifecycleStatusFailed
	case "RUNNING":
		return fse.LifecycleStatusRunning
	case "PAUSED":
		return fse.LifecycleStatusPaused
	case "CANCELED":
		return fse.LifecycleStatusCanceled
	case "TIMED_OUT":
		return fse.LifecycleStatusTimedOut
	case "INTERRUPTED":
		return fse.LifecycleStatusInterrupted
	case "TERMINATED":
		return fse.LifecycleStatusTerminated
	default:
		return ""
	}
}

func legacyResultStatus(
	events []recording.FactoryEvent,
	state recording.FactoryWorldState,
	status fse.LifecycleStatus,
) fse.ResultStatus {
	if bracket := state.SessionBracket; bracket != nil && strings.TrimSpace(bracket.ResultStatus) != "" {
		return fse.ResultStatus(bracket.ResultStatus)
	}
	if runtime := state.JavaScriptRuntime; runtime != nil && strings.TrimSpace(runtime.ResultStatus) != "" {
		return fse.ResultStatus(runtime.ResultStatus)
	}
	var observed fse.ResultStatus
	for _, event := range events {
		switch event.Type {
		case recording.FactoryEventTypeSessionResultUpdated:
			var payload factorydefinitions.FactorySessionResultUpdatedEventPayload
			if event.DecodePayload(&payload) == nil {
				observed = fse.ResultStatus(payload.ResultStatus)
			}
		case recording.FactoryEventTypeSessionCompleted:
			var payload factorydefinitions.FactorySessionCompletedEventPayload
			if event.DecodePayload(&payload) == nil && payload.ResultStatus != nil {
				observed = fse.ResultStatus(*payload.ResultStatus)
			}
		}
	}
	if observed != "" {
		return observed
	}
	switch status {
	case fse.LifecycleStatusSucceeded:
		return fse.ResultStatusFinal
	case fse.LifecycleStatusFailed:
		if len(state.FailedDispatches) > 0 || len(state.FailureDetailsByWorkID) > 0 {
			return fse.ResultStatusUnavailable
		}
		return fse.ResultStatusUnavailable
	default:
		return fse.ResultStatusNotReady
	}
}

func legacyArtifactIDs(state recording.FactoryWorldState) []string {
	if state.SessionBracket == nil {
		return nil
	}
	return append([]string(nil), state.SessionBracket.ArtifactIDs...)
}

func legacyOverallFailure(state recording.FactoryWorldState) *workers.FailureDetail {
	if state.SessionBracket != nil && state.SessionBracket.FailureDetail != nil {
		return workers.CloneFailureDetail(state.SessionBracket.FailureDetail)
	}
	ids := make([]string, 0, len(state.FailureDetailsByWorkID))
	for id := range state.FailureDetailsByWorkID {
		ids = append(ids, id)
	}
	sortStrings(ids)
	for _, id := range ids {
		if detail := state.FailureDetailsByWorkID[id].FailureDetail; detail != nil {
			return workers.CloneFailureDetail(detail)
		}
	}
	return nil
}

func enrichLegacyFailureDetails(state *recording.FactoryWorldState) {
	if state == nil {
		return
	}
	if state.FailureDetailsByWorkID == nil {
		state.FailureDetailsByWorkID = make(map[string]recording.FactoryWorldFailureDetail)
	}
	completions := state.FailedDispatches
	if len(completions) == 0 {
		for _, completion := range state.CompletedDispatches {
			if completion.Result.Outcome == string(workers.OutcomeFailed) {
				completions = append(completions, completion)
			}
		}
	}
	for _, completion := range completions {
		if completion.Result.Outcome != string(workers.OutcomeFailed) {
			continue
		}
		workIDs := make([]string, 0, len(completion.WorkItemIDs)+len(completion.OutputWorkItems)+len(completion.InputWorkItems))
		workIDs = append(workIDs, completion.WorkItemIDs...)
		for _, item := range completion.OutputWorkItems {
			workIDs = append(workIDs, item.ID)
		}
		for _, item := range completion.InputWorkItems {
			workIDs = append(workIDs, item.ID)
		}
		workIDs = uniqueSortedStrings(workIDs)
		for _, workID := range workIDs {
			if strings.TrimSpace(workID) == "" {
				continue
			}
			item, ok := state.WorkItemsByID[workID]
			if !ok {
				for _, candidate := range completion.OutputWorkItems {
					if candidate.ID == workID {
						item, ok = candidate, true
						break
					}
				}
			}
			if !ok {
				for _, candidate := range completion.InputWorkItems {
					if candidate.ID == workID {
						item, ok = candidate, true
						break
					}
				}
			}
			if !ok {
				continue
			}
			detail := state.FailureDetailsByWorkID[workID]
			detail.DispatchID = firstNonEmpty(detail.DispatchID, completion.DispatchID)
			detail.TransitionID = firstNonEmpty(detail.TransitionID, completion.TransitionID)
			if detail.WorkItem.ID == "" {
				detail.WorkItem = item
			}
			if detail.FailureDetail == nil {
				detail.FailureDetail = legacyFailureDetail(completion.Result)
			}
			if detail.ArtifactVerification == nil {
				detail.ArtifactVerification = completion.Result.ArtifactVerification.Clone()
			}
			state.FailureDetailsByWorkID[workID] = detail
		}
	}
}

func legacyFailureDetail(result factorydefinitions.WorkstationResult) *workers.FailureDetail {
	if result.FailureDetail != nil {
		return workers.CloneFailureDetail(result.FailureDetail)
	}
	reason := workers.WorkFailureTypeUnknown
	if result.FailureMetadata != nil && result.FailureMetadata.Type != "" {
		reason = result.FailureMetadata.Type
	}
	message := strings.TrimSpace(result.Error)
	if message == "" {
		message = strings.TrimSpace(result.Feedback)
	}
	if message == "" {
		message = "recorded worker failure"
	}
	return &workers.FailureDetail{Reason: reason, Message: safeLegacyFailureMessage(message)}
}

func safeLegacyFailureMessage(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) > 512 {
		return value[:512]
	}
	return value
}

func legacyStringPointer(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sortStrings(result)
	return result
}

func sortStrings(values []string) {
	for index := 1; index < len(values); index++ {
		value := values[index]
		position := index
		for position > 0 && values[position-1] > value {
			values[position] = values[position-1]
			position--
		}
		values[position] = value
	}
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
		Usage:        fse.EmptySessionUsage(),
		ArtifactRefs: refs, ArtifactCount: len(refs), Links: fse.InspectionLinksForSession(value.Session.ID, true),
	}
	if value.Result != nil {
		session.ResultSummary = &fse.ResultSummary{ResultStatus: string(result.ResultStatus)}
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
	}
	if projected.SessionStatus == "COMPLETED" {
		projected.SessionStatus = fse.LifecycleStatusSucceeded
	}
	if result == nil {
		return projected
	}
	projected.ResultStatus = fse.ResultStatus(result.Status)
	projected.Mode = fse.ResultMode(result.Mode)
	projected.PrimaryResult = append(json.RawMessage(nil), result.PrimaryResult...)
	projected.ArtifactIDs = append([]string(nil), result.ArtifactIDs...)
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
