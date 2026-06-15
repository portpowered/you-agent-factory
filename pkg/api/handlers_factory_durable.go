package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"go.uber.org/zap"
)

func (s *Server) requireDurableSessionLifecycleAPI(w http.ResponseWriter) (apisurface.DurableSessionLifecycleAPI, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session lifecycle control is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	lifecycle, ok := s.runtime.(apisurface.DurableSessionLifecycleAPI)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session lifecycle control is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return lifecycle, true
}

func (s *Server) writeDurableLifecycleControlError(w http.ResponseWriter, sessionID string, err error) bool {
	if status, response, ok := factorysession.LifecycleControlErrorResponse(sessionID, err); ok {
		s.writeJSON(w, status, response)
		return true
	}
	return false
}

func (s *Server) handleDurableLifecycleControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	operation string,
	invoke func(apisurface.DurableSessionLifecycleAPI, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotImplemented, "durable factory session "+operation+" is not implemented", "INTERNAL_ERROR")
		return
	}

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	req, err := decodeOptionalLifecycleControlRequest(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	response, err := invoke(lifecycle, req)
	if err != nil {
		if s.writeDurableLifecycleControlError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("durable factory session lifecycle control failed",
			zap.Error(err),
			zap.String("session_id", string(sessionID)),
			zap.String("operation", operation),
		)
		s.writeError(w, http.StatusInternalServerError, "durable factory session lifecycle control failed", "INTERNAL_ERROR")
		return
	}

	status := http.StatusOK
	if response.Outcome == factoryapi.FactorySessionLifecycleControlOutcomeAccepted &&
		response.Status == factoryapi.FactorySessionDurableLifecycleStatusCanceling {
		status = http.StatusAccepted
	}
	s.writeJSON(w, status, response)
}

func decodeOptionalLifecycleControlRequest(body io.Reader) (factoryapi.FactorySessionLifecycleControlRequest, error) {
	return decodeOptionalJSONRequest(body, func() factoryapi.FactorySessionLifecycleControlRequest {
		return factoryapi.FactorySessionLifecycleControlRequest{}
	})
}

func decodeOptionalApproveRequest(body io.Reader) (factoryapi.FactorySessionApproveRequest, error) {
	return decodeOptionalJSONRequest(body, func() factoryapi.FactorySessionApproveRequest {
		return factoryapi.FactorySessionApproveRequest{}
	})
}

func decodeOptionalRetryDispatchRequest(body io.Reader) (factoryapi.FactorySessionRetryDispatchRequest, error) {
	return decodeOptionalJSONRequest(body, func() factoryapi.FactorySessionRetryDispatchRequest {
		return factoryapi.FactorySessionRetryDispatchRequest{}
	})
}

func decodeOptionalJSONRequest[T any](body io.Reader, zero func() T) (T, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return zero(), err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return zero(), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var req T
	if err := decoder.Decode(&req); err != nil {
		return zero(), err
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return zero(), err
	}
	return req, nil
}

func (s *Server) handleDurableApproveControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	invoke func(apisurface.DurableSessionLifecycleAPI, factoryapi.FactorySessionApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotImplemented, "durable factory session approve is not implemented", "INTERNAL_ERROR")
		return
	}

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	req, err := decodeOptionalApproveRequest(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	response, err := invoke(lifecycle, req)
	if err != nil {
		if s.writeDurableLifecycleControlError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("durable factory session approve failed",
			zap.Error(err),
			zap.String("session_id", string(sessionID)),
		)
		s.writeError(w, http.StatusInternalServerError, "durable factory session approve failed", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleDurableRetryDispatchControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	invoke func(apisurface.DurableSessionLifecycleAPI, factoryapi.FactorySessionRetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotImplemented, "durable factory session retry-dispatch is not implemented", "INTERNAL_ERROR")
		return
	}

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	req, err := decodeOptionalRetryDispatchRequest(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	response, err := invoke(lifecycle, req)
	if err != nil {
		if s.writeDurableLifecycleControlError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("durable factory session retry-dispatch failed",
			zap.Error(err),
			zap.String("session_id", string(sessionID)),
		)
		s.writeError(w, http.StatusInternalServerError, "durable factory session retry-dispatch failed", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusOK, response)
}
