package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type modelsHTTPOperation int

const (
	modelsHTTPOperationCatalog modelsHTTPOperation = iota
	modelsHTTPOperationPull
	modelsHTTPOperationInvoke
)

const (
	catalogNotFoundMessage            = "model not found"
	catalogListFailedMessage          = "failed to list models"
	catalogGetFailedMessage           = "failed to load model"
	pullFailedMessage                 = "failed to pull model"
	invokeFailedMessage               = "model invocation failed"
	catalogErrorCodeModelNotAvailable = "MODEL_NOT_AVAILABLE"
)

// RootErrorResponse maps typed Models root failures to HTTP status and the
// public ErrorResponse shape. It returns false when err is not a known mapped
// typed failure for the operation.
func RootErrorResponse(err error, operation modelsHTTPOperation) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}
	if operation == modelsHTTPOperationInvoke {
		if status, response, ok := inferenceRootErrorResponse(err); ok {
			return status, response, true
		}
	}

	switch {
	case errors.Is(err, models.ErrNotFound):
		return notFoundErrorResponse(catalogNotFoundMessage)
	case errors.Is(err, models.ErrUnavailable) && operation == modelsHTTPOperationCatalog:
		return modelNotAvailableErrorResponse(strings.TrimSpace(err.Error()))
	case errors.Is(err, models.ErrMissing), errors.Is(err, models.ErrNotAvailable):
		return modelNotAvailableErrorResponse(strings.TrimSpace(err.Error()))
	case errors.Is(err, models.ErrLoading):
		return conflictErrorResponse(strings.TrimSpace(err.Error()), "MODEL_RUNTIME_LOADING")
	case errors.Is(err, models.ErrFailed):
		return serviceUnavailableErrorResponse(strings.TrimSpace(err.Error()), "MODEL_RUNTIME_FAILED")
	case errors.Is(err, models.ErrUnsupported):
		return badRequestErrorResponseWithCode(strings.TrimSpace(err.Error()), "MODEL_RUNTIME_UNSUPPORTED")
	case errors.Is(err, models.ErrUnsupportedOperation),
		errors.Is(err, models.ErrUnsupportedModelOperation),
		errors.Is(err, models.ErrUnsupportedResponseMode):
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	case errors.Is(err, models.ErrPullUnsupported) && operation == modelsHTTPOperationPull:
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	case errors.Is(err, models.ErrAssetUnavailable):
		return modelNotAvailableErrorResponse(strings.TrimSpace(err.Error()))
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

// CatalogRootErrorResponse maps typed Models catalog root failures to HTTP
// status and the public ErrorResponse shape. It returns false when err is not a
// known mapped typed failure.
func CatalogRootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	return RootErrorResponse(err, modelsHTTPOperationCatalog)
}

func gatewayTimeoutErrorResponse(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusGatewayTimeout, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func internalErrorResponse(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusInternalServerError, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func inferenceRootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	switch {
	case errors.Is(err, models.ErrInferenceTimeout):
		return gatewayTimeoutErrorResponse(inferenceErrorMessage(err, models.ErrInferenceTimeout), "MODEL_INFERENCE_TIMEOUT")
	case errors.Is(err, models.ErrInferenceCancelled):
		return conflictErrorResponse(inferenceErrorMessage(err, models.ErrInferenceCancelled), "MODEL_INFERENCE_CANCELLED")
	case errors.Is(err, models.ErrInferenceArtifactInvalid):
		return internalErrorResponse(inferenceErrorMessage(err, models.ErrInferenceArtifactInvalid), "MODEL_INFERENCE_ARTIFACT_INVALID")
	case errors.Is(err, models.ErrInvalidInferenceDependencies):
		return internalErrorResponse(inferenceErrorMessage(err, models.ErrInvalidInferenceDependencies), "MODEL_INFERENCE_DEPENDENCIES_INVALID")
	case errors.Is(err, models.ErrInferenceFailed):
		return internalErrorResponse(inferenceErrorMessage(err, models.ErrInferenceFailed), "MODEL_INFERENCE_RUNTIME_FAILURE")
	case errors.Is(err, models.ErrHostCapacityExhausted):
		return conflictErrorResponse(strings.TrimSpace(err.Error()), "MODEL_LEASE_CAPACITY_EXHAUSTED")
	case errors.Is(err, models.ErrHostCapacityContended):
		return conflictErrorResponse(strings.TrimSpace(err.Error()), "MODEL_LEASE_CAPACITY_CONTENDED")
	case errors.Is(err, models.ErrHostRuntimeNotReady):
		return conflictErrorResponse(strings.TrimSpace(err.Error()), "MODEL_RUNTIME_NOT_READY")
	case errors.Is(err, models.ErrHostLeaseExpired), errors.Is(err, models.ErrHostLeaseNotFound):
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	case errors.Is(err, models.ErrHostInvalidHolder):
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func notFoundErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusNotFound, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyNotFound,
		Code:    factoryapi.ErrorResponseCodeNOTFOUND,
	}, true
}

func modelNotAvailableErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusNotFound, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyNotFound,
		Code:    factoryapi.ErrorResponseCode(catalogErrorCodeModelNotAvailable),
	}, true
}

func conflictErrorResponse(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusConflict, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyConflict,
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func serviceUnavailableErrorResponse(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusServiceUnavailable, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func badRequestErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return badRequestErrorResponseWithCode(message, "BAD_REQUEST")
}

func badRequestErrorResponseWithCode(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusBadRequest, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func modelsErrorMessageLeaksInternalDetail(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "/internal/") ||
		strings.Contains(trimmed, "pkg/services/models/internal") ||
		strings.Contains(trimmed, ".go:") ||
		strings.Contains(lower, "stack trace") {
		return true
	}
	return false
}

func inferenceErrorMessage(err, sentinel error) string {
	message := strings.TrimSpace(err.Error())
	prefix := strings.TrimSpace(sentinel.Error()) + ":"
	if strings.HasPrefix(message, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(message, prefix))
	}
	return message
}
