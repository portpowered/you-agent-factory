package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"go.uber.org/zap"
)

func isDurableExecutionSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "dur-sess-")
}

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
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlRequest{}, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return factoryapi.FactorySessionLifecycleControlRequest{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var req factoryapi.FactorySessionLifecycleControlRequest
	if err := decoder.Decode(&req); err != nil {
		return factoryapi.FactorySessionLifecycleControlRequest{}, err
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return factoryapi.FactorySessionLifecycleControlRequest{}, err
	}
	return req, nil
}
