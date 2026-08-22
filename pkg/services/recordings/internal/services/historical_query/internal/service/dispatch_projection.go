package service

import (
	"errors"
	"math"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// projectHistoricalDispatches folds canonical dispatch lifecycle facts while
// preserving first-seen order and recorded Worker Session associations.
func projectHistoricalDispatches(
	identity recordings.HistoricalRecordingIdentity,
	events []recordings.CanonicalEvent,
) ([]recordings.HistoricalDispatch, error) {
	byID := make(map[string]int)
	dispatches := make([]recordings.HistoricalDispatch, 0)
	for _, event := range events {
		legacy := canonical.FactoryEventFromCanonical(event)
		if legacy.Type == factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc {
			if err := applyHistoricalWorkerSessionAssociation(identity, event, legacy, byID, dispatches); err != nil {
				return nil, err
			}
			continue
		}
		status, kind, transitionID, usage, applies, err := historicalDispatchStatus(legacy)
		if err != nil {
			return nil, historicalDispatchCorrupt(identity, event, err)
		}
		if !applies {
			continue
		}
		dispatchID, err := historicalDispatchID(legacy)
		if err != nil {
			return nil, historicalDispatchCorrupt(identity, event, err)
		}
		index, exists := byID[dispatchID]
		if !exists {
			byID[dispatchID] = len(dispatches)
			dispatches = append(dispatches, recordings.HistoricalDispatch{
				ID: dispatchID, Status: status, DispatchKind: kind, TransitionID: transitionID,
				Usage:       usage,
				FirstCursor: event.Cursor, LastCursor: event.Cursor,
			})
			continue
		}
		dispatches[index].Status = status
		if kind != "" {
			dispatches[index].DispatchKind = kind
		}
		if transitionID != "" {
			dispatches[index].TransitionID = transitionID
		}
		if usage != nil {
			dispatches[index].Usage = usage
		}
		dispatches[index].LastCursor = event.Cursor
	}
	return append([]recordings.HistoricalDispatch(nil), dispatches...), nil
}

func applyHistoricalWorkerSessionAssociation(
	identity recordings.HistoricalRecordingIdentity,
	event recordings.CanonicalEvent,
	legacy factorydefinitions.FactoryEvent,
	byID map[string]int,
	dispatches []recordings.HistoricalDispatch,
) error {
	dispatchID, err := historicalDispatchID(legacy)
	if err != nil {
		return historicalDispatchCorrupt(identity, event, err)
	}
	index, exists := byID[dispatchID]
	if !exists {
		return historicalDispatchCorrupt(identity, event, errors.New("association has no preceding dispatch"))
	}
	var payload factorydefinitions.DispatchWorkerSessionAssociationEventPayload
	if err := legacy.DecodePayload(&payload); err != nil || strings.TrimSpace(payload.WorkerSessionID) == "" {
		return historicalDispatchCorrupt(identity, event, errors.New("invalid dispatch worker session association"))
	}
	association := recordings.HistoricalDispatchWorkerSessionAssociation{
		ID: event.ID, WorkerSessionID: payload.WorkerSessionID,
		RequestID: historicalRequestID(legacy), Cursor: event.Cursor,
	}
	existing := dispatches[index].Association
	if existing == nil {
		dispatches[index].Association = &association
		return nil
	}
	if existing.WorkerSessionID == association.WorkerSessionID && existing.RequestID == association.RequestID {
		return nil
	}
	return historicalDispatchCorrupt(identity, event, errors.New("contradictory dispatch worker session association"))
}

func historicalDispatchStatus(event factorydefinitions.FactoryEvent) (recordings.FactoryDispatchStatus, recordings.FactoryDispatchKind, string, *recordings.FactoryDispatchUsage, bool, error) {
	switch event.Type {
	case factorydefinitions.FactoryEventTypeDispatchRequest:
		var payload factorydefinitions.DispatchRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil || strings.TrimSpace(payload.TransitionID) == "" {
			return "", "", "", nil, true, errors.New("invalid dispatch request")
		}
		return recordings.FactoryDispatchStatusRunning, recordings.FactoryDispatchKindPetriTransition, payload.TransitionID, nil, true, nil
	case factorydefinitions.FactoryEventTypeDispatchResponse:
		return dispatchResponseStatus(event)
	case factorydefinitions.FactoryEventTypeDispatchQueued:
		var payload factorydefinitions.DispatchQueuedEventPayload
		if err := event.DecodePayload(&payload); err != nil || !validDispatchKind(payload.DispatchKind) {
			return "", "", "", nil, true, errors.New("invalid queued dispatch")
		}
		return recordings.FactoryDispatchStatusQueued, recordings.FactoryDispatchKind(payload.DispatchKind), "", nil, true, nil
	case factorydefinitions.FactoryEventTypeDispatchInterrupted:
		var payload factorydefinitions.DispatchInterruptedEventPayload
		if err := event.DecodePayload(&payload); err != nil || !validDispatchStatus(payload.ObservedStatus) {
			return "", "", "", nil, true, errors.New("invalid interrupted dispatch")
		}
		return recordings.FactoryDispatchStatusInterrupted, "", "", nil, true, nil
	case factorydefinitions.FactoryEventTypeDispatchReconciled:
		var payload factorydefinitions.DispatchReconciledEventPayload
		if err := event.DecodePayload(&payload); err != nil || !validDispatchStatus(payload.ReconciledStatus) {
			return "", "", "", nil, true, errors.New("invalid reconciled dispatch")
		}
		// JavaScript usage is intentionally not copied here. Its established
		// reconciliation projection must retain its existing serialized output.
		return recordings.FactoryDispatchStatus(payload.ReconciledStatus), "", "", nil, true, nil
	default:
		return "", "", "", nil, false, nil
	}
}

func dispatchResponseStatus(event factorydefinitions.FactoryEvent) (recordings.FactoryDispatchStatus, recordings.FactoryDispatchKind, string, *recordings.FactoryDispatchUsage, bool, error) {
	var payload workers.DispatchResponseEventPayload
	if err := event.DecodePayload(&payload); err != nil || strings.TrimSpace(payload.TransitionID) == "" {
		return "", "", "", nil, true, errors.New("invalid dispatch response")
	}
	usage := historicalDispatchUsage(payload.Usage, payload.DurationMillis)
	switch payload.Outcome {
	case workers.OutcomeAccepted, workers.OutcomeContinue, workers.OutcomeRejected:
		return recordings.FactoryDispatchStatusCompleted, recordings.FactoryDispatchKindPetriTransition, payload.TransitionID, usage, true, nil
	case workers.OutcomeFailed:
		return recordings.FactoryDispatchStatusFailed, recordings.FactoryDispatchKindPetriTransition, payload.TransitionID, usage, true, nil
	default:
		return "", "", "", nil, true, errors.New("invalid dispatch response outcome")
	}
}

func historicalDispatchUsage(
	usage *workers.DispatchUsageEventPayload,
	fallbackDurationMillis *int64,
) *recordings.FactoryDispatchUsage {
	if usage == nil && fallbackDurationMillis == nil {
		return nil
	}
	result := &recordings.FactoryDispatchUsage{}
	if usage != nil {
		result.DurationMillis = cloneInt64(usage.DurationMillis)
		result.InputTokens = nonNegativeInt64(usage.InputTokens)
		result.OutputTokens = nonNegativeInt64(usage.OutputTokens)
		if result.InputTokens != nil && result.OutputTokens != nil &&
			*result.InputTokens <= math.MaxInt64-*result.OutputTokens {
			totalTokens := *result.InputTokens + *result.OutputTokens
			result.TotalTokens = &totalTokens
		}
	}
	if result.DurationMillis == nil {
		result.DurationMillis = cloneInt64(fallbackDurationMillis)
	}
	return result
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func nonNegativeInt64(value *int64) *int64 {
	if value == nil || *value < 0 {
		return nil
	}
	cloned := *value
	return &cloned
}

func historicalDispatchID(event factorydefinitions.FactoryEvent) (string, error) {
	if event.Context.DispatchID == nil || strings.TrimSpace(*event.Context.DispatchID) == "" {
		return "", errors.New("dispatch lifecycle event has no dispatch identity")
	}
	return *event.Context.DispatchID, nil
}

func historicalRequestID(event factorydefinitions.FactoryEvent) string {
	if event.Context.RequestID == nil {
		return ""
	}
	return *event.Context.RequestID
}

func validDispatchKind(kind factorydefinitions.FactoryDispatchKind) bool {
	switch kind {
	case factorydefinitions.FactoryDispatchKindJavaScriptAgent,
		factorydefinitions.FactoryDispatchKindJavaScriptScript,
		factorydefinitions.FactoryDispatchKindJavaScriptSynthesize,
		factorydefinitions.FactoryDispatchKindJavaScriptSystem,
		factorydefinitions.FactoryDispatchKindJavaScriptTool,
		factorydefinitions.FactoryDispatchKindJavaScriptVerify,
		factorydefinitions.FactoryDispatchKindPetriTransition:
		return true
	default:
		return false
	}
}

func validDispatchStatus(status factorydefinitions.FactoryDispatchStatus) bool {
	switch status {
	case factorydefinitions.FactoryDispatchStatusCompleted,
		factorydefinitions.FactoryDispatchStatusFailed,
		factorydefinitions.FactoryDispatchStatusInterrupted,
		factorydefinitions.FactoryDispatchStatusQueued,
		factorydefinitions.FactoryDispatchStatusRunning:
		return true
	default:
		return false
	}
}

func historicalDispatchCorrupt(identity recordings.HistoricalRecordingIdentity, event recordings.CanonicalEvent, cause error) error {
	return historicalQueryError(recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, event.ID, cause)
}
