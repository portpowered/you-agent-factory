package providersmcp

import (
	"context"
	"errors"
	"strings"
)

const (
	errorCodeBadRequest         = "BAD_REQUEST"
	errorCodeServiceUnavailable = "provider.service.unavailable"
	errorCodeInternalExecution  = "provider.execution.internal"
	errorCodeRequestCanceled    = "provider.request.canceled"
	errorCodeRequestTimedOut    = "provider.request.timed_out"
	errorMessageServiceUnavailable = "providers service is unavailable"
	errorMessageRequestCanceled    = "providers request was canceled"
	errorMessageRequestTimedOut    = "providers request timed out"
	errorMessageInternalExecution  = "providers execution failed"
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
