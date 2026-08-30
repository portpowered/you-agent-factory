// Package http adapts the Costs service into the authored cost-report API.
// The adapter invokes the injected Costs operation; it does not read runtime
// artifacts, load configuration, or calculate monetary values.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const (
	// DefaultQueryTimeout bounds one Costs HTTP request. It is shorter than the
	// standard CLI HTTP timeout so the server can return a typed response before
	// a client-side transport deadline expires.
	DefaultQueryTimeout = 8 * time.Second

	costsQueryCanceledCode   = "COSTS_QUERY_CANCELED"
	costsQueryFailedCode     = "COSTS_QUERY_FAILED"
	costsInvalidRequestCode  = "COSTS_INVALID_REQUEST"
	costsQueryTimeoutCode    = "COSTS_QUERY_TIMEOUT"
	costsSessionNotFoundCode = "METRICS_SESSION_NOT_FOUND"
)

type costsScopeError struct {
	sessionID string
	cause     error
}

func (err *costsScopeError) Error() string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf(
		"Factory Session %q was not found; use `you session list --scope live` to choose a live ID",
		err.sessionID,
	)
}

func (err *costsScopeError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func newCostsSessionNotFoundError(sessionID string, cause error) error {
	return &costsScopeError{sessionID: strings.TrimSpace(sessionID), cause: cause}
}

// Handler owns HTTP error mapping and response encoding for Costs operations.
// Route registration remains in the top-level HTTP transport.
type Handler struct {
	adapter      *Adapter
	logger       *zap.Logger
	queryTimeout time.Duration
}

// NewHandler constructs the Costs HTTP handler with its injected adapter.
func NewHandler(adapter *Adapter, logger *zap.Logger) *Handler {
	return NewHandlerWithQueryTimeout(adapter, logger, DefaultQueryTimeout)
}

// NewHandlerWithQueryTimeout constructs the Costs HTTP handler with an
// explicit server-side completion bound. Tests and isolated hosts can choose a
// shorter bound; production uses DefaultQueryTimeout through NewHandler.
func NewHandlerWithQueryTimeout(adapter *Adapter, logger *zap.Logger, queryTimeout time.Duration) *Handler {
	if adapter == nil {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if queryTimeout <= 0 {
		queryTimeout = DefaultQueryTimeout
	}
	return &Handler{adapter: adapter, logger: logger, queryTimeout: queryTimeout}
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
	if err := r.Context().Err(); err != nil {
		h.writeQueryError(w, err)
		return
	}
	sessionID := ""
	if params.SessionId != nil {
		sessionID = *params.SessionId
	}
	queryTimeout := h.queryTimeout
	if queryTimeout <= 0 {
		queryTimeout = DefaultQueryTimeout
	}
	queryContext, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()
	report, err := h.adapter.GetMetricsCosts(queryContext, sessionID)
	if err != nil {
		h.writeQueryError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, report)
}

func (h *Handler) writeQueryError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := costsQueryFailedCode
	message := "failed to query runtime costs"
	var scopeErr *costsScopeError
	if errors.As(err, &scopeErr) && scopeErr != nil {
		h.writeJSON(w, http.StatusNotFound, factoryapi.ErrorResponse{
			Message: scopeErr.Error(),
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCode(costsSessionNotFoundCode),
		})
		return
	}
	switch {
	case errors.Is(err, context.Canceled):
		status = http.StatusRequestTimeout
		code = costsQueryCanceledCode
		message = "metrics costs query was canceled before completion"
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
		code = costsQueryTimeoutCode
		queryTimeout := h.queryTimeout
		if queryTimeout <= 0 {
			queryTimeout = DefaultQueryTimeout
		}
		message = fmt.Sprintf("metrics costs query exceeded the server timeout of %s; narrow the session scope or retry", queryTimeout)
	}
	var queryErr *costs.QueryError
	if status == http.StatusInternalServerError && errors.As(err, &queryErr) && queryErr != nil {
		message = queryErr.Error()
		if queryErr.Kind == costs.QueryErrorInvalidInput {
			status = http.StatusBadRequest
			code = costsInvalidRequestCode
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
	if status == http.StatusNotFound {
		return factoryapi.ErrorFamilyNotFound
	}
	return factoryapi.ErrorFamilyInternalServerError
}
