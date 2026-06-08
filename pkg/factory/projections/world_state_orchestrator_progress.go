package projections

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func (r *factoryWorldReducer) applyOrchestratorProgressEvent(event factoryapi.FactoryEvent) (bool, error) {
	switch event.Type {
	case factoryapi.FactoryEventTypeOrchestratorPhaseChanged:
		return true, r.applyOrchestratorPhaseChangedEvent(event)
	case factoryapi.FactoryEventTypeOrchestratorCheckpointWritten:
		return true, r.applyOrchestratorCheckpointWrittenEvent(event)
	default:
		return false, nil
	}
}

func (r *factoryWorldReducer) applyOrchestratorPhaseChangedEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsOrchestratorPhaseChangedEventPayload()
	if err != nil {
		return err
	}
	runtime := r.ensureJavaScriptRuntime()
	currentPhase := stringValue(event.Context.PhaseName)
	if currentPhase == "" {
		currentPhase = stringValue(event.Context.PhaseId)
	}
	runtime.Phase = currentPhase
	runtime.Phases = appendOrchestratorPhaseHistory(
		runtime.Phases,
		stringValue(payload.PreviousPhaseName),
		stringValue(payload.PreviousPhaseId),
		currentPhase,
	)
	runtime.ScriptStatus = orchestratorPhaseStatusToScriptStatus(payload.PhaseStatus)
	return nil
}

func (r *factoryWorldReducer) applyOrchestratorCheckpointWrittenEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsOrchestratorCheckpointWrittenEventPayload()
	if err != nil {
		return err
	}
	checkpointID := stringValue(event.Context.CheckpointId)
	if checkpointID == "" && payload.ArtifactRef != nil {
		checkpointID = payload.ArtifactRef.Id
	}
	checkpoint := interfaces.FactorySessionJavaScriptCheckpointRef{
		ID:                 checkpointID,
		Label:              payload.Label,
		ResumabilityStatus: string(payload.ResumabilityStatus),
		Warnings:           projectOrchestratorCheckpointWarnings(payload.Warnings),
	}
	if payload.Timestamp != nil {
		checkpoint.Timestamp = payload.Timestamp.UTC()
	}
	if payload.ArtifactRef != nil {
		checkpoint.ArtifactRef = &interfaces.JavaScriptCheckpointArtifactRef{
			ID:         payload.ArtifactRef.Id,
			Kind:       string(payload.ArtifactRef.Kind),
			Visibility: string(payload.ArtifactRef.Visibility),
		}
		if payload.ArtifactRef.ContentHash != nil {
			checkpoint.ArtifactRef.ContentHash = *payload.ArtifactRef.ContentHash
		}
		if payload.ArtifactRef.SizeBytes != nil {
			checkpoint.ArtifactRef.SizeBytes = *payload.ArtifactRef.SizeBytes
		}
	}
	r.stateValue.JavaScriptCheckpoints = append(r.stateValue.JavaScriptCheckpoints, checkpoint)
	if runtime := r.ensureJavaScriptRuntime(); runtime != nil {
		runtime.Checkpoints = append(runtime.Checkpoints, checkpoint)
	}
	return nil
}

func appendOrchestratorPhaseHistory(phases []string, previousName, previousID, currentPhase string) []string {
	if previous := orchestratorPhaseHistoryName(previousName, previousID); previous != "" {
		phases = appendPhaseHistoryEntry(phases, previous)
	}
	if current := strings.TrimSpace(currentPhase); current != "" {
		phases = appendPhaseHistoryEntry(phases, current)
	}
	return phases
}

func orchestratorPhaseHistoryName(name, id string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(id)
}

func appendPhaseHistoryEntry(phases []string, phase string) []string {
	if phase == "" {
		return phases
	}
	if len(phases) > 0 && phases[len(phases)-1] == phase {
		return phases
	}
	return append(phases, phase)
}

func orchestratorPhaseStatusToScriptStatus(status factoryapi.OrchestratorPhaseStatus) string {
	switch status {
	case factoryapi.ACTIVE:
		return string(factoryapi.FactorySessionJavaScriptScriptStatusRUNNING)
	case factoryapi.COMPLETED:
		return string(factoryapi.FactorySessionJavaScriptScriptStatusFINISHED)
	case factoryapi.SKIPPED:
		return "SKIPPED"
	default:
		return string(status)
	}
}

func projectOrchestratorCheckpointWarnings(
	warnings *[]factoryapi.FactoryDispatchWarning,
) []interfaces.FactorySessionDispatchWarning {
	if warnings == nil || len(*warnings) == 0 {
		return nil
	}
	projected := make([]interfaces.FactorySessionDispatchWarning, 0, len(*warnings))
	for _, warning := range *warnings {
		projected = append(projected, interfaces.FactorySessionDispatchWarning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}
	return projected
}
