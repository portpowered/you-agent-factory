package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

const (
	sessionEventStreamFactorySessionHeader = "X-Factory-Session-Factory-Session-Id"
	sessionEventStreamGenerationHeader     = "X-Factory-Session-Stream-Generation-Id"
)

// SessionEventStreamRetainedCountHeader carries the number of already-committed
// events replayed before an event stream switches to live delivery. It is the
// same header the Factory Sessions stream publishes.
const SessionEventStreamRetainedCountHeader = factorysessionmapping.SessionEventStreamRetainedCountHeader

// SessionEventStreamFactorySessionHeader identifies the resolved session on an
// event stream response.
const SessionEventStreamFactorySessionHeader = sessionEventStreamFactorySessionHeader

// SessionEventStreamGenerationHeader identifies the stream generation on an
// event stream response.
const SessionEventStreamGenerationHeader = sessionEventStreamGenerationHeader

// GetEventsBySessionId decodes one session-scoped event subscribe or history
// probe request, invokes the accepted Recordings root, and encodes the public
// HTTP success or stream response.
func (a *Adapter) GetEventsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.GetEventsBySessionIdParams,
) {
	input := EventSubscribeInput{
		SessionID:          string(sessionID),
		Params:             params,
		StreamGenerationID: r.Header.Get(SessionEventStreamGenerationHeader),
	}
	if a.handleLegacyDurableEvents(w, r, input) {
		return
	}
	if a.handleLegacyLiveFallbackEvents(w, r, input) {
		return
	}
	if a.handleLegacyLiveEvents(w, r, input) {
		return
	}
	if a.handleEventRecoveryProbe(w, r, input) {
		return
	}
	if a.handleHistoricalEvents(w, r, input) {
		return
	}
	a.handleLiveEvents(w, r, input)
}

func (a *Adapter) handleLegacyDurableEvents(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) bool {
	if !isDurableHistorySession(input.SessionID) || !a.hasLegacyHistory() {
		return false
	}
	if a.root != nil {
		_, err := a.historicalRecording(r.Context(), input.SessionID)
		if err == nil || !isExpectedLiveFallback(err) {
			return false
		}
	}
	return a.serveLegacyDurableEvents(w, r, input)
}

func (a *Adapter) serveLegacyDurableEvents(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) bool {
	if requestsJSONEventRecoveryProbe(r) {
		a.probeLegacyEventRecovery(w, r, input.SessionID, input.Params)
		return true
	}
	stream, err := a.legacyEvents(r.Context(), input.SessionID, input.Params)
	if shouldEndOnRequestContext(r.Context(), err) {
		return true
	}
	if err != nil {
		a.writeLegacyError(w, err, "failed to subscribe to factory events")
		return true
	}
	a.streamLegacyFactoryEvents(w, r, stream, input.SessionID)
	return true
}

func (a *Adapter) handleLegacyLiveFallbackEvents(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) bool {
	if !isDurableHistorySession(input.SessionID) || !a.hasLegacyLiveEvents() {
		return false
	}
	if a.root != nil {
		_, err := a.historicalRecording(r.Context(), input.SessionID)
		if err == nil || !isExpectedLiveFallback(err) {
			return false
		}
	}
	return a.serveLegacyLiveEvents(w, r, input)
}

func (a *Adapter) handleLegacyLiveEvents(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) bool {
	if isDurableHistorySession(input.SessionID) || !a.hasLegacyLiveEvents() {
		return false
	}
	return a.serveLegacyLiveEvents(w, r, input)
}

func (a *Adapter) serveLegacyLiveEvents(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) bool {
	if requestsJSONEventRecoveryProbe(r) {
		a.probeLegacyLiveEventRecovery(w, r, input.SessionID, input.Params)
		return true
	}
	stream, err := a.legacyLive(r.Context(), input.SessionID, input.Params)
	if shouldEndOnRequestContext(r.Context(), err) {
		return true
	}
	if err != nil {
		a.writeLegacyError(w, err, "failed to subscribe to factory events")
		return true
	}
	a.streamLegacyFactoryEvents(w, r, stream, input.SessionID)
	return true
}

func (a *Adapter) handleEventRecoveryProbe(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) bool {
	if !requestsJSONEventRecoveryProbe(r) {
		return false
	}
	if isDurableHistorySession(input.SessionID) {
		a.probeHistoricalEventStreamRecovery(w, r, input)
		return true
	}
	a.probeEventStreamRecovery(w, input)
	return true
}

func (a *Adapter) handleHistoricalEvents(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) bool {
	if !isDurableHistorySession(input.SessionID) {
		return false
	}
	result, err := a.historicalRecording(r.Context(), input.SessionID)
	if shouldEndOnRequestContext(r.Context(), err) {
		return true
	}
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationHistoricalRead, err)
		return true
	}
	start, err := historicalEventStart(result.Events, input.Params, input.StreamGenerationID)
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationEventSubscribe, err)
		return true
	}
	streamGenerationID := input.StreamGenerationID
	if strings.TrimSpace(streamGenerationID) == "" && len(result.Events) > 0 {
		streamGenerationID = result.Events[0].Cursor.StreamGenerationID
	}
	a.streamFactoryEvents(
		w, r, historicalEventSubscription(result.Events[start:]),
		input.SessionID, streamGenerationID, len(result.Events[start:]),
	)
	return true
}

func (a *Adapter) handleLiveEvents(
	w http.ResponseWriter,
	r *http.Request,
	input EventSubscribeInput,
) {
	request, err := SubscribeRequestFromAPI(input)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid event reconnect cursor", "BAD_REQUEST")
		return
	}
	if requestContextEnded(r.Context()) {
		return
	}
	result, err := a.invokeSubscribeFrom(r.Context(), request)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationEventSubscribe, err)
		return
	}
	a.streamFactoryEvents(
		w, r, result.Subscription, input.SessionID,
		input.StreamGenerationID, result.RetainedEventCount,
	)
}

func (a *Adapter) probeEventStreamRecovery(w http.ResponseWriter, input EventSubscribeInput) {
	request, err := SubscribeRequestFromAPI(input)
	if err != nil {
		a.writeJSON(
			w,
			http.StatusOK,
			EventStreamRecoveryToAPI(
				input.SessionID,
				factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE,
				true,
			),
		)
		return
	}
	_, err = a.invokeSubscribeFrom(context.Background(), request)
	if err != nil {
		if isEventReconnectValidationError(err) {
			a.writeJSON(
				w,
				http.StatusOK,
				EventStreamRecoveryToAPI(
					input.SessionID,
					factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE,
					true,
				),
			)
			return
		}
		a.writeJSON(
			w,
			http.StatusOK,
			EventStreamRecoveryToAPI(
				input.SessionID,
				factoryapi.FactorySessionEventStreamRecoveryOutcomeINTERNALERROR,
				false,
			),
		)
		return
	}
	a.writeJSON(
		w,
		http.StatusOK,
		EventStreamRecoveryToAPI(
			input.SessionID,
			factoryapi.FactorySessionEventStreamRecoveryOutcomeSTREAMREADY,
			false,
		),
	)
}

func requestsJSONEventRecoveryProbe(r *http.Request) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Accept"))), "application/json")
}

func (a *Adapter) streamFactoryEvents(
	w http.ResponseWriter,
	r *http.Request,
	subscription recordings.EventSubscription,
	sessionID string,
	streamGenerationID string,
	retainedEventCount int,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		a.writeError(w, http.StatusInternalServerError, "streaming unsupported", "INTERNAL_ERROR")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		w.Header().Set(SessionEventStreamFactorySessionHeader, sessionID)
	}
	if streamGenerationID = strings.TrimSpace(streamGenerationID); streamGenerationID != "" {
		w.Header().Set(SessionEventStreamGenerationHeader, streamGenerationID)
	}
	w.Header().Set(
		SessionEventStreamRetainedCountHeader,
		strconv.Itoa(retainedEventCount),
	)
	// Publish the response headers before waiting for the first live event.
	// A client uses the retained count as the boundary between replay and live
	// delivery, so it must be able to establish that boundary even when the
	// stream currently has no retained events.
	flusher.Flush()

	for {
		if requestContextEnded(r.Context()) {
			return
		}
		outcome := subscription.Next(r.Context())
		switch outcome.Kind {
		case recordings.SubscriptionClosed:
			return
		case recordings.SubscriptionGap:
			return
		case recordings.SubscriptionEvent:
			apiEvent, err := FactoryEventToAPI(outcome.Event)
			if err != nil {
				return
			}
			if err := writeSSEDataJSON(w, apiEvent); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
