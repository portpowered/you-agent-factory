package api

import (
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func (s *Server) InvokeFactorySessionBySessionId(w http.ResponseWriter, _ *http.Request, _ factoryapi.SessionID) {
	s.writeError(w, http.StatusInternalServerError, "factory invocation API is not implemented", "INTERNAL_ERROR")
}
