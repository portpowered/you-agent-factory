package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	executeInvalidRequestMessage      = "invalid provider execution request"
	executeFailedMessage              = "provider execution failed"
	executeCanceledMessage            = "provider execution canceled"
	executeTimeoutMessage             = "provider execution timed out"
	executeErrorCodeCanceled          = "PROVIDER_EXECUTION_CANCELED"
	executeErrorCodeTimeout           = "PROVIDER_EXECUTION_TIMEOUT"
	executeErrorCodeAuthentication    = "PROVIDER_EXECUTION_AUTHENTICATION"
	executeErrorCodeThrottled         = "PROVIDER_EXECUTION_THROTTLED"
	executeErrorCodeDependency        = "PROVIDER_EXECUTION_DEPENDENCY"
	executeErrorCodeFailed            = "PROVIDER_EXECUTION_FAILED"
)

// ExecuteRootErrorResponse maps typed Providers execute root failures and
// adapter decode validation failures to HTTP status and the public ErrorResponse
// shape. It returns false when err is not a known mapped typed failure.
func ExecuteRootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	if errors.Is(err, ErrInvalidExecuteRequest) {
		return badRequestErrorResponse(executeInvalidRequestMessage)
	}

	switch {
	case errors.Is(err, providers.ErrInvalidID):
		return badRequestErrorResponse(catalogInvalidProviderIDMessage)
	case errors.Is(err, providers.ErrUnknownProvider):
		return notFoundErrorResponse(catalogProviderNotFoundMessage)
	case errors.Is(err, providers.ErrInvalidSessionRef):
		return badRequestErrorResponse(executeInvalidRequestMessage)
	}

	var failure providers.ExecuteFailure
	if errors.As(err, &failure) {
		return executeFailureErrorResponse(failure)
	}

	switch {
	case errors.Is(err, providers.ErrExecuteCancelled):
		return internalErrorResponseWithCode(executeCanceledMessage, executeErrorCodeCanceled)
	case errors.Is(err, providers.ErrExecuteTimeout):
		return gatewayTimeoutErrorResponse(executeTimeoutMessage, executeErrorCodeTimeout)
	case errors.Is(err, providers.ErrExecuteFailed):
		return internalErrorResponseWithCode(executeFailedMessage, executeErrorCodeFailed)
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func (a *Adapter) writeExecuteError(w http.ResponseWriter, err error) bool {
	if status, response, ok := ExecuteRootErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func (a *Adapter) writeExecuteOrInternalError(w http.ResponseWriter, err error) {
	if a.writeExecuteError(w, err) {
		return
	}
	a.writeError(
		w,
		http.StatusInternalServerError,
		executeFailedMessage,
		string(factoryapi.ErrorResponseCodeINTERNALERROR),
	)
}

func executeFailureErrorResponse(failure providers.ExecuteFailure) (int, factoryapi.ErrorResponse, bool) {
	message := strings.TrimSpace(failure.Message)
	switch failure.Kind {
	case providers.ExecuteFailureKindCanceled:
		if message == "" {
			message = executeCanceledMessage
		}
		return internalErrorResponseWithCode(message, executeErrorCodeCanceled)
	case providers.ExecuteFailureKindTimeout:
		if message == "" {
			message = executeTimeoutMessage
		}
		return gatewayTimeoutErrorResponse(message, executeErrorCodeTimeout)
	case providers.ExecuteFailureKindAuthentication:
		if message == "" {
			message = "provider execution authentication failed"
		}
		return unauthorizedErrorResponse(message, executeErrorCodeAuthentication)
	case providers.ExecuteFailureKindInvalidRequest:
		if message == "" {
			message = executeInvalidRequestMessage
		}
		return badRequestErrorResponse(message)
	case providers.ExecuteFailureKindThrottled:
		if message == "" {
			message = "provider execution throttled"
		}
		return tooManyRequestsErrorResponse(message, executeErrorCodeThrottled)
	case providers.ExecuteFailureKindDependency:
		if message == "" {
			message = "provider execution dependency failed"
		}
		return serviceUnavailableErrorResponse(message, executeErrorCodeDependency)
	default:
		if message == "" {
			message = executeFailedMessage
		}
		return internalErrorResponseWithCode(message, executeErrorCodeFailed)
	}
}

func internalErrorResponseWithCode(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusInternalServerError, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func gatewayTimeoutErrorResponse(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusGatewayTimeout, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func unauthorizedErrorResponse(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusUnauthorized, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCode(code),
	}, true
}

func tooManyRequestsErrorResponse(message, code string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusTooManyRequests, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
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
