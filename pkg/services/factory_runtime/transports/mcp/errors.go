package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const (
	errorCodeBadRequest                 = "BAD_REQUEST"
	errorCodeRuntimeNotRunning          = "factory_runtime.runtime.not_running"
	errorCodeTargetNotFound             = "factory_runtime.target.not_found"
	errorCodeInvalidObservationScope    = "factory_runtime.observation.invalid_scope"
	errorCodeInternalExecution          = "factory_runtime.execution.internal"
	errorCodeRuntimeUnavailable         = "factory_runtime.runtime.unavailable"
	errorCodeRequestCanceled            = "factory_runtime.request.canceled"
	errorCodeRequestTimedOut            = "factory_runtime.request.timed_out"
	errorMessageRuntimeNotRunning       = "factory runtime is not running"
	errorMessageTargetNotFound          = "factory runtime target not found"
	errorMessageInvalidObservationScope = "factory runtime invalid observation scope"
	errorMessageInternalExecution       = "factory runtime execution failed"
	errorMessageRuntimeUnavailable      = "factory runtime is unavailable"
	errorMessageRequestCanceled         = "factory runtime request was canceled"
	errorMessageRequestTimedOut         = "factory runtime request timed out"
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

func decodeInputErrorEnvelope(context string, err error) ToolErrorEnvelope {
	message := context
	details := map[string]any{}
	if err != nil {
		if trimmed := strings.TrimSpace(err.Error()); trimmed != "" {
			message = fmt.Sprintf("%s: %s", context, trimmed)
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

func validationErrorEnvelope(err error) ToolErrorEnvelope {
	message := "invalid tool input"
	details := map[string]any{}
	if err != nil {
		if trimmed := strings.TrimSpace(err.Error()); trimmed != "" {
			message = trimmed
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

func executionErrorEnvelope(err error) ToolErrorEnvelope {
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
		return envelope
	}
	if envelope, ok := rootErrorEnvelope(err); ok {
		return envelope
	}
	if isSafeClientFacingError(err) {
		return ToolErrorEnvelope{
			Code:      errorCodeBadRequest,
			Message:   strings.TrimSpace(err.Error()),
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	}
	return unmappedExecutionErrorEnvelope()
}

func rootErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	switch {
	case errors.Is(err, factoryruntime.ErrNotRunning):
		return notRunningErrorEnvelope(), true
	case errors.Is(err, factoryruntime.ErrNotFound):
		return targetNotFoundErrorEnvelope(), true
	case errors.Is(err, factoryruntime.ErrInvalidObservationScope):
		return invalidObservationScopeErrorEnvelope(), true
	default:
		return ToolErrorEnvelope{}, false
	}
}

func notRunningErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeRuntimeNotRunning,
		Message:   errorMessageRuntimeNotRunning,
		Retryable: true,
		Details: map[string]any{
			"reason": "NOT_RUNNING",
		},
	}
}

func targetNotFoundErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeTargetNotFound,
		Message:   errorMessageTargetNotFound,
		Retryable: false,
		Details: map[string]any{
			"reason": "NOT_FOUND",
		},
	}
}

func invalidObservationScopeErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeInvalidObservationScope,
		Message:   errorMessageInvalidObservationScope,
		Retryable: false,
		Details: map[string]any{
			"reason": "INVALID_OBSERVATION_SCOPE",
		},
	}
}

func isSafeClientFacingError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	if strings.Contains(message, "factory_runtime/internal") {
		return false
	}
	if strings.Contains(message, "goroutine ") {
		return false
	}
	return strings.TrimSpace(message) != ""
}

func unmappedExecutionErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeInternalExecution,
		Message:   errorMessageInternalExecution,
		Retryable: false,
	}
}

func unavailableRuntimeErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeRuntimeUnavailable,
		Message:   errorMessageRuntimeUnavailable,
		Retryable: false,
	}
}
