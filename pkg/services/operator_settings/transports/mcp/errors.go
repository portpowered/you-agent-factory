package operatorsettingsmcp

import (
	"context"
	"errors"
	"fmt"
)

const (
	errorCodeBadRequest          = "BAD_REQUEST"
	errorCodeServiceUnavailable  = "operator_settings.service.unavailable"
	errorCodeInternalExecution   = "operator_settings.execution.internal"
	errorCodeRequestCanceled     = "operator_settings.request.canceled"
	errorCodeRequestTimedOut     = "operator_settings.request.timed_out"
	errorMessageServiceUnavailable = "operator settings service is unavailable"
	errorMessageRequestCanceled    = "operator settings request was canceled"
	errorMessageRequestTimedOut    = "operator settings request timed out"
	errorMessageInternalExecution  = "operator settings execution failed"
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
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   fmt.Sprintf("%s: %v", operation, err),
		Retryable: false,
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
