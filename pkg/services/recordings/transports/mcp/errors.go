package recordingmcp

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const (
	errorCodeBadRequest         = "BAD_REQUEST"
	errorCodeServiceUnavailable = "recording.service.unavailable"
	errorCodeMissingTarget      = "recording.target.missing"
	errorCodeInternalExecution  = "recording.execution.internal"

	errorMessageServiceUnavailable = "recordings service is unavailable"
	errorMessageMissingTarget      = "recording target not found"
	errorMessageInternalExecution  = "recording execution failed"
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
		Code:      "recording.request.canceled",
		Message:   "recording request was canceled",
		Retryable: false,
		Details: map[string]any{
			"reason": "CANCELED",
		},
	}
}

func contextDeadlineExceededErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "recording.request.timed_out",
		Message:   "recording request timed out",
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

func unavailableServiceErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeServiceUnavailable,
		Message:   errorMessageServiceUnavailable,
		Retryable: false,
	}
}

func statusQueryErrorEnvelope(recordingID string, err error) ToolErrorEnvelope {
	if errors.Is(err, recordings.ErrMissingRecordingTarget) ||
		errors.Is(err, recordings.ErrInvalidRecordingScope) {
		return ToolErrorEnvelope{
			Code:        errorCodeMissingTarget,
			Message:     errorMessageMissingTarget,
			Retryable:   false,
			RecordingID: strings.TrimSpace(recordingID),
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	}
	return executionErrorEnvelope(recordingID, err)
}

func executionErrorEnvelope(recordingID string, err error) ToolErrorEnvelope {
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
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
	return ToolErrorEnvelope{
		Code:        errorCodeInternalExecution,
		Message:     errorMessageInternalExecution,
		Retryable:   false,
		RecordingID: strings.TrimSpace(recordingID),
	}
}

func isSafeClientFacingError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	if strings.Contains(message, "recordings/internal") {
		return false
	}
	if strings.Contains(message, "goroutine ") {
		return false
	}
	return strings.TrimSpace(message) != ""
}
