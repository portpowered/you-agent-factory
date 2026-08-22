package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func historicalEventStart(
	events []recordings.CanonicalEvent,
	params factoryapi.GetEventsBySessionIdParams,
	streamGenerationID string,
) (int, error) {
	if params.AfterEventId != nil {
		eventID := strings.TrimSpace(string(*params.AfterEventId))
		if eventID == "" {
			return 0, recordings.ErrInvalidReconnectCursor
		}
		for index, event := range events {
			if string(event.ID) == eventID {
				return index + 1, nil
			}
		}
		return 0, recordings.ErrReconnectCursorNotFound
	}
	if params.AfterSequence == nil {
		return 0, nil
	}
	if *params.AfterSequence < 0 || strings.TrimSpace(streamGenerationID) == "" {
		return 0, recordings.ErrInvalidReconnectCursor
	}
	sequence := recordings.CanonicalEventSequence(*params.AfterSequence)
	for index, event := range events {
		if event.Cursor.StreamGenerationID == streamGenerationID && event.Cursor.Sequence == sequence {
			return index + 1, nil
		}
	}
	return 0, recordings.ErrReconnectCursorNotFound
}

func historicalEventSubscription(events []recordings.CanonicalEvent) recordings.EventSubscription {
	index := 0
	return func(ctx context.Context) recordings.SubscriptionOutcome {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
		}
		if index >= len(events) {
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		}
		event := events[index]
		index++
		return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event}
	}
}

func (a *Adapter) probeHistoricalEventStreamRecovery(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) {
	result, err := a.historicalRecording(r.Context(), input.SessionID)
	if err == nil {
		_, err = historicalEventStart(result.Events, input.Params, input.StreamGenerationID)
	}
	if err != nil {
		outcome := factoryapi.FactorySessionEventStreamRecoveryOutcomeINTERNALERROR
		omitCursor := false
		switch {
		case isHistoricalSessionMissing(err):
			outcome = factoryapi.FactorySessionEventStreamRecoveryOutcomeUNKNOWNSESSION
		case isEventReconnectValidationError(err):
			outcome = factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE
			omitCursor = true
		}
		a.writeJSON(w, http.StatusOK, EventStreamRecoveryToAPI(input.SessionID, outcome, omitCursor))
		return
	}
	a.writeJSON(w, http.StatusOK, EventStreamRecoveryToAPI(
		input.SessionID,
		factoryapi.FactorySessionEventStreamRecoveryOutcomeSTREAMREADY,
		false,
	))
}

func isHistoricalSessionMissing(err error) bool {
	if errors.Is(err, recordings.ErrMissingRecordingTarget) ||
		errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		return true
	}
	var historicalErr *recordings.HistoricalRecordingQueryError
	return errors.As(err, &historicalErr) &&
		historicalErr.Kind == recordings.HistoricalRecordingQueryErrorMissingHistory
}
