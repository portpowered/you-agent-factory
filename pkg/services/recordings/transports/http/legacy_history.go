package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func (a *Adapter) hasLegacyHistory() bool {
	return a != nil && a.legacyHistory != nil
}

func (a *Adapter) hasLegacyLiveEvents() bool {
	return a != nil && a.legacyLiveEvents != nil
}

func isExpectedLiveFallback(err error) bool {
	if errors.Is(err, recordings.ErrPortableArtifactUnavailable) ||
		errors.Is(err, recordings.ErrMissingRecordingTarget) {
		return true
	}
	var historicalErr *recordings.HistoricalRecordingQueryError
	return errors.As(err, &historicalErr) &&
		historicalErr.Kind == recordings.HistoricalRecordingQueryErrorUnavailable
}

func legacyEventReconnectCursor(params factoryapi.GetEventsBySessionIdParams) *interfaces.FactoryEventReconnectCursor {
	if params.AfterEventId == nil && params.AfterSequence == nil {
		return nil
	}
	cursor := &interfaces.FactoryEventReconnectCursor{}
	if params.AfterEventId != nil {
		cursor.AfterEventID = string(*params.AfterEventId)
	}
	if params.AfterSequence != nil {
		sequence := int(*params.AfterSequence)
		cursor.AfterSequence = &sequence
	}
	return cursor
}

func (a *Adapter) legacyLive(ctx context.Context, sessionID string, params factoryapi.GetEventsBySessionIdParams) (*interfaces.FactoryEventStream, error) {
	return a.legacyLiveEvents.SubscribeFactoryEventsForSession(ctx, sessionID, legacyEventReconnectCursor(params))
}

func (a *Adapter) legacyLiveProbe(ctx context.Context, sessionID string, params factoryapi.GetEventsBySessionIdParams) error {
	return a.legacyLiveEvents.ProbeFactoryEventsForSession(ctx, sessionID, legacyEventReconnectCursor(params))
}

func (a *Adapter) legacyResult(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetFactorySessionResultsParams,
) (factoryapi.FactorySessionResult, error) {
	request, err := factorysessionmapping.ResultRequestFromAPI(params)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	if a.legacyPreparation != nil {
		request, err = a.legacyPreparation.PrepareResult(request)
		if err != nil {
			return factoryapi.FactorySessionResult{}, err
		}
	}
	return a.legacyHistory.GetDurableFactorySessionResult(ctx, sessionID, request)
}

func (a *Adapter) legacyEvents(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetEventsBySessionIdParams,
) (*interfaces.FactoryEventStream, error) {
	request, err := factorysessionmapping.EventReconnectRequestFromAPI(params)
	if err != nil {
		return nil, err
	}
	if a.legacyPreparation != nil {
		request, err = a.legacyPreparation.PrepareEventReconnect(request)
		if err != nil {
			return nil, err
		}
	}
	return a.legacyHistory.ReadDurableFactorySessionEvents(ctx, sessionID, request)
}

func (a *Adapter) legacyProbe(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetEventsBySessionIdParams,
) error {
	request, err := factorysessionmapping.EventReconnectRequestFromAPI(params)
	if err != nil {
		return err
	}
	if a.legacyPreparation != nil {
		request, err = a.legacyPreparation.PrepareEventReconnect(request)
		if err != nil {
			return err
		}
	}
	return a.legacyHistory.ProbeDurableFactorySessionEvents(ctx, sessionID, request)
}

func (a *Adapter) legacyDispatches(
	ctx context.Context,
	sessionID string,
	params factoryapi.ListFactorySessionDispatchesParams,
) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	return a.legacyHistory.ListDurableFactorySessionDispatches(ctx, sessionID, params)
}

func (a *Adapter) legacyDispatch(ctx context.Context, sessionID, dispatchID string) (factoryapi.FactoryDispatch, error) {
	return a.legacyHistory.GetDurableFactorySessionDispatch(ctx, sessionID, dispatchID)
}

func (a *Adapter) legacyArtifacts(ctx context.Context, sessionID string) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	return a.legacyHistory.ListDurableFactorySessionArtifacts(ctx, sessionID)
}

func (a *Adapter) legacyArtifact(ctx context.Context, sessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error) {
	return a.legacyHistory.GetDurableFactorySessionArtifact(ctx, sessionID, artifactID)
}

func (a *Adapter) writeLegacyError(w http.ResponseWriter, err error, fallback string) bool {
	if err == nil {
		return false
	}
	if status, response, ok := factorysessionmapping.ExecutionErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	var status int
	message := "factory session not found"
	code := string(factoryapi.ErrorResponseCodeNOTFOUND)
	switch {
	case errors.Is(err, factorysessions.ErrDurableSessionNotFound), errors.Is(err, factorysessions.ErrSessionNotFound), errors.Is(err, apisurface.ErrFactorySessionNotFound):
		status = http.StatusNotFound
	case errors.Is(err, factorysessions.ErrDispatchNotFound):
		status, message = http.StatusNotFound, "dispatch not found"
	case errors.Is(err, factorysessions.ErrArtifactNotFound):
		status, message = http.StatusNotFound, "factory session artifact not found"
	case errors.Is(err, factorysessions.ErrReconnectCursorNotFound), errors.Is(err, apisurface.ErrInvalidEventReconnectCursor):
		status, message, code = http.StatusBadRequest, "invalid event reconnect cursor", string(factoryapi.ErrorResponseCodeBADREQUEST)
	default:
		status, message, code = http.StatusInternalServerError, fallback, string(factoryapi.ErrorResponseCodeINTERNALERROR)
	}
	a.writeError(w, status, message, code)
	return true
}

func (a *Adapter) probeLegacyLiveEventRecovery(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	params factoryapi.GetEventsBySessionIdParams,
) {
	err := a.legacyLiveProbe(r.Context(), sessionID, params)
	if err != nil {
		outcome := factoryapi.FactorySessionEventStreamRecoveryOutcomeINTERNALERROR
		omitCursor := false
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			outcome = factoryapi.FactorySessionEventStreamRecoveryOutcomeUNKNOWNSESSION
		} else if errors.Is(err, factorysessions.ErrReconnectCursorNotFound) || errors.Is(err, apisurface.ErrInvalidEventReconnectCursor) || errors.Is(err, recordings.ErrReconnectCursorNotFound) {
			outcome = factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE
			omitCursor = true
		}
		a.writeJSON(w, http.StatusOK, EventStreamRecoveryToAPI(sessionID, outcome, omitCursor))
		return
	}
	a.writeJSON(w, http.StatusOK, EventStreamRecoveryToAPI(sessionID, factoryapi.FactorySessionEventStreamRecoveryOutcomeSTREAMREADY, false))
}

func (a *Adapter) streamLegacyFactoryEvents(
	w http.ResponseWriter,
	r *http.Request,
	stream *interfaces.FactoryEventStream,
	sessionID string,
) {
	if stream == nil {
		a.writeError(w, http.StatusInternalServerError, "failed to subscribe to factory events", "INTERNAL_ERROR")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		a.writeError(w, http.StatusInternalServerError, "streaming unsupported", "INTERNAL_ERROR")
		return
	}
	write := legacyFactoryEventWriter(w)
	writeLegacyStreamHeaders(w, stream, sessionID)
	if !writeLegacyHistory(flusher, stream.History, write) {
		return
	}
	followLegacyFactoryEvents(r, flusher, stream.Events, write)
}

func writeLegacyStreamHeaders(
	w http.ResponseWriter,
	stream *interfaces.FactoryEventStream,
	sessionID string,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, strconv.Itoa(len(stream.History)))
	if value := strings.TrimSpace(stream.BackendScopeID); value != "" {
		w.Header().Set("X-Factory-Session-Backend-Scope-Id", value)
	}
	if value := strings.TrimSpace(stream.LogicalSessionKeyID); value != "" {
		w.Header().Set("X-Factory-Session-Logical-Session-Key-Id", value)
	}
	if value := strings.TrimSpace(stream.FactorySessionID); value != "" {
		w.Header().Set(SessionEventStreamFactorySessionHeader, value)
	}
	if value := strings.TrimSpace(stream.StreamGenerationID); value != "" {
		w.Header().Set(SessionEventStreamGenerationHeader, value)
	}
}

func legacyFactoryEventWriter(w http.ResponseWriter) func(interfaces.FactoryEvent) error {
	return func(event interfaces.FactoryEvent) error {
		var apiEvent factoryapi.FactoryEvent
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(encoded, &apiEvent); err != nil {
			return err
		}
		return writeSSEDataJSON(w, apiEvent)
	}
}

func writeLegacyHistory(
	flusher http.Flusher,
	history []interfaces.FactoryEvent,
	write func(interfaces.FactoryEvent) error,
) bool {
	for _, event := range history {
		if err := write(event); err != nil {
			return false
		}
		flusher.Flush()
	}
	return true
}

func followLegacyFactoryEvents(
	r *http.Request,
	flusher http.Flusher,
	events <-chan interfaces.FactoryEvent,
	write func(interfaces.FactoryEvent) error,
) {
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := write(event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (a *Adapter) probeLegacyEventRecovery(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	params factoryapi.GetEventsBySessionIdParams,
) {
	err := a.legacyProbe(r.Context(), sessionID, params)
	if err != nil {
		outcome := factoryapi.FactorySessionEventStreamRecoveryOutcomeINTERNALERROR
		omitCursor := false
		if errors.Is(err, factorysessions.ErrDurableSessionNotFound) || errors.Is(err, factorysessions.ErrSessionNotFound) {
			outcome = factoryapi.FactorySessionEventStreamRecoveryOutcomeUNKNOWNSESSION
		} else if errors.Is(err, factorysessions.ErrReconnectCursorNotFound) || errors.Is(err, apisurface.ErrInvalidEventReconnectCursor) {
			outcome = factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE
			omitCursor = true
		}
		a.writeJSON(w, http.StatusOK, EventStreamRecoveryToAPI(sessionID, outcome, omitCursor))
		return
	}
	a.writeJSON(w, http.StatusOK, EventStreamRecoveryToAPI(
		sessionID,
		factoryapi.FactorySessionEventStreamRecoveryOutcomeSTREAMREADY,
		false,
	))
}
