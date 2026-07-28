package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
		if failure, ok := workers.AsInferenceFailure(err); ok {
			return inferenceFailureErrorResponse(failure)
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
	case errors.Is(err, models.ErrUnsupportedOperation), errors.Is(err, models.ErrUnsupportedResponseMode):
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
	case errors.Is(err, models.ErrPullUnsupported) && operation == modelsHTTPOperationPull:
		return badRequestErrorResponse(strings.TrimSpace(err.Error()))
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

func inferenceFailureErrorResponse(failure *workers.InferenceFailure) (int, factoryapi.ErrorResponse, bool) {
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
