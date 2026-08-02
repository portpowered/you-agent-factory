package http

import (
	"errors"
	"io"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func (s *Server) InvokeFactorySessionBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	req, err := decodeInvocationRequestBody(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if s.invocation == nil {
		s.writeError(w, http.StatusInternalServerError, "session invocation API is unavailable", "INTERNAL_ERROR")
		return
	}

	result, err := s.invocation.InvokeFactorySession(r.Context(), string(sessionID), req)
	if err != nil {
		switch typed := err.(type) {
		case *work.InputError:
			s.writeError(w, http.StatusBadRequest, typed.Message, string(typed.Code))
		case *work.ArgumentError:
			s.writeError(w, http.StatusBadRequest, typed.Message, string(typed.Code))
		case *apisurface.RequestValidationError:
			s.writeError(w, http.StatusBadRequest, typed.Message, "BAD_REQUEST")
		default:
			if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
				s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
				return
			}
			s.logger.Error("invoke factory session failed", zap.Error(err), zap.String("session_id", string(sessionID)))
			s.writeError(w, http.StatusInternalServerError, "failed to invoke factory session", "INTERNAL_ERROR")
		}
		return
	}

	s.writeJSON(w, http.StatusOK, apisurface.InvocationResponseFromResult(result))
}

func decodeInvocationRequestBody(body io.Reader) (factoryapi.InvokeFactorySessionBySessionIdJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.InvokeFactorySessionBySessionIdJSONRequestBody](body)
}
