package api

import (
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func (s *Server) MoveWork(w http.ResponseWriter, r *http.Request, id factoryapi.WorkOrTokenID) {
	s.writeError(w, http.StatusNotImplemented, "work move is not implemented yet", "NOT_IMPLEMENTED")
}

func (s *Server) MoveWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	s.writeError(w, http.StatusNotImplemented, "work move is not implemented yet", "NOT_IMPLEMENTED")
}
