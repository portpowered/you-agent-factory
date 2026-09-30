// Package http adapts the public HTTP model endpoints to the Models API role.
package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
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
	if h.guardModelsRequestContext(w, r) {
		return
	}
	response, err := h.adapter.ListModels(r.Context())
	if err != nil {
		h.writeCatalogError(w, err, catalogListFailedMessage)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetModel(w http.ResponseWriter, r *http.Request, modelName string) {
	if h.guardModelsRequestContext(w, r) {
		return
	}
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
	if h.guardModelsRequestContext(w, r) {
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

	h.writeJSON(w, http.StatusOK, modelInvocationResponseFromResult(result))
}

// InvokeGenericModel serves the customer-facing provider-neutral invocation
// contract. The response remains an ordered named-output list; no backend,
// cache, process, or filesystem detail is exposed at this boundary.
func (h *Handler) InvokeGenericModel(w http.ResponseWriter, r *http.Request) {
	request, err := decodeGenericModelInvocationHTTP(w, r)
	if err != nil {
		message := "invalid request payload"
		var validationErr requestValidationError
		if errors.As(err, &validationErr) {
			message = validationErr.message
		}
		h.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
		return
	}
	if h.guardModelsRequestContext(w, r) {
		return
	}

	result, err := h.adapter.InvokeGenericModel(r.Context(), request)
	if err != nil {
		h.writeRootOrInternalError(w, modelsHTTPOperationGenericInvoke, err, invokeFailedMessage)
		return
	}
	h.writeJSON(w, http.StatusOK, GenericInvocationResponseToGenerated(result))
}

func (h *Handler) writeInvocationError(w http.ResponseWriter, err error) {
	h.writeRootOrInternalError(w, modelsHTTPOperationInvoke, err, invokeFailedMessage)
}

func (h *Handler) PullModel(w http.ResponseWriter, r *http.Request, modelName string) {
	if h.guardModelsRequestContext(w, r) {
		return
	}
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

func (h *Handler) RemoveModel(
	w http.ResponseWriter,
	r *http.Request,
	modelName string,
	reclaimUnusedCache bool,
) {
	if h.guardModelsRequestContext(w, r) {
		return
	}
	result, err := h.adapter.RemoveModel(r.Context(), modelName, reclaimUnusedCache)
	if err != nil {
		h.writeRootOrInternalError(w, modelsHTTPOperationRemove, err, removeFailedMessage)
		return
	}
	h.writeJSON(w, http.StatusOK, modelRemoveResponseFromService(result, reclaimUnusedCache))
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

const (
	maxGenericMultipartBody = 64 << 20
	maxGenericRequestPart   = 1 << 20
	maxGenericFilePart      = 8 << 20
	maxGenericFileCount     = 64
)

type genericUpload struct {
	content   []byte
	mediaType string
}

func decodeGenericModelInvocationHTTP(w http.ResponseWriter, r *http.Request) (factoryapi.GenericModelInvocationRequest, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && mediaType == "multipart/form-data" {
		boundary := params["boundary"]
		if boundary == "" {
			return factoryapi.GenericModelInvocationRequest{}, requestValidationError{message: "multipart boundary is required"}
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxGenericMultipartBody)
		return decodeGenericModelInvocationMultipart(multipart.NewReader(r.Body, boundary))
	}
	return decodeGenericModelInvocationRequestFromHTTP(r.Body)
}

func decodeGenericModelInvocationMultipart(reader *multipart.Reader) (factoryapi.GenericModelInvocationRequest, error) {
	var request factoryapi.GenericModelInvocationRequest
	var uploads []genericUpload
	requestSeen := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return request, err
		}
		switch part.FormName() {
		case "request":
			if requestSeen {
				return request, requestValidationError{message: "multipart request part must occur once"}
			}
			requestSeen = true
			request, err = decodeGenericMultipartRequestPart(part)
		case "files":
			if len(uploads) >= maxGenericFileCount {
				return request, requestValidationError{message: "too many multipart files"}
			}
			var upload genericUpload
			upload, err = decodeGenericMultipartFilePart(part)
			if err == nil {
				uploads = append(uploads, upload)
			}
		default:
			err = requestValidationError{message: "unexpected multipart field"}
		}
		_ = part.Close()
		if err != nil {
			return request, err
		}
	}
	if !requestSeen {
		return request, requestValidationError{message: "multipart request part is required"}
	}
	return attachGenericMultipartFiles(request, uploads)
}

func decodeGenericMultipartRequestPart(part *multipart.Part) (factoryapi.GenericModelInvocationRequest, error) {
	if contentType := part.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			return factoryapi.GenericModelInvocationRequest{}, requestValidationError{message: "multipart request part must be application/json"}
		}
	}
	data, err := io.ReadAll(io.LimitReader(part, maxGenericRequestPart+1))
	if err != nil {
		return factoryapi.GenericModelInvocationRequest{}, err
	}
	if len(data) > maxGenericRequestPart {
		return factoryapi.GenericModelInvocationRequest{}, requestValidationError{message: "multipart request part exceeds 1 MiB"}
	}
	return decodeGenericModelInvocationRequestFromHTTP(bytes.NewReader(data))
}

func decodeGenericMultipartFilePart(part *multipart.Part) (genericUpload, error) {
	data, err := io.ReadAll(io.LimitReader(part, maxGenericFilePart+1))
	if err != nil {
		return genericUpload{}, err
	}
	if len(data) > maxGenericFilePart {
		return genericUpload{}, requestValidationError{message: "multipart file exceeds 8 MiB"}
	}
	if len(data) == 0 {
		return genericUpload{}, requestValidationError{message: "multipart file must not be empty"}
	}
	mediaType := ""
	if contentType := part.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, err = mime.ParseMediaType(contentType)
		if err != nil {
			return genericUpload{}, requestValidationError{message: "multipart file has invalid Content-Type"}
		}
	}
	return genericUpload{content: data, mediaType: mediaType}, nil
}

func attachGenericMultipartFiles(request factoryapi.GenericModelInvocationRequest, uploads []genericUpload) (factoryapi.GenericModelInvocationRequest, error) {
	if request.Inputs == nil {
		if len(uploads) != 0 {
			return request, requestValidationError{message: "multipart file has no matching media input"}
		}
		return request, nil
	}
	fileIndex := 0
	for index := range *request.Inputs {
		input := &(*request.Inputs)[index]
		carriers := 0
		for _, present := range []bool{input.Content != nil, input.ContentBase64 != nil, input.ArtifactRef != nil} {
			if present {
				carriers++
			}
		}
		if carriers > 1 {
			return request, requestValidationError{message: "input must set only one content carrier"}
		}
		if !genericMultipartMediaModality(input.Modality) || carriers != 0 {
			continue
		}
		if fileIndex >= len(uploads) {
			return request, requestValidationError{message: fmt.Sprintf("multipart file is required for input %q", input.Name)}
		}
		upload := uploads[fileIndex]
		fileIndex++
		if err := attachGenericMultipartFile(input, upload); err != nil {
			return request, err
		}
	}
	if fileIndex != len(uploads) {
		return request, requestValidationError{message: "multipart file has no matching media input"}
	}
	return request, nil
}

func attachGenericMultipartFile(input *factoryapi.ModelInvocationInput, upload genericUpload) error {
	if input.MediaType != nil && strings.TrimSpace(*input.MediaType) != "" {
		declared, _, err := mime.ParseMediaType(*input.MediaType)
		if err != nil {
			return requestValidationError{message: "input mediaType is invalid"}
		}
		if upload.mediaType != "" && !strings.EqualFold(declared, upload.mediaType) {
			return requestValidationError{message: "multipart file Content-Type does not match input mediaType"}
		}
		input.MediaType = &declared
	} else if upload.mediaType != "" {
		input.MediaType = &upload.mediaType
	} else {
		return requestValidationError{message: "multipart file or input must declare a media type"}
	}
	if input.ContentType != nil && strings.Contains(*input.ContentType, "/") {
		contentType, _, err := mime.ParseMediaType(*input.ContentType)
		if err != nil {
			return requestValidationError{message: "input contentType is invalid"}
		}
		if !strings.EqualFold(contentType, *input.MediaType) {
			return requestValidationError{message: "multipart file Content-Type does not match input contentType"}
		}
	}
	input.ContentBase64 = &upload.content
	return nil
}

func genericMultipartMediaModality(modality factoryapi.ModelInvocationContentType) bool {
	switch modality {
	case factoryapi.ModelInvocationContentTypeImage,
		factoryapi.ModelInvocationContentTypeAudio,
		factoryapi.ModelInvocationContentTypeVideo,
		factoryapi.ModelInvocationContentTypeBinary:
		return true
	default:
		return false
	}
}
