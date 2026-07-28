package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	sessionEventStreamFactorySessionHeader = "X-Factory-Session-Factory-Session-Id"
	sessionEventStreamGenerationHeader     = "X-Factory-Session-Stream-Generation-Id"
)

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
	if requestsJSONEventRecoveryProbe(r) {
		a.probeEventStreamRecovery(w, input)
		return
	}
	request, err := SubscribeRequestFromAPI(input)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid event reconnect cursor", "BAD_REQUEST")
		return
	}
	result, err := a.invokeSubscribeFrom(r.Context(), request)
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationEventSubscribe, err)
		return
	}
	a.streamFactoryEvents(w, r, result.Subscription, input.SessionID, input.StreamGenerationID)
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

	for {
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
