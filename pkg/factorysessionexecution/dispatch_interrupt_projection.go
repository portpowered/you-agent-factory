package factorysessionexecution

import (
	"encoding/json"
	"strings"
)

type interruptedDispatchPreservation struct {
	summary              DispatchSummary
	statusTransitions    []DispatchStatus
	javaScriptProjection *DispatchJavaScriptProjection
}

func snapshotInterruptedDispatches(state *runtimeSessionState) map[string]interruptedDispatchPreservation {
	if state == nil || len(state.dispatches) == 0 {
		return nil
	}
	preserved := make(map[string]interruptedDispatchPreservation)
	for _, dispatch := range state.dispatches {
		if dispatch.Status != DispatchStatusInterrupted {
			continue
		}
		preservation := interruptedDispatchPreservation{
			summary:           cloneDispatchSummary(dispatch),
			statusTransitions: cloneDispatchStatusSlice(state.dispatchStatusTransitions[dispatch.ID]),
		}
		if js, ok := state.dispatchJavaScript[dispatch.ID]; ok {
			projection := js
			preservation.javaScriptProjection = &projection
		}
		preserved[dispatch.ID] = preservation
	}
	if len(preserved) == 0 {
		return nil
	}
	return preserved
}

func restoreInterruptedDispatchResultSuppression(
	state *runtimeSessionState,
	preserved map[string]interruptedDispatchPreservation,
) {
	if state == nil || len(preserved) == 0 {
		return
	}
	projectedByID := make(map[string]DispatchSummary, len(state.dispatches))
	for _, dispatch := range state.dispatches {
		projectedByID[dispatch.ID] = dispatch
	}
	for index, dispatch := range state.dispatches {
		preservation, ok := preserved[dispatch.ID]
		if !ok {
			continue
		}
		state.dispatches[index] = enrichInterruptedDispatchDiagnostics(
			preservation.summary,
			projectedByID[dispatch.ID],
		)
	}
	for dispatchID, preservation := range preserved {
		if _, ok := projectedByID[dispatchID]; ok {
			continue
		}
		state.dispatches = append(state.dispatches, cloneDispatchSummary(preservation.summary))
	}
	if state.dispatchStatusTransitions == nil {
		state.dispatchStatusTransitions = make(map[string][]DispatchStatus, len(preserved))
	}
	for dispatchID, preservation := range preserved {
		if len(preservation.statusTransitions) > 0 {
			state.dispatchStatusTransitions[dispatchID] = cloneDispatchStatusSlice(preservation.statusTransitions)
		}
		if preservation.javaScriptProjection != nil {
			if state.dispatchJavaScript == nil {
				state.dispatchJavaScript = make(map[string]DispatchJavaScriptProjection)
			}
			state.dispatchJavaScript[dispatchID] = *preservation.javaScriptProjection
		}
	}
	state.artifacts = filterArtifactsSuppressingInterruptedLateResults(state.artifacts, preserved)
	recalculateSessionProgressFromDispatches(state)
}

func enrichInterruptedDispatchDiagnostics(preserved, projected DispatchSummary) DispatchSummary {
	enriched := cloneDispatchSummary(preserved)
	if enriched.Label == "" {
		enriched.Label = projected.Label
	}
	if enriched.RunnerID == "" {
		enriched.RunnerID = projected.RunnerID
	}
	if enriched.Model == "" {
		enriched.Model = projected.Model
	}
	if enriched.Provider == "" {
		enriched.Provider = projected.Provider
	}
	if len(enriched.ProviderSessionRefs) == 0 && len(projected.ProviderSessionRefs) > 0 {
		enriched.ProviderSessionRefs = cloneProviderSessionRefs(projected.ProviderSessionRefs)
	}
	return enriched
}

func filterArtifactsSuppressingInterruptedLateResults(
	artifacts []ArtifactSummary,
	preserved map[string]interruptedDispatchPreservation,
) []ArtifactSummary {
	if len(artifacts) == 0 || len(preserved) == 0 {
		return artifacts
	}
	filtered := make([]ArtifactSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		dispatchID := strings.TrimSpace(artifact.DispatchID)
		if dispatchID == "" {
			filtered = append(filtered, artifact)
			continue
		}
		if _, interrupted := preserved[dispatchID]; interrupted && artifact.Kind == "CHILD_OUTPUT" {
			continue
		}
		filtered = append(filtered, artifact)
	}
	return filtered
}

func recalculateSessionProgressFromDispatches(state *runtimeSessionState) {
	if state == nil {
		return
	}
	phaseCount := 0
	if state.session.Progress != nil {
		phaseCount = state.session.Progress.PhaseCount
	}
	progress := progressCountsFromDispatches(state.dispatches, phaseCount)
	state.session.Progress = &progress
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.session.ArtifactCount = len(state.session.ArtifactRefs)
}

func extractDispatchInterruptedEvents(events []json.RawMessage) []json.RawMessage {
	if len(events) == 0 {
		return nil
	}
	interrupted := make([]json.RawMessage, 0)
	for _, raw := range events {
		var envelope factoryEventEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if strings.TrimSpace(envelope.Type) != "DISPATCH_INTERRUPTED" {
			continue
		}
		interrupted = append(interrupted, append(json.RawMessage(nil), raw...))
	}
	return interrupted
}

func mergePreservedDispatchInterruptedEvents(projected, preserved []json.RawMessage) []json.RawMessage {
	if len(preserved) == 0 {
		return projected
	}
	merged := make([]json.RawMessage, 0, len(projected)+len(preserved))
	seen := make(map[string]struct{}, len(preserved))
	for _, raw := range projected {
		var envelope factoryEventEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			merged = append(merged, raw)
			continue
		}
		if strings.TrimSpace(envelope.Type) == "DISPATCH_INTERRUPTED" {
			seen[eventIdentityKey(raw)] = struct{}{}
		}
		merged = append(merged, raw)
	}
	for _, raw := range preserved {
		key := eventIdentityKey(raw)
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, raw)
		seen[key] = struct{}{}
	}
	return merged
}

func eventIdentityKey(raw json.RawMessage) string {
	var envelope struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return string(raw)
	}
	if id := strings.TrimSpace(envelope.ID); id != "" {
		return id
	}
	return strings.TrimSpace(envelope.Type)
}

func cloneDispatchStatusSlice(transitions []DispatchStatus) []DispatchStatus {
	if len(transitions) == 0 {
		return nil
	}
	return append([]DispatchStatus(nil), transitions...)
}

func cloneProviderSessionRefs(refs []ProviderSessionRef) []ProviderSessionRef {
	if len(refs) == 0 {
		return nil
	}
	return append([]ProviderSessionRef(nil), refs...)
}

func cloneDispatchSummary(dispatch DispatchSummary) DispatchSummary {
	cloned := dispatch
	if len(dispatch.OutputArtifactIDs) > 0 {
		cloned.OutputArtifactIDs = append([]string(nil), dispatch.OutputArtifactIDs...)
	}
	if len(dispatch.ProviderSessionRefs) > 0 {
		cloned.ProviderSessionRefs = cloneProviderSessionRefs(dispatch.ProviderSessionRefs)
	}
	if dispatch.FailureDetail != nil {
		detail := *dispatch.FailureDetail
		cloned.FailureDetail = &detail
	}
	return cloned
}
