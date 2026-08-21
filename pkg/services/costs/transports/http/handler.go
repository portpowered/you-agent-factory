// Package http adapts the Costs service into the authored cost-report API.
// The adapter invokes the injected Costs operation; it does not read runtime
// artifacts, load configuration, or calculate monetary values.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// Handler owns HTTP error mapping and response encoding for Costs operations.
// Route registration remains in the top-level HTTP transport.
type Handler struct {
	adapter *Adapter
	logger  *zap.Logger
}

// NewHandler constructs the Costs HTTP handler with its injected adapter.
func NewHandler(adapter *Adapter, logger *zap.Logger) *Handler {
	if adapter == nil {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{adapter: adapter, logger: logger}
}

// GetMetricsCosts serves the typed cost-report operation.
func (h *Handler) GetMetricsCosts(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.GetMetricsCostsParams,
) {
	if h == nil || h.adapter == nil {
		response := factoryapi.ErrorResponse{
			Message: "Costs handler is unavailable",
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Code:    factoryapi.ErrorResponseCode("INTERNAL_ERROR"),
		}
		writeResponseJSON(w, http.StatusInternalServerError, response, zap.NewNop())
		return
	}
	sessionID := ""
	if params.SessionId != nil {
		sessionID = *params.SessionId
	}
	report, err := h.adapter.GetMetricsCosts(r.Context(), sessionID)
	if err != nil {
		h.writeQueryError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, report)
}

func (h *Handler) writeQueryError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "COSTS_QUERY_FAILED"
	message := "failed to query runtime costs"
	var queryErr *costs.QueryError
	if errors.As(err, &queryErr) && queryErr != nil {
		message = queryErr.Error()
		if queryErr.Kind == costs.QueryErrorInvalidInput {
			status = http.StatusBadRequest
			code = "COSTS_INVALID_REQUEST"
		}
	}
	h.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	writeResponseJSON(w, status, value, h.logger)
}

func writeResponseJSON(w http.ResponseWriter, status int, value any, logger *zap.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		// The response has already been committed, so only the structured log can
		// report an encoding failure without corrupting a partial success body.
		if logger != nil {
			logger.Error("encode Costs response failed", zap.Error(err))
		}
	}
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
	if status == http.StatusBadRequest {
		return factoryapi.ErrorFamilyBadRequest
	}
	return factoryapi.ErrorFamilyInternalServerError
}
