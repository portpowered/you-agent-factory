package api

import (
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"go.uber.org/zap"
)

func (s *Server) requireDurableSessionExecution(w http.ResponseWriter) (apisurface.DurableSessionExecutionAPI, bool) {
	if s.durableSessionExecution == nil {
		s.writeError(w, http.StatusNotImplemented, "durable factory session execution is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableSessionExecution, true
}

// StartDurableFactorySessionAsync handles POST /factory-sessions/async.
func (s *Server) StartDurableFactorySessionAsync(w http.ResponseWriter, r *http.Request) {
	execution, ok := s.requireDurableSessionExecution(w)
	if !ok {
		return
	}
	req, err := decodeStrictJSON[factoryapi.FactorySessionExecutionRequest](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	response, err := execution.StartDurableFactorySessionAsync(r.Context(), req)
	if err != nil {
		s.writeDurableSessionStartError(w, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, response)
}

// StartDurableFactorySessionSync handles POST /factory-sessions/sync.
func (s *Server) StartDurableFactorySessionSync(w http.ResponseWriter, r *http.Request) {
	execution, ok := s.requireDurableSessionExecution(w)
	if !ok {
		return
	}
	req, err := decodeStrictJSON[factoryapi.FactorySessionExecutionRequest](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	response, err := execution.StartDurableFactorySessionSync(r.Context(), req)
	if err != nil {
		s.writeDurableSessionStartError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) writeDurableSessionStartError(w http.ResponseWriter, err error) {
	var validationErr *apisurface.RequestValidationError
	if errors.As(err, &validationErr) {
		s.writeError(w, http.StatusBadRequest, validationErr.Error(), "BAD_REQUEST")
		return
	}
	var domainValidationErr *factorysessionexecution.ValidationError
	if errors.As(err, &domainValidationErr) {
		s.writeError(w, http.StatusBadRequest, domainValidationErr.Error(), "BAD_REQUEST")
		return
	}
	if errors.Is(err, factorysessionexecution.ErrExecutionRequestIDConflict) {
		s.writeError(w, http.StatusConflict, "requestId was reused with a different execution request", "EXECUTION_REQUEST_ID_CONFLICT")
		return
	}
	s.logger.Error("durable factory session start failed", zap.Error(err))
	s.writeError(w, http.StatusInternalServerError, "failed to start durable factory session", "INTERNAL_ERROR")
}
