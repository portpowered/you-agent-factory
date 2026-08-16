package service

import (
	"errors"
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
		status, kind, transitionID, applies, err := historicalDispatchStatus(legacy)
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

func historicalDispatchStatus(event factorydefinitions.FactoryEvent) (recordings.FactoryDispatchStatus, recordings.FactoryDispatchKind, string, bool, error) {
	switch event.Type {
	case factorydefinitions.FactoryEventTypeDispatchRequest:
		var payload factorydefinitions.DispatchRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil || strings.TrimSpace(payload.TransitionID) == "" {
			return "", "", "", true, errors.New("invalid dispatch request")
		}
		return recordings.FactoryDispatchStatusRunning, recordings.FactoryDispatchKindPetriTransition, payload.TransitionID, true, nil
	case factorydefinitions.FactoryEventTypeDispatchResponse:
		return dispatchResponseStatus(event)
	case factorydefinitions.FactoryEventTypeDispatchQueued:
		var payload factorydefinitions.DispatchQueuedEventPayload
		if err := event.DecodePayload(&payload); err != nil || !validDispatchKind(payload.DispatchKind) {
			return "", "", "", true, errors.New("invalid queued dispatch")
		}
		return recordings.FactoryDispatchStatusQueued, recordings.FactoryDispatchKind(payload.DispatchKind), "", true, nil
	case factorydefinitions.FactoryEventTypeDispatchInterrupted:
		var payload factorydefinitions.DispatchInterruptedEventPayload
		if err := event.DecodePayload(&payload); err != nil || !validDispatchStatus(payload.ObservedStatus) {
			return "", "", "", true, errors.New("invalid interrupted dispatch")
		}
		return recordings.FactoryDispatchStatusInterrupted, "", "", true, nil
	case factorydefinitions.FactoryEventTypeDispatchReconciled:
		var payload factorydefinitions.DispatchReconciledEventPayload
		if err := event.DecodePayload(&payload); err != nil || !validDispatchStatus(payload.ReconciledStatus) {
			return "", "", "", true, errors.New("invalid reconciled dispatch")
		}
		return recordings.FactoryDispatchStatus(payload.ReconciledStatus), "", "", true, nil
	default:
		return "", "", "", false, nil
	}
}

func dispatchResponseStatus(event factorydefinitions.FactoryEvent) (recordings.FactoryDispatchStatus, recordings.FactoryDispatchKind, string, bool, error) {
	var payload workers.DispatchResponseEventPayload
	if err := event.DecodePayload(&payload); err != nil || strings.TrimSpace(payload.TransitionID) == "" {
		return "", "", "", true, errors.New("invalid dispatch response")
	}
	switch payload.Outcome {
	case workers.OutcomeAccepted, workers.OutcomeContinue, workers.OutcomeRejected:
		return recordings.FactoryDispatchStatusCompleted, recordings.FactoryDispatchKindPetriTransition, payload.TransitionID, true, nil
	case workers.OutcomeFailed:
		return recordings.FactoryDispatchStatusFailed, recordings.FactoryDispatchKindPetriTransition, payload.TransitionID, true, nil
	default:
		return "", "", "", true, errors.New("invalid dispatch response outcome")
	}
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
