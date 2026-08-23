package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const factoryDefinitionsHTTPBoundary = "factory_definitions.http"

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("encode response failed", zap.Error(err))
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message, code string) {
	s.writeErrorWithTargets(w, status, message, code, nil)
}

func (s *Server) writeErrorWithTargets(
	w http.ResponseWriter,
	status int,
	message, code string,
	targets []factoryapi.FactoryValidationTarget,
) {
	var targetPtr *[]factoryapi.FactoryValidationTarget
	if len(targets) > 0 {
		targetPtr = &targets
	}
	s.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
		Targets: targetPtr,
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
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}

func requestFieldValidationMessage(err error) (string, bool) {
	var validationErr httpcompat.RequestFieldValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Message, true
	}
	return "", false
}

func decodeJSONWithDiagnostics[T any](body io.Reader) (httpcompat.DecodeResult[T], error) {
	return httpcompat.Decode[T](body)
}

func (s *Adapter) writeCompatibilityWarning(w http.ResponseWriter, operation string, paths []string) {
	httpcompat.ApplyWarning(w, s.logger, factoryDefinitionsHTTPBoundary, operation, paths)
}
