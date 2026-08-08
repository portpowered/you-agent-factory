package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// Handler owns request decoding, Worker Sessions root invocation, error
// mapping, and response encoding. Route registration remains top-level.
type Handler struct {
	adapter *Adapter
	logger  *zap.Logger
}

// NewHandler constructs the Worker Sessions HTTP handler.
func NewHandler(adapter *Adapter, logger *zap.Logger) *Handler {
	if adapter == nil || logger == nil {
		return nil
	}
	return &Handler{adapter: adapter, logger: logger}
}

// ListWorkerSessionsBySessionId handles the session-scoped Worker Sessions
// list operation.
func (h *Handler) ListWorkerSessionsBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ListWorkerSessionsBySessionIdParams,
) {
	if h == nil || h.adapter == nil {
		writeError(w, http.StatusInternalServerError, "Worker Sessions handler is unavailable", "INTERNAL_ERROR")
		return
	}
	if strings.TrimSpace(string(sessionID)) == "" {
		writeError(w, http.StatusBadRequest, "session id is required", "BAD_REQUEST")
		return
	}
	workID := strings.TrimSpace(params.WorkId)
	if workID == "" {
		writeError(w, http.StatusBadRequest, "workId is required", "BAD_REQUEST")
		return
	}
	if r == nil {
		writeError(w, http.StatusBadRequest, "request is required", "BAD_REQUEST")
		return
	}
	response, err := h.adapter.ListWorkerSessions(r.Context(), string(sessionID), workID)
	if err != nil {
		h.writeMappedError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) writeMappedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, work.ErrWorkNotFound):
		writeError(w, http.StatusNotFound, "work not found", "NOT_FOUND")
	case errors.Is(err, context.Canceled), errors.Is(err, workersessions.ErrObservationCanceled):
		return
	case strings.Contains(err.Error(), "session id is required"), strings.Contains(err.Error(), "work id is required"):
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
	default:
		writeError(w, http.StatusInternalServerError, "failed to list Worker Sessions", "INTERNAL_ERROR")
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && h.logger != nil {
		h.logger.Error("encode Worker Sessions response failed", zap.Error(err))
	}
}

func writeError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
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
