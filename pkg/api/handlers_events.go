package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

const sessionEventStreamGenerationHeader = "X-Factory-Session-Stream-Generation-Id"

// GetStatus handles GET /status as the supported runtime status read model.
func (s *Server) GetStatus(w http.ResponseWriter, r *http.Request) {
	s.getStatus(w, r, s.runtime.GetEngineStateSnapshot)
}

func (s *Server) GetStatusBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getStatus(w, r, func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(sessionID))
	})
}

func (s *Server) getStatus(
	w http.ResponseWriter,
	r *http.Request,
	loadSnapshot func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error),
) {
	snapshot, err := loadSnapshot(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get engine state snapshot failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get engine state snapshot", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusOK, statusFromEngineStateSnapshot(*snapshot))
}

// GetEvents handles compatibility-only process-global GET /events.
func (s *Server) GetEvents(w http.ResponseWriter, r *http.Request, params factoryapi.GetEventsParams) {
	reconnect := reconnectCursorFromParams(params.AfterEventId, params.AfterSequence)
	s.getEvents(w, r, false, func(ctx context.Context) (*interfaces.FactoryEventStream, error) {
		return s.runtime.SubscribeFactoryEvents(ctx, reconnect, interfaces.FactoryEventReconnectScope{})
	})
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
		stream, err := reader.ReadDurableFactorySessionEvents(r.Context(), string(sessionID), params)
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

	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getEvents(w, r, true, func(ctx context.Context) (*interfaces.FactoryEventStream, error) {
		return sessionRuntime.SubscribeFactoryEventsForSession(ctx, string(sessionID), reconnect)
	})
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
	probeCtx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var err error
	if isDurableExecutionSessionID(sessionID) {
		reader, ok := s.requireDurableSessionEventsReader(w)
		if !ok {
			return
		}
		_, err = reader.ReadDurableFactorySessionEvents(probeCtx, sessionID, factoryapi.GetEventsBySessionIdParams{
			AfterEventId: afterEventIDParam(reconnect),
			AfterSequence: afterSequenceParam(reconnect),
		})
	} else {
		sessionRuntime, ok := s.requireSessionRuntime(w)
		if !ok {
			return
		}
		_, err = sessionRuntime.SubscribeFactoryEventsForSession(probeCtx, sessionID, reconnect)
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
		reconnectCursorFromParams(params.AfterEventId, params.AfterSequence),
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
		if streamGenerationID := strings.TrimSpace(stream.StreamGenerationID); streamGenerationID != "" {
			w.Header().Set(sessionEventStreamGenerationHeader, streamGenerationID)
		}
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

func tokenToResponse(t *interfaces.Token, includeHistory bool) factoryapi.TokenResponse {
	resp := factoryapi.TokenResponse{
		Id:                       t.ID,
		PlaceId:                  t.PlaceID,
		WorkId:                   t.Color.WorkID,
		WorkType:                 t.Color.WorkTypeID,
		ChainingTraceDepth:       intPtrIfPositive(t.Color.ChainingTraceDepth),
		CurrentChainingTraceId:   stringPtrIfNotEmpty(firstNonEmptyString(t.Color.CurrentChainingTraceID, t.Color.TraceID)),
		PreviousChainingTraceIds: stringSlicePtrCopy(t.Color.PreviousChainingTraceIDs),
		TraceId:                  t.Color.TraceID,
		Content:                  domainWorkContentToGeneratedPtr(t.Color.Content),
		Tags:                     stringMapPtr(t.Color.Tags),
		CreatedAt:                t.CreatedAt,
		EnteredAt:                t.EnteredAt,
	}
	if t.Color.Name != "" {
		resp.Name = &t.Color.Name
	}
	if len(t.Color.Tags) == 0 {
		resp.Tags = nil
	}
	if includeHistory {
		resp.History = &factoryapi.TokenHistory{
			TotalVisits:         integerMapPtr(t.History.TotalVisits),
			ConsecutiveFailures: integerMapPtr(t.History.ConsecutiveFailures),
			PlaceVisits:         integerMapPtr(t.History.PlaceVisits),
			LastError:           stringPtrIfNotEmpty(t.History.LastError),
		}
	}
	return resp
}

func statusFromEngineStateSnapshot(snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) factoryapi.StatusResponse {
	categories, resources := categorizeStatusTokens(&snapshot.Marking, snapshot.Topology)
	response := factoryapi.StatusResponse{
		Categories:    categories,
		FactoryState:  snapshot.FactoryState,
		Resources:     resourceUsagePtr(resources),
		RuntimeStatus: string(snapshot.RuntimeStatus),
		TotalTokens:   countPublicStatusTokens(&snapshot.Marking),
	}
	if lifecycleControlStatus := strings.TrimSpace(snapshot.LifecycleControlStatus); lifecycleControlStatus != "" {
		status := factoryapi.FactorySessionDurableLifecycleStatus(lifecycleControlStatus)
		response.LifecycleControlStatus = &status
	}
	return response
}

func categorizeStatusTokens(marking *petri.MarkingSnapshot, net *state.Net) (factoryapi.StatusCategories, []factoryapi.ResourceUsage) {
	var categories factoryapi.StatusCategories
	resourceCounts := make(map[string]int)
	resourceTotals := resourceTotalsFromTopology(net)

	if marking == nil {
		return categories, resourceUsage(resourceCounts, resourceTotals)
	}

	for _, token := range marking.Tokens {
		if token == nil {
			continue
		}
		if interfaces.IsSystemTimeToken(token) {
			continue
		}

		if token.Color.DataType == interfaces.DataTypeResource {
			resourceID, resourceState := state.SplitPlaceID(token.PlaceID)
			if _, ok := resourceTotals[resourceID]; !ok {
				resourceTotals[resourceID]++
			}
			if resourceState == interfaces.ResourceStateAvailable {
				resourceCounts[resourceID]++
			}
			continue
		}

		switch statusStateCategory(net, token.PlaceID) {
		case state.StateCategoryFailed:
			categories.Failed++
		case state.StateCategoryTerminal:
			categories.Terminal++
		case state.StateCategoryInitial:
			categories.Initial++
		default:
			categories.Processing++
		}
	}

	return categories, resourceUsage(resourceCounts, resourceTotals)
}

func countPublicStatusTokens(marking *petri.MarkingSnapshot) int {
	if marking == nil {
		return 0
	}
	count := 0
	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		count++
	}
	return count
}

func statusStateCategory(net *state.Net, placeID string) state.StateCategory {
	if net == nil {
		return state.StateCategoryProcessing
	}
	return net.StateCategoryForPlace(placeID)
}

func resourceTotalsFromTopology(net *state.Net) map[string]int {
	totals := make(map[string]int)
	if net == nil {
		return totals
	}
	for id, resource := range net.Resources {
		if resource == nil {
			continue
		}
		totals[id] = resource.Capacity
	}
	return totals
}

func resourceUsage(counts map[string]int, totals map[string]int) []factoryapi.ResourceUsage {
	ids := make([]string, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	resources := make([]factoryapi.ResourceUsage, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, factoryapi.ResourceUsage{
			Available: counts[id],
			Name:      id,
			Total:     totals[id],
		})
	}
	return resources
}

func resourceUsagePtr(values []factoryapi.ResourceUsage) *[]factoryapi.ResourceUsage {
	if len(values) == 0 {
		return nil
	}
	return &values
}
