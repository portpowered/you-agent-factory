package factoryvisualization

import (
	"context"
	"errors"
	"strings"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

const (
	errorCodeBadRequest           = "BAD_REQUEST"
	errorCodeServiceUnavailable   = "factory_visualization.service.unavailable"
	errorCodeRequestCanceled      = "factory_visualization.request.canceled"
	errorCodeRequestTimedOut      = "factory_visualization.request.timed_out"
	errorMessageServiceUnavailable = "factory visualization service is unavailable"
	errorMessageRequestCanceled   = "factory visualization request was canceled"
	errorMessageRequestTimedOut   = "factory visualization request timed out"
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
			message = context + ": " + trimmed
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

func missingContextErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   errMissingRequestContext.Error(),
		Retryable: false,
	}
}

func serviceUnavailableErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeServiceUnavailable,
		Message:   errorMessageServiceUnavailable,
		Retryable: false,
	}
}

func requestValidationErrorEnvelope(message string) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   message,
		Retryable: false,
	}
}

func mapRootError(err error) ToolErrorEnvelope {
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
		return envelope
	}
	var lifeErr *factoryvisualization.LifecycleError
	if errors.As(err, &lifeErr) {
		return ToolErrorEnvelope{
			Code:      "factory_visualization.lifecycle." + strings.ToLower(string(lifeErr.Kind)),
			Message:   lifeErr.Error(),
			Retryable: false,
			Details: map[string]any{
				"kind": string(lifeErr.Kind),
			},
		}
	}
	var projErr *factoryvisualization.ProjectionError
	if errors.As(err, &projErr) {
		return ToolErrorEnvelope{
			Code:      "factory_visualization.projection." + strings.ToLower(string(projErr.Kind)),
			Message:   projErr.Error(),
			Retryable: false,
			Details: map[string]any{
				"kind": string(projErr.Kind),
			},
		}
	}
	var presErr *factoryvisualization.PresentationError
	if errors.As(err, &presErr) {
		return ToolErrorEnvelope{
			Code:      "factory_visualization.presentation." + strings.ToLower(string(presErr.Kind)),
			Message:   presErr.Error(),
			Retryable: false,
			Details: map[string]any{
				"kind": string(presErr.Kind),
			},
		}
	}
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   strings.TrimSpace(err.Error()),
		Retryable: false,
	}
}
