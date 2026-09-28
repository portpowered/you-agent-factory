package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// isSessionReconnectCursorFailure reports whether a durable session read
// failed because its reconnect cursor no longer matches retained history.
func isSessionReconnectCursorFailure(err error) bool {
	return errors.Is(err, factorysessions.ErrReconnectCursorNotFound) ||
		errors.Is(err, apisurface.ErrInvalidEventReconnectCursor)
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

func sessionEventReconnectCursor(params factoryapi.GetEventsBySessionIdParams) *interfaces.FactoryEventReconnectCursor {
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

func (a *Adapter) sessionLive(ctx context.Context, sessionID string, params factoryapi.GetEventsBySessionIdParams) (*interfaces.FactoryEventStream, error) {
	return a.sessions.SubscribeFactoryEventsForSession(ctx, sessionID, sessionEventReconnectCursor(params))
}

func (a *Adapter) sessionLiveProbe(ctx context.Context, sessionID string, params factoryapi.GetEventsBySessionIdParams) error {
	return a.sessions.ProbeFactoryEventsForSession(ctx, sessionID, sessionEventReconnectCursor(params))
}

func (a *Adapter) writeSessionReadError(w http.ResponseWriter, err error, fallback string) bool {
	if err == nil {
		return false
	}
	var requestValidation *apisurface.RequestValidationError
	if errors.As(err, &requestValidation) {
		a.writeError(w, http.StatusBadRequest, requestValidation.Error(), "BAD_REQUEST")
		return true
	}
	var executionValidation *factorysessions.ExecutionValidationError
	if errors.As(err, &executionValidation) {
		a.writeError(w, http.StatusBadRequest, executionValidation.Message, "BAD_REQUEST")
		return true
	}
	if errors.Is(err, factorysessions.ErrExecutionRequestIDConflict) {
		a.writeError(w, http.StatusConflict, "requestId was already used with different execution inputs.", "EXECUTION_REQUEST_ID_CONFLICT")
		return true
	}
	var resumeError *factorysessions.ResumeError
	if errors.As(err, &resumeError) {
		a.writeError(w, http.StatusBadRequest, resumeError.Error(), "BAD_REQUEST")
		return true
	}
	var status int
	message := "factory session not found"
	code := string(factoryapi.ErrorResponseCodeNOTFOUND)
	switch {
	case errors.Is(err, factorysessions.ErrDurableSessionNotFound), errors.Is(err, factorysessions.ErrSessionNotFound),
		errors.Is(err, apisurface.ErrFactorySessionNotFound):
		status = http.StatusNotFound
	case errors.Is(err, factorysessions.ErrDispatchNotFound):
		status, message = http.StatusNotFound, "dispatch not found"
	case errors.Is(err, factorysessions.ErrArtifactNotFound):
		status, message = http.StatusNotFound, "factory session artifact not found"
	case errors.Is(err, factorysessions.ErrReconnectCursorNotFound),
		errors.Is(err, recordings.ErrReconnectCursorNotFound),
		errors.Is(err, apisurface.ErrInvalidEventReconnectCursor):
		status, message, code = http.StatusBadRequest, "invalid event reconnect cursor", string(factoryapi.ErrorResponseCodeBADREQUEST)
	default:
		status, message, code = http.StatusInternalServerError, fallback, string(factoryapi.ErrorResponseCodeINTERNALERROR)
	}
	a.writeError(w, status, message, code)
	return true
}

func (a *Adapter) probeSessionLiveEventRecovery(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	params factoryapi.GetEventsBySessionIdParams,
) {
	err := a.sessionLiveProbe(r.Context(), sessionID, params)
	if err != nil {
		outcome := factoryapi.FactorySessionEventStreamRecoveryOutcomeINTERNALERROR
		omitCursor := false
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			outcome = factoryapi.FactorySessionEventStreamRecoveryOutcomeUNKNOWNSESSION
		} else if isSessionReconnectCursorFailure(err) || errors.Is(err, recordings.ErrReconnectCursorNotFound) {
			outcome = factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE
			omitCursor = true
		}
		a.writeJSON(w, http.StatusOK, EventStreamRecoveryToAPI(sessionID, outcome, omitCursor))
		return
	}
	a.writeJSON(w, http.StatusOK, EventStreamRecoveryToAPI(sessionID, factoryapi.FactorySessionEventStreamRecoveryOutcomeSTREAMREADY, false))
}

func (a *Adapter) streamSessionFactoryEvents(
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
	write := sessionFactoryEventWriter(w)
	writeSessionStreamHeaders(w, stream, sessionID)
	if !writeSessionHistory(flusher, stream.History, write) {
		return
	}
	followSessionFactoryEvents(r, flusher, stream.Events, write)
}

func writeSessionStreamHeaders(
	w http.ResponseWriter,
	stream *interfaces.FactoryEventStream,
	sessionID string,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set(SessionEventStreamRetainedCountHeader, strconv.Itoa(len(stream.History)))
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

func sessionFactoryEventWriter(w http.ResponseWriter) func(interfaces.FactoryEvent) error {
	return func(event interfaces.FactoryEvent) error {
		apiEvent, err := apisurface.FactoryEventToAPI(event)
		if err != nil {
			return err
		}
		return writeSSEDataJSON(w, apiEvent)
	}
}

func writeSessionHistory(
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

func followSessionFactoryEvents(
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
