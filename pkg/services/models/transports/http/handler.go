// Package http adapts the public HTTP model endpoints to the Models API role.
package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
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
		h.logger.Error("list models failed", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to list models", "INTERNAL_ERROR")
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetModel(w http.ResponseWriter, r *http.Request, modelName string) {
	model, err := h.adapter.GetModel(r.Context(), modelName)
	if err != nil {
		if errors.Is(err, modelinference.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
			return
		}
		h.logger.Error("get model failed", zap.Error(err), zap.String("model_name", modelName))
		h.writeError(w, http.StatusInternalServerError, "failed to load model", "INTERNAL_ERROR")
		return
	}
	h.writeJSON(w, http.StatusOK, model)
}

func (h *Handler) InvokeModel(w http.ResponseWriter, r *http.Request, modelName string) {
	req, err := decodeModelInvocationRequestBody(r.Body)
	if err != nil {
		message := "invalid request payload"
		var validationErr requestValidationError
		if errors.As(err, &validationErr) {
			message = validationErr.message
		}
		h.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.Operation) == "" {
		h.writeError(w, http.StatusBadRequest, "operation is required", "BAD_REQUEST")
		return
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

	h.writeJSON(w, http.StatusOK, factoryapi.ModelInvocationResponse{
		ModelName:        result.ModelName,
		Worker:           result.Worker,
		Operation:        result.Operation,
		ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Content:          derefGeneratedWorkContent(contentcontract.GeneratedPtrFromParts(result.Content)),
		Bindings:         generatedResolvedModelInvocationBindings(result.Bindings),
	})
}

func (h *Handler) writeInvocationError(w http.ResponseWriter, err error) {
	if failure, ok := workers.AsInferenceFailure(err); ok {
		h.writeError(w, inferenceFailureHTTPStatus(failure), failure.Error(), inferenceFailureErrorCode(failure))
		return
	}
	switch {
	case errors.Is(err, modelinference.ErrNotFound):
		h.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
	case errors.Is(err, modelinference.ErrMissing), errors.Is(err, modelinference.ErrNotAvailable):
		h.writeError(w, http.StatusNotFound, err.Error(), "MODEL_NOT_AVAILABLE")
	case errors.Is(err, modelinference.ErrLoading):
		h.writeError(w, http.StatusConflict, err.Error(), "MODEL_RUNTIME_LOADING")
	case errors.Is(err, modelinference.ErrFailed):
		h.writeError(w, http.StatusServiceUnavailable, err.Error(), "MODEL_RUNTIME_FAILED")
	case errors.Is(err, modelinference.ErrUnsupported):
		h.writeError(w, http.StatusBadRequest, err.Error(), "MODEL_RUNTIME_UNSUPPORTED")
	case errors.Is(err, modelinference.ErrUnsupportedOperation), errors.Is(err, modelinference.ErrUnsupportedResponseMode):
		h.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
	default:
		h.writeError(w, http.StatusBadRequest, strings.TrimSpace(err.Error()), "BAD_REQUEST")
	}
}

func (h *Handler) PullModel(w http.ResponseWriter, r *http.Request, modelName string) {
	result, err := h.adapter.PullModel(r.Context(), modelName)
	if err != nil {
		switch {
		case errors.Is(err, modelinference.ErrNotFound):
			h.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
		case errors.Is(err, modelinference.ErrPullUnsupported):
			h.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		case isModelPullError(err):
			var pullErr *modelinference.PullError
			errors.As(err, &pullErr)
			h.writeJSON(w, managedRuntimePullHTTPStatus(pullErr.Result), modelPullResponseFromService(pullErr.Result))
		default:
			h.writeError(w, http.StatusInternalServerError, strings.TrimSpace(err.Error()), "INTERNAL_ERROR")
		}
		return
	}
	h.writeJSON(w, http.StatusOK, modelPullResponseFromService(result))
}

func isModelPullError(err error) bool {
	var pullErr *modelinference.PullError
	return errors.As(err, &pullErr) && pullErr != nil
}

type requestValidationError struct{ message string }

func (e requestValidationError) Error() string { return e.message }

func decodeModelInvocationRequestBody(body io.Reader) (factoryapi.ModelInvocationRequest, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = io.ReadAll(body)
		if err != nil {
			return factoryapi.ModelInvocationRequest{}, err
		}
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return factoryapi.ModelInvocationRequest{}, requestValidationError{message: "request body is required"}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}
	if err := validateWorkContentField(fields, ""); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}
	var request factoryapi.ModelInvocationRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}
	return request, nil
}

func generatedResolvedModelInvocationBindings(values []modelinference.ResolvedModelOperationBinding) []factoryapi.ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	bindings := make([]factoryapi.ResolvedModelOperationBinding, 0, len(values))
	for _, binding := range values {
		bindings = append(bindings, factoryapi.ResolvedModelOperationBinding{
			Slot: binding.Slot, Source: factoryapi.ResolvedModelOperationBindingSource(binding.Source),
			Content: derefGeneratedWorkContent(contentcontract.GeneratedPtrFromParts(binding.Content)),
		})
	}
	return bindings
}

func derefGeneratedWorkContent(content *factoryapi.WorkContent) factoryapi.WorkContent {
	if content == nil {
		return nil
	}
	return *content
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
