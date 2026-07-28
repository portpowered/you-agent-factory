package providersmcp

import (
	"context"
	"errors"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	errorCodeBadRequest         = "BAD_REQUEST"
	errorCodeServiceUnavailable    = "provider.service.unavailable"
	errorCodeIdentityInvalid       = "provider.identity.invalid"
	errorCodeCatalogUnknown        = "provider.catalog.unknown"
	errorCodeCatalogUnavailable    = "provider.catalog.unavailable"
	errorCodeInternalExecution          = "provider.execution.internal"
	errorCodeExecutionCanceled          = "provider.execution.canceled"
	errorCodeExecutionTimedOut          = "provider.execution.timed_out"
	errorCodeExecutionAuthentication    = "provider.execution.authentication"
	errorCodeExecutionInvalidRequest    = "provider.execution.invalid_request"
	errorCodeExecutionThrottled         = "provider.execution.throttled"
	errorCodeExecutionDependency        = "provider.execution.dependency"
	errorCodeExecutionUnknown           = "provider.execution.unknown"
	errorCodeRequestCanceled            = "provider.request.canceled"
	errorCodeRequestTimedOut            = "provider.request.timed_out"
	errorMessageServiceUnavailable      = "providers service is unavailable"
	errorMessageIdentityInvalid         = "provider id is invalid"
	errorMessageCatalogUnknown          = "provider is unknown"
	errorMessageCatalogUnavailable      = "provider is unavailable"
	errorMessageExecutionCanceled       = "provider execution was canceled"
	errorMessageExecutionTimedOut       = "provider execution timed out"
	errorMessageExecutionAuthentication = "provider authentication failed"
	errorMessageExecutionInvalidRequest = "provider execution request is invalid"
	errorMessageExecutionThrottled      = "provider execution was throttled"
	errorMessageExecutionDependency     = "provider execution dependency failed"
	errorMessageExecutionUnknown        = "provider execution failed"
	errorMessageRequestCanceled         = "providers request was canceled"
	errorMessageRequestTimedOut         = "providers request timed out"
	errorMessageInternalExecution       = "providers execution failed"
)

func requestContextErrorResponse[T any](ctx context.Context) (ToolResponse[T], bool) {
	if envelope, ok := contextRequestErrorEnvelope(ctx.Err()); ok {
		return ToolResponse[T]{Error: &envelope}, true
	}
	return ToolResponse[T]{}, false
}

func contextRequestErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	if err == nil {
		return ToolErrorEnvelope{}, false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return contextDeadlineExceededErrorEnvelope(), true
	}
	if errors.Is(err, context.Canceled) {
		return contextCanceledErrorEnvelope(), true
	}
	return ToolErrorEnvelope{}, false
}

func contextCanceledErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeRequestCanceled,
		Message:   errorMessageRequestCanceled,
		Retryable: false,
		Details: map[string]any{
			"reason": "CANCELED",
		},
	}
}

func contextDeadlineExceededErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeRequestTimedOut,
		Message:   errorMessageRequestTimedOut,
		Retryable: true,
		Details: map[string]any{
			"reason": "TIMED_OUT",
		},
	}
}

func decodeInputErrorEnvelope(operation string, err error) ToolErrorEnvelope {
	message := operation
	details := map[string]any{}
	if err != nil {
		if trimmed := strings.TrimSpace(err.Error()); trimmed != "" {
			message = operation + ": " + trimmed
		}
		details["reason"] = err.Error()
	}
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   message,
		Retryable: false,
		Details:   details,
	}
}

func unavailableServiceErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeServiceUnavailable,
		Message:   errorMessageServiceUnavailable,
		Retryable: false,
	}
}

func catalogErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	switch {
	case errors.Is(err, providers.ErrInvalidID):
		return ToolErrorEnvelope{
			Code:      errorCodeIdentityInvalid,
			Message:   errorMessageIdentityInvalid,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}, true
	case errors.Is(err, providers.ErrUnknownProvider):
		return ToolErrorEnvelope{
			Code:      errorCodeCatalogUnknown,
			Message:   errorMessageCatalogUnknown,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}, true
	case errors.Is(err, providers.ErrProviderUnavailable):
		return ToolErrorEnvelope{
			Code:      errorCodeCatalogUnavailable,
			Message:   errorMessageCatalogUnavailable,
			Retryable: true,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}, true
	default:
		return ToolErrorEnvelope{}, false
	}
}

func getProviderErrorEnvelope(err error) ToolErrorEnvelope {
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
		return envelope
	}
	if envelope, ok := catalogErrorEnvelope(err); ok {
		return envelope
	}
	return executionErrorEnvelope(err)
}

func executeErrorEnvelope(err error) ToolErrorEnvelope {
	if err == nil {
		return opaqueExecutionErrorEnvelope()
	}
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
		return envelope
	}
	if envelope, ok := catalogErrorEnvelope(err); ok {
		return envelope
	}
	var failure providers.ExecuteFailure
	if errors.As(err, &failure) {
		return executeFailureErrorEnvelope(failure, err)
	}
	if errors.Is(err, providers.ErrExecuteCancelled) {
		return executeCanceledErrorEnvelope(err)
	}
	if errors.Is(err, providers.ErrExecuteTimeout) {
		return executeTimedOutErrorEnvelope(err)
	}
	return executionErrorEnvelope(err)
}

func executeFailureErrorEnvelope(failure providers.ExecuteFailure, err error) ToolErrorEnvelope {
	code, message, retryable := executeFailurePresentation(failure)
	if trimmed := strings.TrimSpace(failure.Message); trimmed != "" {
		message = trimmed
	}
	return ToolErrorEnvelope{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Details:   executeFailureDetails(failure, err),
	}
}

func executeFailurePresentation(failure providers.ExecuteFailure) (code string, message string, retryable bool) {
	switch failure.Kind {
	case providers.ExecuteFailureKindCanceled:
		return errorCodeExecutionCanceled, errorMessageExecutionCanceled, false
	case providers.ExecuteFailureKindTimeout:
		return errorCodeExecutionTimedOut, errorMessageExecutionTimedOut, true
	case providers.ExecuteFailureKindAuthentication:
		return errorCodeExecutionAuthentication, errorMessageExecutionAuthentication, false
	case providers.ExecuteFailureKindInvalidRequest:
		return errorCodeExecutionInvalidRequest, errorMessageExecutionInvalidRequest, false
	case providers.ExecuteFailureKindThrottled:
		return errorCodeExecutionThrottled, errorMessageExecutionThrottled, true
	case providers.ExecuteFailureKindDependency:
		return errorCodeExecutionDependency, errorMessageExecutionDependency, true
	default:
		return errorCodeExecutionUnknown, errorMessageExecutionUnknown, false
	}
}

func executeFailureDetails(failure providers.ExecuteFailure, err error) map[string]any {
	details := map[string]any{
		"kind":   string(failure.Kind),
		"reason": err.Error(),
	}
	if failure.SessionRef != nil {
		details["sessionRef"] = map[string]any{
			"provider": string(failure.SessionRef.Provider),
			"kind":     failure.SessionRef.Kind,
			"id":       failure.SessionRef.ID,
		}
	}
	return details
}

func executeCanceledErrorEnvelope(err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeExecutionCanceled,
		Message:   errorMessageExecutionCanceled,
		Retryable: false,
		Details: map[string]any{
			"kind":   string(providers.ExecuteFailureKindCanceled),
			"reason": err.Error(),
		},
	}
}

func executeTimedOutErrorEnvelope(err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeExecutionTimedOut,
		Message:   errorMessageExecutionTimedOut,
		Retryable: true,
		Details: map[string]any{
			"kind":   string(providers.ExecuteFailureKindTimeout),
			"reason": err.Error(),
		},
	}
}

func executionErrorEnvelope(err error) ToolErrorEnvelope {
	if err == nil {
		return opaqueExecutionErrorEnvelope()
	}
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
		return envelope
	}
	return opaqueExecutionErrorEnvelope()
}

func opaqueExecutionErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeInternalExecution,
		Message:   errorMessageInternalExecution,
		Retryable: false,
	}
}
