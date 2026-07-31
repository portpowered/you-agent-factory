package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	events "github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

const (
	sessionEventStreamBackendScopeHeader      = "X-Factory-Session-Backend-Scope-Id"
	sessionEventStreamLogicalSessionKeyHeader = "X-Factory-Session-Logical-Session-Key-Id"
	sessionEventStreamFactorySessionHeader    = "X-Factory-Session-Factory-Session-Id"
	sessionEventStreamGenerationHeader        = "X-Factory-Session-Stream-Generation-Id"
)

const (
	// SessionEventStreamBackendScopeHeader identifies the backend scope used by
	// a session-scoped event stream.
	SessionEventStreamBackendScopeHeader = sessionEventStreamBackendScopeHeader
	// SessionEventStreamLogicalSessionKeyHeader identifies the logical session.
	SessionEventStreamLogicalSessionKeyHeader = sessionEventStreamLogicalSessionKeyHeader
	// SessionEventStreamFactorySessionHeader identifies the resolved session.
	SessionEventStreamFactorySessionHeader = sessionEventStreamFactorySessionHeader
	// SessionEventStreamGenerationHeader identifies the stream generation.
	SessionEventStreamGenerationHeader = sessionEventStreamGenerationHeader
)

// GetStatus handles GET /status as the supported runtime status read model.
func (s *Server) GetStatus(w http.ResponseWriter, r *http.Request) {
	s.getStatus(w, r, "")
}

func (s *Server) GetStatusBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.getStatus(w, r, string(sessionID))
}

func (s *Server) getStatus(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
) {
	if s.factoryStatus == nil {
		s.writeError(w, http.StatusServiceUnavailable, "factory status is unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	status, err := s.factoryStatus.ProjectFactoryStatus(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get engine state snapshot failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get engine state snapshot", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusOK, apisurface.FactoryStatusToAPI(status))
}

func (s *Server) GetEventsBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.GetEventsBySessionIdParams) {
	reconnect := reconnectCursorFromParams(params.AfterEventId, params.AfterSequence)
	if requestsJSONEventRecoveryProbe(r) {
		s.probeFactorySessionEventStreamRecovery(w, r, string(sessionID), reconnect)
		return
	}

	if isDurableExecutionSessionID(string(sessionID)) {
		reader, ok := s.requireDurableSessionEventsReader(w)
		if !ok {
			return
		}
		raw, err := factorysessionmapping.EventReconnectRequestFromAPI(params)
		if err == nil {
			raw, err = s.sessionRequests.PrepareEventReconnect(raw)
		}
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
			return
		}
		stream, err := reader.ReadDurableFactorySessionEvents(r.Context(), string(sessionID), raw)
		if err != nil {
			if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
				s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
				return
			}
			if errors.Is(err, apisurface.ErrInvalidEventReconnectCursor) || errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
				s.writeError(w, http.StatusBadRequest, "invalid event reconnect cursor", "BAD_REQUEST")
				return
			}
			if s.writeDurableSessionReadError(w, err) {
				return
			}
			s.logger.Error("read durable factory session events failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to subscribe to factory events", "INTERNAL_ERROR")
			return
		}
		s.getEvents(w, r, false, func(ctx context.Context) (*interfaces.FactoryEventStream, error) {
			return stream, nil
		})
		return
	}

	workAPI, ok := s.requireWorkAPI(w)
	if !ok {
		return
	}
	s.getEvents(w, r, true, func(ctx context.Context) (*interfaces.FactoryEventStream, error) {
		return workAPI.SubscribeFactoryEventsForSession(ctx, string(sessionID), reconnect)
	})
}

// GetFactoryResponseEventsBySessionId streams the retained-then-live ephemeral
// response-event cursor owned by exactly one Factory Session.
func (s *Server) GetFactoryResponseEventsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.GetFactoryResponseEventsBySessionIdParams,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported", "INTERNAL_ERROR")
		return
	}

	request, ok := s.responseEventStreamRequest(w, string(sessionID), params)
	if !ok {
		return
	}

	if isDurableExecutionSessionID(string(sessionID)) {
		reader, ok := s.requireDurableSessionResponseEventsReader(w)
		if !ok {
			return
		}
		subscription, err := reader.SubscribeDurableFactoryResponseEvents(r.Context(), request)
		if err != nil {
			if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
				s.writeError(w, http.StatusNotFound, "factory response-event session not found", "RESPONSE_EVENT_SESSION_NOT_FOUND")
				return
			}
			if errors.Is(err, apisurface.ErrFactoryResponseEventStreamExpired) {
				s.writeError(w, http.StatusGone, "factory response-event stream expired", "RESPONSE_EVENT_STREAM_EXPIRED")
				return
			}
			s.logger.Error("subscribe durable factory response events failed", zap.String("session_id", string(sessionID)), zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to subscribe to factory response events", "INTERNAL_ERROR")
			return
		}
		defer subscription.Detach()
		streamFactoryResponseEvents(w, r, flusher, subscription, string(sessionID), s.logger)
		return
	}

	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	subscription, err := sessionRuntime.SubscribeFactoryResponseEventsForSession(r.Context(), request)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory response-event session not found", "RESPONSE_EVENT_SESSION_NOT_FOUND")
			return
		}
		if errors.Is(err, apisurface.ErrFactoryResponseEventStreamExpired) {
			s.writeError(w, http.StatusGone, "factory response-event stream expired", "RESPONSE_EVENT_STREAM_EXPIRED")
			return
		}
		s.logger.Error("subscribe factory response events failed", zap.String("session_id", string(sessionID)), zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to subscribe to factory response events", "INTERNAL_ERROR")
		return
	}
	defer subscription.Detach()

	streamFactoryResponseEvents(w, r, flusher, subscription, string(sessionID), s.logger)
}

func streamFactoryResponseEvents(
	w http.ResponseWriter,
	r *http.Request,
	flusher http.Flusher,
	subscription apisurface.FactoryResponseEventSubscription,
	sessionID string,
	logger *zap.Logger,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	// Commit the SSE response before waiting for the first event. Without this
	// flush, clients cannot distinguish an established quiet subscription from
	// a server that never accepted the request, and zero-event sessions block in
	// http.Client.Do until their context expires.
	flusher.Flush()

	for {
		events, err := subscription.Next(r.Context())
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, factorysessionexecution.ErrResponseEventSubscriptionClosed) {
				return
			}
			logger.Error("read factory response events failed", zap.String("session_id", sessionID), zap.Error(err))
			return
		}
		for _, event := range events {
			if err := writeFactoryResponseEventSSE(w, event); err != nil {
				logger.Debug("write factory response event failed", zap.String("session_id", sessionID), zap.Error(err))
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) responseEventStreamRequest(
	w http.ResponseWriter,
	sessionID string,
	params factoryapi.GetFactoryResponseEventsBySessionIdParams,
) (factorysessionexecution.ResponseEventSubscriptionRequest, bool) {
	request := factorysessionexecution.ResponseEventSubscriptionRequest{
		SessionID: sessionID,
	}
	var afterSequence int64
	if params.AfterSequence != nil {
		afterSequence = int64(*params.AfterSequence)
		if afterSequence < 0 {
			s.writeError(w, http.StatusBadRequest, "after_sequence must be non-negative", "INVALID_RESPONSE_EVENT_CURSOR")
			return factorysessionexecution.ResponseEventSubscriptionRequest{}, false
		}
	}
	request.AfterSequence = afterSequence
	var dispatchID string
	if params.DispatchId != nil {
		dispatchID = strings.TrimSpace(string(*params.DispatchId))
		if dispatchID == "" {
			s.writeError(w, http.StatusBadRequest, "dispatch_id must not be empty", "INVALID_RESPONSE_EVENT_FILTER")
			return factorysessionexecution.ResponseEventSubscriptionRequest{}, false
		}
	}
	request.DispatchID = dispatchID
	kinds, valid := responseEventKinds(params.Kind)
	if !valid {
		s.writeError(w, http.StatusBadRequest, "kind must contain only public FactoryResponseEventKind values", "INVALID_RESPONSE_EVENT_FILTER")
		return factorysessionexecution.ResponseEventSubscriptionRequest{}, false
	}
	request.Kinds = kinds
	return request, true
}

func responseEventKinds(values *factoryapi.ResponseEventKind) ([]factorysessionexecution.ResponseEventKind, bool) {
	if values == nil {
		return nil, true
	}
	if len(*values) == 0 {
		return nil, false
	}
	kinds := make([]factorysessionexecution.ResponseEventKind, 0, len(*values))
	for _, value := range *values {
		kind := factorysessionexecution.ResponseEventKind(value)
		switch kind {
		case factorysessionexecution.ResponseEventKindSession, factorysessionexecution.ResponseEventKindRun, factorysessionexecution.ResponseEventKindTurn,
			factorysessionexecution.ResponseEventKindMessage, factorysessionexecution.ResponseEventKindReasoning, factorysessionexecution.ResponseEventKindTool,
			factorysessionexecution.ResponseEventKindFileChange, factorysessionexecution.ResponseEventKindPlan, factorysessionexecution.ResponseEventKindProgress,
			factorysessionexecution.ResponseEventKindUsage, factorysessionexecution.ResponseEventKindError, factorysessionexecution.ResponseEventKindStreamGap:
			kinds = append(kinds, kind)
		default:
			return nil, false
		}
	}
	return kinds, true
}

func writeFactoryResponseEventSSE(w http.ResponseWriter, event apisurface.FactoryResponseEventRecord) error {
	_, err := fmt.Fprintf(w, "id: %s\ndata: %s\n\n", strconv.FormatInt(event.Sequence, 10), event.Data)
	return err
}

func requestsJSONEventRecoveryProbe(r *http.Request) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Accept"))), "application/json")
}

func (s *Server) probeFactorySessionEventStreamRecovery(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) {
	var err error
	if isDurableExecutionSessionID(sessionID) {
		reader, ok := s.requireDurableSessionEventsReader(w)
		if !ok {
			return
		}
		raw, prepareErr := factorysessionmapping.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{
			AfterEventId:  afterEventIDParam(reconnect),
			AfterSequence: afterSequenceParam(reconnect),
		})
		if prepareErr == nil {
			raw, prepareErr = s.sessionRequests.PrepareEventReconnect(raw)
		}
		if prepareErr != nil {
			err = prepareErr
		} else {
			err = reader.ProbeDurableFactorySessionEvents(r.Context(), sessionID, raw)
		}
	} else {
		workAPI, ok := s.requireWorkAPI(w)
		if !ok {
			return
		}
		err = workAPI.ProbeFactoryEventsForSession(r.Context(), sessionID, reconnect)
	}
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeJSON(w, http.StatusOK, staleCursorRecoveryResponse(sessionID, factoryapi.FactorySessionEventStreamRecoveryOutcomeUNKNOWNSESSION, false))
			return
		}
		if errors.Is(err, apisurface.ErrInvalidEventReconnectCursor) ||
			errors.Is(err, events.ErrReconnectCursorNotFound) ||
			errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
			s.writeJSON(w, http.StatusOK, staleCursorRecoveryResponse(sessionID, factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE, true))
			return
		}
		s.logger.Error("probe factory session event recovery failed", zap.String("session_id", sessionID), zap.Error(err))
		s.writeJSON(w, http.StatusOK, staleCursorRecoveryResponse(sessionID, factoryapi.FactorySessionEventStreamRecoveryOutcomeINTERNALERROR, false))
		return
	}
	s.writeJSON(w, http.StatusOK, staleCursorRecoveryResponse(sessionID, factoryapi.FactorySessionEventStreamRecoveryOutcomeSTREAMREADY, false))
}

func staleCursorRecoveryResponse(
	sessionID string,
	outcome factoryapi.FactorySessionEventStreamRecoveryOutcome,
	omitReconnectCursor bool,
) factoryapi.FactorySessionEventStreamRecovery {
	return factoryapi.FactorySessionEventStreamRecovery{
		FactorySessionId: sessionID,
		Outcome:          outcome,
		Retry: factoryapi.FactorySessionEventStreamRecoveryRetry{
			OmitAfterEventId:  omitReconnectCursor,
			OmitAfterSequence: omitReconnectCursor,
		},
	}
}

func (s *Server) GetFactorySessionSyncPreflightBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.GetFactorySessionSyncPreflightBySessionIdParams,
) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	response, err := sessionRuntime.GetFactorySessionSyncPreflight(
		r.Context(),
		string(sessionID),
		interfaces.FactorySessionSyncPreflightOptions{
			Reconnect:           reconnectCursorFromParams(params.AfterEventId, params.AfterSequence),
			BackendScopeID:      params.BackendScopeId,
			LogicalSessionKeyID: params.LogicalSessionKeyId,
		},
	)
	if err != nil {
		s.logger.Error("get factory session sync preflight failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get factory session sync preflight", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func reconnectCursorFromParams(afterEventID *factoryapi.AfterEventId, afterSequence *factoryapi.AfterSequence) *interfaces.FactoryEventReconnectCursor {
	if afterEventID == nil && afterSequence == nil {
		return nil
	}
	cursor := &interfaces.FactoryEventReconnectCursor{}
	if afterEventID != nil {
		cursor.AfterEventID = string(*afterEventID)
	}
	if afterSequence != nil {
		sequence := int(*afterSequence)
		cursor.AfterSequence = &sequence
	}
	return cursor
}

// ReconnectCursorFromParams maps generated reconnect query parameters into the
// canonical Factory Event cursor.
func ReconnectCursorFromParams(afterEventID *factoryapi.AfterEventId, afterSequence *factoryapi.AfterSequence) *interfaces.FactoryEventReconnectCursor {
	return reconnectCursorFromParams(afterEventID, afterSequence)
}

func afterEventIDParam(cursor *interfaces.FactoryEventReconnectCursor) *factoryapi.AfterEventId {
	if cursor == nil || strings.TrimSpace(cursor.AfterEventID) == "" {
		return nil
	}
	value := factoryapi.AfterEventId(cursor.AfterEventID)
	return &value
}

func afterSequenceParam(cursor *interfaces.FactoryEventReconnectCursor) *factoryapi.AfterSequence {
	if cursor == nil || cursor.AfterSequence == nil {
		return nil
	}
	value := factoryapi.AfterSequence(*cursor.AfterSequence)
	return &value
}

func writeSessionEventStreamHandshakeHeaders(
	w http.ResponseWriter,
	stream *interfaces.FactoryEventStream,
) {
	if backendScopeID := strings.TrimSpace(stream.BackendScopeID); backendScopeID != "" {
		w.Header().Set(sessionEventStreamBackendScopeHeader, backendScopeID)
	}
	if logicalSessionKeyID := strings.TrimSpace(stream.LogicalSessionKeyID); logicalSessionKeyID != "" {
		w.Header().Set(sessionEventStreamLogicalSessionKeyHeader, logicalSessionKeyID)
	}
	if factorySessionID := strings.TrimSpace(stream.FactorySessionID); factorySessionID != "" {
		w.Header().Set(sessionEventStreamFactorySessionHeader, factorySessionID)
	}
	if streamGenerationID := strings.TrimSpace(stream.StreamGenerationID); streamGenerationID != "" {
		w.Header().Set(sessionEventStreamGenerationHeader, streamGenerationID)
	}
}

func (s *Server) getEvents(
	w http.ResponseWriter,
	r *http.Request,
	includeSessionHandshake bool,
	subscribe func(context.Context) (*interfaces.FactoryEventStream, error),
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported", "INTERNAL_ERROR")
		return
	}

	stream, err := subscribe(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if errors.Is(err, apisurface.ErrInvalidEventReconnectCursor) || errors.Is(err, events.ErrReconnectCursorNotFound) {
			s.writeError(w, http.StatusBadRequest, "invalid event reconnect cursor", "BAD_REQUEST")
			return
		}
		s.logger.Error("subscribe factory events failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to subscribe to factory events", "INTERNAL_ERROR")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if includeSessionHandshake {
		writeSessionEventStreamHandshakeHeaders(w, stream)
	}

	for _, event := range stream.History {
		if err := s.writeSSEDataJSON(w, event); err != nil {
			s.logger.Debug("write historical factory event failed", zap.Error(err))
			return
		}
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-stream.Events:
			if !ok {
				return
			}
			if err := s.writeSSEDataJSON(w, event); err != nil {
				s.logger.Debug("write live factory event failed", zap.Error(err))
				return
			}
			flusher.Flush()
		}
	}
}

// StreamFactoryEvents writes one canonical Factory Event subscription as SSE.
func (s *Handler) StreamFactoryEvents(
	w http.ResponseWriter,
	r *http.Request,
	includeSessionHandshake bool,
	subscribe func(context.Context) (*interfaces.FactoryEventStream, error),
) {
	s.getEvents(w, r, includeSessionHandshake, subscribe)
}
