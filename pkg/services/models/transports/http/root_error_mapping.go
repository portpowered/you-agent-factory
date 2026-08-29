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
	modelsHTTPOperationRemove
	modelsHTTPOperationInvoke
	modelsHTTPOperationGenericInvoke
)

const (
	catalogNotFoundMessage            = "model not found"
	catalogListFailedMessage          = "failed to list models"
	catalogGetFailedMessage           = "failed to load model"
	pullFailedMessage                 = "failed to pull model"
	removeFailedMessage               = "failed to remove model cache"
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
	if operation == modelsHTTPOperationInvoke || operation == modelsHTTPOperationGenericInvoke {
		if status, response, ok := classifiedInferenceErrorResponse(err); ok {
			return status, response, ok
		}
		if status, response, ok := invocationFailureErrorResponse(err); ok {
			return status, response, ok
		}
	}
	return rootSentinelErrorResponse(err, operation)
}

func rootSentinelErrorResponse(err error, operation modelsHTTPOperation) (int, factoryapi.ErrorResponse, bool) {
	if operation == modelsHTTPOperationGenericInvoke {
		if status, response, ok := genericSentinelErrorResponse(err); ok {
			return status, response, ok
		}
	}
	return commonSentinelErrorResponse(err, operation)
}

func genericSentinelErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	switch {
	case errors.Is(err, models.ErrRuntimeScopeInvalid), errors.Is(err, models.ErrHostInvalidHolder):
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	case errors.Is(err, models.ErrRuntimeScopeStale), errors.Is(err, models.ErrRuntimeScopeClosed), errors.Is(err, models.ErrRuntimeScopeForeign):
		return conflictErrorResponse(strings.TrimSpace(err.Error()), "MODEL_RUNTIME_SCOPE_UNAVAILABLE")
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func commonSentinelErrorResponse(err error, operation modelsHTTPOperation) (int, factoryapi.ErrorResponse, bool) {
	if status, response, ok := removeSentinelErrorResponse(err, operation); ok {
		return status, response, true
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
	case errors.Is(err, models.ErrUnsupportedOperation), errors.Is(err, models.ErrUnsupportedResponseMode):
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	case errors.Is(err, models.ErrPullUnsupported) && operation == modelsHTTPOperationPull:
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func removeSentinelErrorResponse(err error, operation modelsHTTPOperation) (int, factoryapi.ErrorResponse, bool) {
	if operation != modelsHTTPOperationRemove {
		return 0, factoryapi.ErrorResponse{}, false
	}
	switch {
	case errors.Is(err, models.ErrModelCacheNotFound):
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: strings.TrimSpace(err.Error()),
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCode("MODEL_CACHE_NOT_FOUND"),
		}, true
	case errors.Is(err, models.ErrModelCacheInUse):
		return http.StatusConflict, factoryapi.ErrorResponse{
			Message: strings.TrimSpace(err.Error()),
			Family:  factoryapi.ErrorFamilyConflict,
			Code:    factoryapi.ErrorResponseCode("MODEL_CACHE_IN_USE"),
		}, true
	case errors.Is(err, models.ErrModelCacheUnsafe):
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func invocationFailureErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}
	status, code := invocationFailureHTTPStatusAndCode(failure.Class)
	return status, factoryapi.ErrorResponse{
		Message: failure.Error(),
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func invocationFailureHTTPStatusAndCode(class models.InvocationFailureClass) (int, string) {
	switch class {
	case models.InvocationFailureClassInvalidModelReference,
		models.InvocationFailureClassRevisionResolution:
		return http.StatusNotFound, "MODEL_NOT_AVAILABLE"
	case models.InvocationFailureClassInvalidOperation,
		models.InvocationFailureClassInvalidSlot,
		models.InvocationFailureClassSlotArity,
		models.InvocationFailureClassInvalidParameter,
		models.InvocationFailureClassMediaCapability,
		models.InvocationFailureClassArtifact:
		return http.StatusBadRequest, "BAD_REQUEST"
	case models.InvocationFailureClassOfflineCache:
		return http.StatusConflict, "MODEL_OFFLINE_CACHE_UNAVAILABLE"
	case models.InvocationFailureClassBackendReadiness:
		return http.StatusServiceUnavailable, "MODEL_BACKEND_NOT_READY"
	case models.InvocationFailureClassBackendProtocol,
		models.InvocationFailureClassMalformedResponse:
		return http.StatusBadGateway, "MODEL_BACKEND_FAILURE"
	case models.InvocationFailureClassCancellation,
		models.InvocationFailureClassTimeout:
		return http.StatusRequestTimeout, "MODEL_INFERENCE_TIMEOUT"
	case models.InvocationFailureClassConfiguration:
		return http.StatusInternalServerError, "MODEL_CONFIGURATION_FAILURE"
	default:
		return http.StatusInternalServerError, "MODEL_INFERENCE_RUNTIME_FAILURE"
	}
}

// CatalogRootErrorResponse maps typed Models catalog root failures to HTTP
// status and the public ErrorResponse shape. It returns false when err is not a
// known mapped typed failure.
func CatalogRootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	return RootErrorResponse(err, modelsHTTPOperationCatalog)
}

func inferenceFailureErrorResponse(failure *models.InferenceFailure) (int, factoryapi.ErrorResponse, bool) {
	if failure == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}
	status := inferenceFailureHTTPStatus(failure)
	return status, factoryapi.ErrorResponse{
		Message: failure.Error(),
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(inferenceFailureErrorCode(failure)),
	}, true
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
