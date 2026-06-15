package api

import (
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
)

func (s *Server) requireDurableExecutionAPI(w http.ResponseWriter) (apisurface.DurableSessionExecutionAPI, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	execution, ok := s.runtime.(apisurface.DurableSessionExecutionAPI)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session execution is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return execution, true
}

func (s *Server) writeDurableExecutionError(w http.ResponseWriter, err error) bool {
	if status, response, ok := factorysession.ExecutionErrorResponse(err); ok {
		s.writeJSON(w, status, response)
		return true
	}
	return false
}

func (s *Server) StartDurableFactorySessionAsync(w http.ResponseWriter, r *http.Request) {
	execution, ok := s.requireDurableExecutionAPI(w)
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
		if s.writeDurableExecutionError(w, err) {
			return
		}
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution failed", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) StartDurableFactorySessionSync(w http.ResponseWriter, r *http.Request) {
	execution, ok := s.requireDurableExecutionAPI(w)
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
		if s.writeDurableExecutionError(w, err) {
			return
		}
		s.writeError(w, http.StatusInternalServerError, "durable factory session execution failed", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}
