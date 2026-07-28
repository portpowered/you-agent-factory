// Package http adapts the public HTTP model endpoints to the Models API role.
package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// Handler owns HTTP decoding, model API invocation, error mapping, and response
// encoding for the model endpoint family. Route registration remains in the
// top-level HTTP transport.
type Handler struct {
	adapter *Adapter
	logger  *zap.Logger
}

// NewHandler constructs the Models HTTP handler with its representation adapter.
func NewHandler(adapter *Adapter, logger *zap.Logger) *Handler {
	if adapter == nil || logger == nil {
		return nil
	}
	return &Handler{adapter: adapter, logger: logger}
}

func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	response, err := h.adapter.ListModels(r.Context())
	if err != nil {
		h.writeCatalogError(w, err, catalogListFailedMessage)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetModel(w http.ResponseWriter, r *http.Request, modelName string) {
	model, err := h.adapter.GetModel(r.Context(), modelName)
	if err != nil {
		h.writeCatalogError(w, err, catalogGetFailedMessage)
		return
	}
	h.writeJSON(w, http.StatusOK, model)
}

func (h *Handler) InvokeModel(w http.ResponseWriter, r *http.Request, modelName string) {
	req, err := decodeModelInvocationRequestFromHTTP(r.Body)
	if err != nil {
		message := "invalid request payload"
		var validationErr requestValidationError
		if errors.As(err, &validationErr) {
			message = validationErr.message
		}
		h.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
		return
	}
	if err := validateModelInvocationOperation(req); err != nil {
		var validationErr requestValidationError
		if errors.As(err, &validationErr) {
			h.writeError(w, http.StatusBadRequest, validationErr.message, "BAD_REQUEST")
			return
		}
	}

	result, err := h.adapter.InvokeModel(r.Context(), modelName, req)
	if err != nil {
		h.writeInvocationError(w, err)
		return
	}
	if strings.TrimSpace(result.StreamFile) != "" {
		if result.StreamContentType != "" {
			w.Header().Set("Content-Type", result.StreamContentType)
		}
		http.ServeFile(w, r, result.StreamFile)
		return
	}

	h.writeJSON(w, http.StatusOK, modelInvocationResponseFromResult(result))
}

func (h *Handler) writeInvocationError(w http.ResponseWriter, err error) {
	h.writeRootOrInternalError(w, modelsHTTPOperationInvoke, err, invokeFailedMessage)
}

func (h *Handler) PullModel(w http.ResponseWriter, r *http.Request, modelName string) {
	result, err := h.adapter.PullModel(r.Context(), modelName)
	if err != nil {
		if isModelPullError(err) {
			var pullErr *modelinference.PullError
			errors.As(err, &pullErr)
			h.writeJSON(w, managedRuntimePullHTTPStatus(pullErr.Result), modelPullResponseFromService(pullErr.Result))
			return
		}
		h.writeRootOrInternalError(w, modelsHTTPOperationPull, err, pullFailedMessage)
		return
	}
	h.writeJSON(w, http.StatusOK, modelPullResponseFromService(result))
}

func isModelPullError(err error) bool {
	var pullErr *modelinference.PullError
	return errors.As(err, &pullErr) && pullErr != nil
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
	case http.StatusConflict:
		return factoryapi.ErrorFamilyConflict
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}
