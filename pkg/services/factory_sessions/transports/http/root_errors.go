package http

import (
	"errors"
	"net/http"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// sessionsRootErrorResponse maps typed Sessions root failures to HTTP status and a
// public response body. sessionID may be empty when the operation is not
// session-scoped.
func sessionsRootErrorResponse(sessionID string, err error) (int, any, bool) {
	if err == nil {
		return 0, nil, false
	}

	if status, response, ok := factorysession.LifecycleControlErrorResponse(sessionID, err); ok {
		return status, response, true
	}
	if status, response, ok := factorysession.ExecutionErrorResponse(err); ok {
		return status, response, true
	}
	if status, response, ok := sessionsRootNotFoundErrorResponse(err); ok {
		return status, response, true
	}

	var validationErr *factorysessions.ExecutionValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: validationErr.Message,
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCodeBADREQUEST,
		}, true
	}

	return 0, nil, false
}

func sessionsRootNotFoundErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	switch {
	case errors.Is(err, factorysessions.ErrSessionNotFound),
		errors.Is(err, factorysessions.ErrDurableSessionNotFound):
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "factory session not found",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		}, true
	case errors.Is(err, factorysessions.ErrDispatchNotFound):
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "dispatch not found",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		}, true
	case errors.Is(err, factorysessions.ErrArtifactNotFound):
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "factory session artifact not found",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		}, true
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func (s *Server) writeSessionsRootError(w http.ResponseWriter, sessionID string, err error) bool {
	if status, response, ok := sessionsRootErrorResponse(sessionID, err); ok {
		s.writeJSON(w, status, response)
		return true
	}
	return false
}

func (s *Server) writeSessionsRootErrorOrInternal(w http.ResponseWriter, sessionID string, err error, fallbackMessage string) {
	if s.writeSessionsRootError(w, sessionID, err) {
		return
	}
	message := strings.TrimSpace(fallbackMessage)
	if message == "" {
		message = "factory session request failed"
	}
	s.writeError(w, http.StatusInternalServerError, message, string(factoryapi.ErrorResponseCodeINTERNALERROR))
}
