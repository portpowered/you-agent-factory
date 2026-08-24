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

const factoryVisualizationHTTPBoundary = "factory_visualization.http"

func (a *Adapter) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		a.logger.Error("encode response failed", zap.Error(err))
	}
}

func (a *Adapter) writeError(w http.ResponseWriter, status int, message, code string) {
	a.writeJSON(w, status, factoryapi.ErrorResponse{
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

func decodeOptionalJSONWithDiagnostics[T any](body io.Reader, zero func() T) (httpcompat.DecodeResult[T], error) {
	return httpcompat.DecodeOptional(body, zero)
}

func (a *Adapter) writeCompatibilityWarning(w http.ResponseWriter, operation string, paths []string) {
	httpcompat.ApplyWarning(w, a.logger, factoryVisualizationHTTPBoundary, operation, paths)
}
