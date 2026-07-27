package http

import (
	"encoding/json"
	"net/http"

	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// DurableExecutionSessionLister remains an API-construction alias while the
// owning contract lives with the Factory Sessions HTTP adapter.
type DurableExecutionSessionLister = factorysessionshttp.DurableExecutionSessionLister

// Top-level response helpers are used only by protocol composition endpoints
// that are not owned by the Factory Sessions adapter.
func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		s.logger.Error("encode response failed", zap.Error(err))
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message, code string) {
	s.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	})
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusConflict:
		return factoryapi.ErrorFamilyConflict
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	case http.StatusMethodNotAllowed:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusGone:
		return factoryapi.ErrorFamilyGone
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}
