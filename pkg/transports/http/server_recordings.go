package http

import (
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// GetEventsBySessionId routes canonical Factory Event history to Recordings.
// The Sessions fallback remains available for standalone durable-execution
// bindings that use the compatibility NewServer constructor.
func (s *Server) GetEventsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.GetEventsBySessionIdParams,
) {
	if s != nil && s.recordingsHTTP != nil {
		s.recordingsHTTP.GetEventsBySessionId(w, r, sessionID, params)
		return
	}
	if s != nil && s.factorySessionsAdapter != nil && s.factorySessionsAdapter.Adapter != nil {
		s.factorySessionsAdapter.GetEventsBySessionId(w, r, sessionID, params)
		return
	}
	s.writeError(w, http.StatusInternalServerError, "Recordings handler is unavailable", "INTERNAL_ERROR")
}

// GetFactorySessionResults routes durable historical result projection to
// Recordings while preserving the existing Sessions compatibility path.
func (s *Server) GetFactorySessionResults(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.GetFactorySessionResultsParams,
) {
	if s != nil && s.recordingsHTTP != nil {
		s.recordingsHTTP.GetFactorySessionResults(w, r, sessionID, params)
		return
	}
	if s != nil && s.factorySessionsAdapter != nil && s.factorySessionsAdapter.Adapter != nil {
		s.factorySessionsAdapter.GetFactorySessionResults(w, r, sessionID, params)
		return
	}
	s.writeError(w, http.StatusInternalServerError, "Recordings handler is unavailable", "INTERNAL_ERROR")
}

// ListFactorySessionDispatches routes durable dispatch history to Recordings.
func (s *Server) ListFactorySessionDispatches(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ListFactorySessionDispatchesParams,
) {
	if s != nil && s.recordingsHTTP != nil {
		s.recordingsHTTP.ListFactorySessionDispatches(w, r, sessionID, params)
		return
	}
	if s != nil && s.factorySessionsAdapter != nil && s.factorySessionsAdapter.Adapter != nil {
		s.factorySessionsAdapter.ListFactorySessionDispatches(w, r, sessionID, params)
		return
	}
	s.writeError(w, http.StatusInternalServerError, "Recordings handler is unavailable", "INTERNAL_ERROR")
}

// GetFactorySessionDispatch routes one durable dispatch projection to
// Recordings.
func (s *Server) GetFactorySessionDispatch(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	dispatchID factoryapi.DispatchID,
) {
	if s != nil && s.recordingsHTTP != nil {
		s.recordingsHTTP.GetFactorySessionDispatch(w, r, sessionID, dispatchID)
		return
	}
	if s != nil && s.factorySessionsAdapter != nil && s.factorySessionsAdapter.Adapter != nil {
		s.factorySessionsAdapter.GetFactorySessionDispatch(w, r, sessionID, dispatchID)
		return
	}
	s.writeError(w, http.StatusInternalServerError, "Recordings handler is unavailable", "INTERNAL_ERROR")
}

// ListFactorySessionArtifacts routes durable artifact history to Recordings.
func (s *Server) ListFactorySessionArtifacts(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	if s != nil && s.recordingsHTTP != nil {
		s.recordingsHTTP.ListFactorySessionArtifacts(w, r, sessionID)
		return
	}
	if s != nil && s.factorySessionsAdapter != nil && s.factorySessionsAdapter.Adapter != nil {
		s.factorySessionsAdapter.ListFactorySessionArtifacts(w, r, sessionID)
		return
	}
	s.writeError(w, http.StatusInternalServerError, "Recordings handler is unavailable", "INTERNAL_ERROR")
}

// GetFactorySessionArtifact routes one durable artifact projection to
// Recordings.
func (s *Server) GetFactorySessionArtifact(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	artifactID factoryapi.ArtifactID,
) {
	if s != nil && s.recordingsHTTP != nil {
		s.recordingsHTTP.GetFactorySessionArtifact(w, r, sessionID, artifactID)
		return
	}
	if s != nil && s.factorySessionsAdapter != nil && s.factorySessionsAdapter.Adapter != nil {
		s.factorySessionsAdapter.GetFactorySessionArtifact(w, r, sessionID, artifactID)
		return
	}
	s.writeError(w, http.StatusInternalServerError, "Recordings handler is unavailable", "INTERNAL_ERROR")
}
