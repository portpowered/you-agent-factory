package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap"
)

func (s *Server) ListModels(w http.ResponseWriter, r *http.Request) {
	response, err := s.runtime.ListModels(r.Context())
	if err != nil {
		s.logger.Error("list models failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to list models", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) GetModel(w http.ResponseWriter, r *http.Request, modelName string) {
	model, err := s.runtime.GetModel(r.Context(), modelName)
	if err != nil {
		if errors.Is(err, apisurface.ErrModelNotFound) {
			s.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get model failed", zap.Error(err), zap.String("model_name", modelName))
		s.writeError(w, http.StatusInternalServerError, "failed to load model", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, model)
}

func (s *Server) InvokeModel(w http.ResponseWriter, r *http.Request, modelName string) {
	req, err := decodeModelInvocationRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.Operation) == "" {
		s.writeError(w, http.StatusBadRequest, "operation is required", "BAD_REQUEST")
		return
	}

	result, err := s.runtime.InvokeModel(r.Context(), modelName, req)
	if err != nil {
		if failure, ok := apisurface.AsInferenceFailure(err); ok {
			s.writeError(
				w,
				apisurface.InferenceFailureHTTPStatus(failure),
				failure.Error(),
				apisurface.InferenceFailureErrorCode(failure),
			)
			return
		}
		switch {
		case errors.Is(err, apisurface.ErrModelNotFound):
			s.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
		case apisurface.IsManagedRuntimeMissing(err), errors.Is(err, apisurface.ErrModelNotAvailable):
			s.writeError(w, http.StatusNotFound, err.Error(), "MODEL_NOT_AVAILABLE")
		case errors.Is(err, apisurface.ErrManagedRuntimeLoading):
			s.writeError(w, http.StatusConflict, err.Error(), "MODEL_RUNTIME_LOADING")
		case errors.Is(err, apisurface.ErrManagedRuntimeFailed):
			s.writeError(w, http.StatusServiceUnavailable, err.Error(), "MODEL_RUNTIME_FAILED")
		case errors.Is(err, apisurface.ErrManagedRuntimeUnsupported):
			s.writeError(w, http.StatusBadRequest, err.Error(), "MODEL_RUNTIME_UNSUPPORTED")
		case errors.Is(err, apisurface.ErrModelInvocationUnsupportedOperation), errors.Is(err, apisurface.ErrModelInvocationUnsupportedMode):
			s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		default:
			errText := strings.TrimSpace(err.Error())
			s.writeError(w, http.StatusBadRequest, errText, "BAD_REQUEST")
		}
		return
	}

	if strings.TrimSpace(result.StreamFile) != "" {
		if result.StreamContentType != "" {
			w.Header().Set("Content-Type", result.StreamContentType)
		}
		http.ServeFile(w, r, result.StreamFile)
		return
	}

	s.writeJSON(w, http.StatusOK, factoryapi.ModelInvocationResponse{
		ModelName:        result.ModelName,
		Worker:           result.Worker,
		Operation:        result.Operation,
		ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Content:          derefGeneratedWorkContent(contentcontract.GeneratedPtrFromParts(result.Content)),
		Bindings:         generatedResolvedModelInvocationBindings(result.Bindings),
	})
}

func (s *Server) PullModel(w http.ResponseWriter, r *http.Request, modelName string) {
	result, err := s.runtime.PullModel(r.Context(), modelName)
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrModelNotFound):
			s.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
		case errors.Is(err, apisurface.ErrModelPullUnsupported):
			s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		case apisurface.IsManagedRuntimePullError(err):
			pullErr, _ := apisurface.AsManagedRuntimePullError(err)
			s.writeJSON(w, apisurface.ManagedRuntimePullHTTPStatus(pullErr.Result), apisurface.ModelPullResponseFromService(pullErr.Result))
			return
		default:
			s.writeError(w, http.StatusInternalServerError, strings.TrimSpace(err.Error()), "INTERNAL_ERROR")
		}
		return
	}
	s.writeJSON(w, http.StatusOK, apisurface.ModelPullResponseFromService(result))
}

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
		return factoryapi.ModelInvocationRequest{}, requestFieldValidationError{message: "request body is required"}
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

func generatedResolvedModelInvocationBindings(values []workerexecution.ResolvedModelOperationBinding) []factoryapi.ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	bindings := make([]factoryapi.ResolvedModelOperationBinding, 0, len(values))
	for _, binding := range values {
		content := contentcontract.GeneratedPtrFromParts(binding.Content)
		bindings = append(bindings, factoryapi.ResolvedModelOperationBinding{
			Slot:    binding.Slot,
			Source:  factoryapi.ResolvedModelOperationBindingSource(binding.Source),
			Content: derefGeneratedWorkContent(content),
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
