package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"go.uber.org/zap"
)

type durableSessionGetter interface {
	GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error)
}

type durableExecutionSessionLister interface {
	ListDurableExecutionSessions(
		context.Context,
		factorysessionexecution.ListSessionsRequest,
	) (factorysessionexecution.ListSessionsResult, error)
}

func (s *Server) requireDurableSessionGetter(w http.ResponseWriter) (durableSessionGetter, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session read is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	getter, ok := s.runtime.(durableSessionGetter)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session read is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return getter, true
}

func isDurableExecutionSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "dur-sess-")
}

func (s *Server) mergeScopedFactorySessionList(
	ctx context.Context,
	normalized factorysessionexecution.ListSessionsRequest,
) (factoryapi.ListFactorySessionsResponse, error) {
	needsWorkspaceLive := normalized.Scope == factorysessionexecution.SessionListScopeLive ||
		normalized.Scope == factorysessionexecution.SessionListScopeAll
	needsDurable := normalized.Scope == factorysessionexecution.SessionListScopePersisted ||
		normalized.Scope == factorysessionexecution.SessionListScopeAll ||
		normalized.Scope == factorysessionexecution.SessionListScopeLive

	var workspaceSessions []factoryapi.FactorySessionSummary
	if needsWorkspaceLive {
		if s.sessionRuntime == nil {
			return factoryapi.ListFactorySessionsResponse{}, errors.New("session runtime is unavailable")
		}
		liveResponse, err := s.sessionRuntime.ListFactorySessions(ctx)
		if err != nil {
			return factoryapi.ListFactorySessionsResponse{}, err
		}
		workspaceSessions = append([]factoryapi.FactorySessionSummary(nil), liveResponse.Sessions...)
	}

	var durableScoped factorysessionexecution.ListSessionsResult
	if needsDurable {
		if lister, ok := s.runtime.(durableExecutionSessionLister); ok {
			durableResult, err := lister.ListDurableExecutionSessions(ctx, factorysessionexecution.ListSessionsRequest{
				Scope: factorysessionexecution.SessionListScopeAll,
			})
			if err != nil {
				return factoryapi.ListFactorySessionsResponse{}, err
			}
			durableScoped = factorysessionexecution.ApplySessionListScope(factorysessionexecution.ListSessionsResult{
				Scope:           normalized.Scope,
				LiveSessions:    durableResult.LiveSessions,
				DurableSessions: durableResult.DurableSessions,
			}, normalized)
		}
	}

	durableAPI := factorysession.ListSessionsResponseToAPI(durableScoped)
	switch normalized.Scope {
	case factorysessionexecution.SessionListScopePersisted:
		return durableAPI, nil
	default:
		mergedSessions := append([]factoryapi.FactorySessionSummary(nil), workspaceSessions...)
		mergedSessions = append(mergedSessions, durableAPI.Sessions...)
		mergedSessions = sortFactorySessionSummaries(mergedSessions)
		response := durableAPI
		response.Sessions = mergedSessions
		if normalized.Scope == factorysessionexecution.SessionListScopeLive {
			response.DurableSessions = nil
		}
		return response, nil
	}
}

func sortFactorySessionSummaries(sessions []factoryapi.FactorySessionSummary) []factoryapi.FactorySessionSummary {
	if len(sessions) < 2 {
		return sessions
	}
	sorted := append([]factoryapi.FactorySessionSummary(nil), sessions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Compare(sorted[i].Id, sorted[j].Id) < 0
	})
	return sorted
}

func (s *Server) writeDurableSessionReadError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
		s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return true
	}
	var validationErr *factorysessionexecution.ValidationError
	if errors.As(err, &validationErr) {
		s.writeError(w, http.StatusBadRequest, validationErr.Message, "BAD_REQUEST")
		return true
	}
	return false
}

func (s *Server) writeDurableSessionListError(w http.ResponseWriter, err error) {
	if s.writeDurableSessionReadError(w, err) {
		return
	}
	s.logger.Error("list durable factory sessions failed", zap.Error(err))
	s.writeError(w, http.StatusInternalServerError, "failed to list factory sessions", "INTERNAL_ERROR")
}
