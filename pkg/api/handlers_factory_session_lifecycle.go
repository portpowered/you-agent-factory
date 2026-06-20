package api

import (
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

func (s *Server) ApproveFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableApproveControl(w, r, sessionID, func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factoryapi.FactorySessionApproveRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.ApproveDurableFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) PauseFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "pause", func(
			lifecycle apisurface.DurableSessionLifecycleAPI,
			req factoryapi.FactorySessionLifecycleControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return lifecycle.PauseDurableFactorySession(r.Context(), string(sessionID), req)
		})
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, func(
		sessionRuntime apisurface.SessionAPI,
		req factoryapi.FactorySessionLifecycleControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return sessionRuntime.PauseLiveFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) ResumeFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "resume", func(
			lifecycle apisurface.DurableSessionLifecycleAPI,
			req factoryapi.FactorySessionLifecycleControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return lifecycle.ResumeDurableFactorySession(r.Context(), string(sessionID), req)
		})
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, func(
		sessionRuntime apisurface.SessionAPI,
		req factoryapi.FactorySessionLifecycleControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return sessionRuntime.ResumeLiveFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) CancelFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableLifecycleControl(w, r, sessionID, "cancel", func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factoryapi.FactorySessionLifecycleControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.CancelDurableFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) TerminateFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableLifecycleControl(w, r, sessionID, "terminate", func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factoryapi.FactorySessionLifecycleControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.TerminateDurableFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) RetryFactorySessionDispatch(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableRetryDispatchControl(w, r, sessionID, func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factoryapi.FactorySessionRetryDispatchRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.RetryDurableFactorySessionDispatch(r.Context(), string(sessionID), req)
	})
}
