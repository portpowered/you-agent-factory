package http

import (
	"encoding/json"
	"errors"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// Handler owns HTTP decoding, Provider Sessions root invocation, and response
// encoding for owned Provider Sessions HTTP operations. Route registration
// remains in the top-level HTTP transport until PSS-I02 fan-in.
type Handler struct {
	adapter *Adapter
	logger  *zap.Logger
}

// NewHandler constructs the Provider Sessions HTTP handler with its adapter.
func NewHandler(adapter *Adapter, logger *zap.Logger) *Handler {
	if adapter == nil || logger == nil {
		return nil
	}
	return &Handler{adapter: adapter, logger: logger}
}

func (h *Handler) GetProviderSessionDetails(
	w http.ResponseWriter,
	_ *http.Request,
	params factoryapi.GetProviderSessionDetailsParams,
) {
	response, err := h.adapter.GetProviderSessionDetails(params)
	if err != nil {
		var validationErr requestValidationError
		if errors.As(err, &validationErr) {
			h.writeError(w, http.StatusBadRequest, validationErr.message, "BAD_REQUEST")
			return
		}
		h.writeProviderSessionError(w, params, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.Error("encode response failed", zap.Error(err))
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message, code string) {
	h.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	})
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}
