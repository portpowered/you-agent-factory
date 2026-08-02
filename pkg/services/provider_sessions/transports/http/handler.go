package http

import (
	"encoding/json"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// Handler owns HTTP decoding, Provider Sessions root invocation, and response
// encoding for owned Provider Sessions HTTP operations. Route registration
// remains in the top-level HTTP transport.
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
	r *http.Request,
	params factoryapi.GetProviderSessionDetailsParams,
) {
	response, err := h.adapter.GetProviderSessionDetails(r.Context(), params)
	if err != nil {
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
