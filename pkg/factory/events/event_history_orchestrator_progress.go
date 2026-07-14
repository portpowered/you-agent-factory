package events

import (
	"fmt"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	eventIDOrchestratorPhaseChangedPrefix      = "factory-event/orchestrator-phase-changed"
	eventIDOrchestratorCheckpointWrittenPrefix = "factory-event/orchestrator-checkpoint-written"
)

// OrchestratorPhaseChangedInput carries replay-safe facts for ORCHESTRATOR_PHASE_CHANGED.
type OrchestratorPhaseChangedInput struct {
	SessionID           string
	OrchestratorKind    factoryapi.FactoryOrchestratorKind
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	Source              string
	Tick                int
	PreviousPhaseID     string
	PreviousPhaseName   string
	PhaseStatus         factoryapi.OrchestratorPhaseStatus
	StartedAt           *time.Time
	CompletedAt         *time.Time
	ProgressSummary     string
}

// OrchestratorCheckpointWrittenInput carries replay-safe facts for ORCHESTRATOR_CHECKPOINT_WRITTEN.
type OrchestratorCheckpointWrittenInput struct {
	SessionID             string
	OrchestratorKind      factoryapi.FactoryOrchestratorKind
	OrchestratorDialect   string
	PhaseID               string
	PhaseName             string
	CheckpointID          string
	Source                string
	Tick                  int
	Label                 string
	Timestamp             *time.Time
	SourceHash            string
	RuntimeSnapshotDigest string
	ArtifactRef           *factoryapi.FactoryArtifactRef
	ResumabilityStatus    factoryapi.CheckpointResumabilityStatus
	Warnings              []factoryapi.FactoryDispatchWarning
}

// RecordOrchestratorPhaseChanged records a canonical orchestrator workflow phase transition.
func (h *FactoryEventHistory) RecordOrchestratorPhaseChanged(input OrchestratorPhaseChangedInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || input.PhaseStatus == "" {
		return
	}
	eventTime = canonicalOrchestratorEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.orchestratorProgressContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		"",
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.OrchestratorPhaseChangedEventPayload{
		PhaseStatus: input.PhaseStatus,
	}
	if previousPhaseID := strings.TrimSpace(input.PreviousPhaseID); previousPhaseID != "" {
		payload.PreviousPhaseId = &previousPhaseID
	}
	if previousPhaseName := strings.TrimSpace(input.PreviousPhaseName); previousPhaseName != "" {
		payload.PreviousPhaseName = &previousPhaseName
	}
	if input.StartedAt != nil {
		startedAt := canonicalOrchestratorEventTime(*input.StartedAt)
		payload.StartedAt = &startedAt
	}
	if input.CompletedAt != nil {
		completedAt := canonicalOrchestratorEventTime(*input.CompletedAt)
		payload.CompletedAt = &completedAt
	}
	if summary := strings.TrimSpace(input.ProgressSummary); summary != "" {
		payload.ProgressSummary = &summary
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeOrchestratorPhaseChanged,
		fmt.Sprintf("%s/%d", eventIDOrchestratorPhaseChangedPrefix, sequence),
		context,
		payload,
	))
}

// RecordOrchestratorCheckpointWritten records a canonical orchestrator checkpoint reference.
func (h *FactoryEventHistory) RecordOrchestratorCheckpointWritten(input OrchestratorCheckpointWrittenInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.Label) == "" || input.ResumabilityStatus == "" {
		return
	}
	eventTime = canonicalOrchestratorEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.orchestratorProgressContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.CheckpointID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.OrchestratorCheckpointWrittenEventPayload{
		Label:              input.Label,
		ResumabilityStatus: input.ResumabilityStatus,
	}
	if input.Timestamp != nil {
		timestamp := canonicalOrchestratorEventTime(*input.Timestamp)
		payload.Timestamp = &timestamp
	}
	if sourceHash := strings.TrimSpace(input.SourceHash); sourceHash != "" {
		payload.SourceHash = &sourceHash
	}
	if digest := strings.TrimSpace(input.RuntimeSnapshotDigest); digest != "" {
		payload.RuntimeSnapshotDigest = &digest
	}
	if input.ArtifactRef != nil {
		artifactRef := *input.ArtifactRef
		payload.ArtifactRef = &artifactRef
	}
	if len(input.Warnings) > 0 {
		warnings := append([]factoryapi.FactoryDispatchWarning(nil), input.Warnings...)
		payload.Warnings = &warnings
	}
	checkpointID := strings.TrimSpace(input.CheckpointID)
	eventID := fmt.Sprintf("%s/%d", eventIDOrchestratorCheckpointWrittenPrefix, sequence)
	if checkpointID != "" {
		eventID = fmt.Sprintf("%s/%s", eventIDOrchestratorCheckpointWrittenPrefix, checkpointID)
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeOrchestratorCheckpointWritten,
		eventID,
		context,
		payload,
	))
}

func (h *FactoryEventHistory) orchestratorProgressContext(
	sessionID string,
	orchestratorKind factoryapi.FactoryOrchestratorKind,
	orchestratorDialect string,
	phaseID string,
	phaseName string,
	checkpointID string,
	source string,
	tick int,
	eventTime time.Time,
	sessionSequence int,
) factoryapi.FactoryEventContext {
	context := h.sessionLifecycleContext(sessionID, orchestratorKind, orchestratorDialect, source, tick, eventTime, sessionSequence)
	if phaseID := strings.TrimSpace(phaseID); phaseID != "" {
		context.PhaseId = &phaseID
	}
	if phaseName := strings.TrimSpace(phaseName); phaseName != "" {
		context.PhaseName = &phaseName
	}
	if checkpointID := strings.TrimSpace(checkpointID); checkpointID != "" {
		context.CheckpointId = &checkpointID
	}
	return context
}

func canonicalOrchestratorEventTime(eventTime time.Time) time.Time {
	if eventTime.IsZero() {
		return eventTime
	}
	return eventTime.UTC()
}
