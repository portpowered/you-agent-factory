package projections

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func (r *factoryWorldReducer) applyDispatchLifecycleEvent(event factoryapi.FactoryEvent) (bool, error) {
	switch event.Type {
	case factoryapi.FactoryEventTypeDispatchQueued:
		return true, r.applyDispatchQueuedEvent(event)
	case factoryapi.FactoryEventTypeDispatchInterrupted:
		return true, r.applyDispatchInterruptedEvent(event)
	case factoryapi.FactoryEventTypeDispatchReconciled:
		return true, r.applyDispatchReconciledEvent(event)
	default:
		return false, nil
	}
}

func (r *factoryWorldReducer) applyDispatchQueuedEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchQueuedEventPayload()
	if err != nil {
		return err
	}
	dispatchID := stringValue(event.Context.DispatchId)
	if dispatchID == "" {
		return nil
	}
	state := interfaces.FactorySessionDispatchState{
		ID:           dispatchID,
		DispatchKind: string(payload.DispatchKind),
		Status:       string(factoryapi.FactoryDispatchStatusQUEUED),
		Phase:        dispatchLifecyclePhase(event.Context),
		Label:        stringValue(payload.Label),
		RunnerID:     stringValue(payload.RunnerId),
		Model:        stringValue(payload.Model),
		Provider:     stringValue(payload.Provider),
		PromptDigest: stringValue(payload.PromptDigest),
		SchemaDigest: stringValue(payload.SchemaDigest),
		RelatedWorkIDs: cloneStringSlice(sliceValue(payload.InputWorkIds)),
	}
	if payload.DispatchKind == factoryapi.FactoryDispatchKindPETRITRANSITION {
		state.Petri = &interfaces.FactorySessionDispatchPetriState{
			TransitionID: dispatchID,
		}
	} else {
		state.JavaScript = &interfaces.FactorySessionDispatchJavaScriptState{
			TaskKind:  javaScriptTaskKindFromDispatchKind(payload.DispatchKind),
			TaskLabel: stringValue(payload.Label),
		}
	}
	r.upsertJavaScriptDispatch(state)
	return nil
}

func (r *factoryWorldReducer) applyDispatchInterruptedEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchInterruptedEventPayload()
	if err != nil {
		return err
	}
	dispatchID := stringValue(event.Context.DispatchId)
	if dispatchID == "" {
		return nil
	}
	state := interfaces.FactorySessionDispatchState{
		ID:     dispatchID,
		Status: string(payload.ObservedStatus),
		Phase:  dispatchLifecyclePhase(event.Context),
	}
	if payload.Reason != "" {
		state.FailureDetail = &interfaces.FactorySessionDispatchFailureDetail{
			Message: payload.Reason,
		}
	}
	r.upsertJavaScriptDispatch(state)
	return nil
}

func (r *factoryWorldReducer) applyDispatchReconciledEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchReconciledEventPayload()
	if err != nil {
		return err
	}
	dispatchID := stringValue(event.Context.DispatchId)
	if dispatchID == "" {
		return nil
	}
	state := interfaces.FactorySessionDispatchState{
		ID:           dispatchID,
		Status:       string(payload.ReconciledStatus),
		Phase:        dispatchLifecyclePhase(event.Context),
		ArtifactIDs:  cloneStringSlice(sliceValue(payload.ArtifactIds)),
	}
	if payload.Usage != nil {
		state.Usage = &interfaces.FactorySessionDispatchUsage{
			InputTokens:    int64Value(payload.Usage.InputTokens),
			OutputTokens:   int64Value(payload.Usage.OutputTokens),
			TotalTokens:    int64Value(payload.Usage.TotalTokens),
			CostUSD:        float64Value(payload.Usage.CostUsd),
			DurationMillis: int64Value(payload.Usage.DurationMillis),
			RetryCount:     int32Value(payload.Usage.RetryCount),
		}
	}
	if payload.FailureDetail != nil {
		state.FailureDetail = &interfaces.FactorySessionDispatchFailureDetail{
			Reason:     stringValue(payload.FailureDetail.Reason),
			Message:    stringValue(payload.FailureDetail.Message),
			ErrorClass: stringValue(payload.FailureDetail.ErrorClass),
		}
	}
	if payload.ResultArtifactRef != nil {
		state.ArtifactIDs = appendUnique(state.ArtifactIDs, payload.ResultArtifactRef.Id)
	}
	r.upsertJavaScriptDispatch(state)
	return nil
}

func (r *factoryWorldReducer) upsertJavaScriptDispatch(state interfaces.FactorySessionDispatchState) {
	if strings.TrimSpace(state.ID) == "" {
		return
	}
	runtime := r.ensureJavaScriptRuntime()
	for index, existing := range runtime.Dispatches {
		if existing.ID != state.ID {
			continue
		}
		runtime.Dispatches[index] = mergeJavaScriptDispatchState(existing, state)
		r.recountJavaScriptDispatchTotals()
		return
	}
	runtime.Dispatches = append(runtime.Dispatches, state)
	r.recountJavaScriptDispatchTotals()
}

// pkgmaintcheck:ignore-cyclomatic-complexity dispatch replay merge keeps JavaScript dispatch field updates together for queue/interrupt/reconcile states.
func mergeJavaScriptDispatchState(
	existing interfaces.FactorySessionDispatchState,
	incoming interfaces.FactorySessionDispatchState,
) interfaces.FactorySessionDispatchState {
	merged := existing
	if incoming.DispatchKind != "" {
		merged.DispatchKind = incoming.DispatchKind
	}
	if incoming.Status != "" {
		merged.Status = incoming.Status
	}
	if incoming.Phase != "" {
		merged.Phase = incoming.Phase
	}
	if incoming.Label != "" {
		merged.Label = incoming.Label
	}
	if incoming.RunnerID != "" {
		merged.RunnerID = incoming.RunnerID
	}
	if incoming.Model != "" {
		merged.Model = incoming.Model
	}
	if incoming.Provider != "" {
		merged.Provider = incoming.Provider
	}
	if incoming.PromptDigest != "" {
		merged.PromptDigest = incoming.PromptDigest
	}
	if incoming.SchemaDigest != "" {
		merged.SchemaDigest = incoming.SchemaDigest
	}
	if len(incoming.RelatedWorkIDs) > 0 {
		merged.RelatedWorkIDs = cloneStringSlice(incoming.RelatedWorkIDs)
	}
	if len(incoming.ArtifactIDs) > 0 {
		merged.ArtifactIDs = cloneStringSlice(merged.ArtifactIDs)
		for _, artifactID := range incoming.ArtifactIDs {
			merged.ArtifactIDs = appendUnique(merged.ArtifactIDs, artifactID)
		}
	}
	if incoming.Usage != nil {
		merged.Usage = incoming.Usage
	}
	if incoming.FailureDetail != nil {
		merged.FailureDetail = incoming.FailureDetail
	}
	if incoming.Petri != nil {
		merged.Petri = incoming.Petri
	}
	if incoming.JavaScript != nil {
		merged.JavaScript = incoming.JavaScript
	}
	return merged
}

func (r *factoryWorldReducer) recountJavaScriptDispatchTotals() {
	runtime := r.ensureJavaScriptRuntime()
	var queued, running, completed int
	for _, dispatch := range runtime.Dispatches {
		switch factoryapi.FactoryDispatchStatus(strings.TrimSpace(dispatch.Status)) {
		case factoryapi.FactoryDispatchStatusQUEUED:
			queued++
		case factoryapi.FactoryDispatchStatusRUNNING:
			running++
		case factoryapi.FactoryDispatchStatusCOMPLETED:
			completed++
		}
	}
	runtime.QueuedDispatches = queued
	runtime.RunningDispatches = running
	runtime.CompletedDispatches = completed
}

func dispatchLifecyclePhase(context factoryapi.FactoryEventContext) string {
	if phase := stringValue(context.PhaseName); phase != "" {
		return phase
	}
	return stringValue(context.PhaseId)
}

func javaScriptTaskKindFromDispatchKind(kind factoryapi.FactoryDispatchKind) string {
	switch kind {
	case factoryapi.FactoryDispatchKindJAVASCRIPTVERIFY:
		return "VERIFY"
	case factoryapi.FactoryDispatchKindJAVASCRIPTSYNTHESIZE:
		return "SYNTHESIZE"
	case factoryapi.FactoryDispatchKindJAVASCRIPTTOOL:
		return "TOOL"
	case factoryapi.FactoryDispatchKindJAVASCRIPTSCRIPT:
		return "SCRIPT"
	case factoryapi.FactoryDispatchKindJAVASCRIPTSYSTEM:
		return "SYSTEM"
	default:
		return "AGENT"
	}
}

func float64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func int32Value(value *int32) int {
	if value == nil {
		return 0
	}
	return int(*value)
}
