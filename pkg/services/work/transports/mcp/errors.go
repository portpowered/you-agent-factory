package workmcp

import (
	"context"
	"errors"
	"strings"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	errorCodeBadRequest               = "BAD_REQUEST"
	errorCodeRequestCanceled          = "work.request.canceled"
	errorCodeRequestTimedOut          = "work.request.timed_out"
	errorMessageRequestCanceled       = "work request was canceled"
	errorMessageRequestTimedOut       = "work request timed out"
	errorCodeAdmissionInvalid         = "work.admission.invalid"
	errorCodeAdmissionConflict        = "work.admission.conflict"
	errorCodeAdmissionRejected        = "work.admission.rejected"
	errorCodeStateAccessNotFound        = "work.state_access.not_found"
	errorCodeStateAccessInvalid         = "work.state_access.invalid"
	errorCodeStateAccessAlreadyApplied  = "work.state_access.already_applied"
	errorMessageAdmissionInvalid      = "invalid Work Request"
	errorMessageAdmissionConflict     = "Work Request admission conflict"
	errorMessageAdmissionRejected     = "Work Request rejected"
	errorMessageStateAccessNotFound       = "Work not found"
	errorMessageStateAccessAlreadyApplied = "Operator move request was already applied"
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

func unavailableServiceErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "work.service.unavailable",
		Message:   "work service is unavailable",
		Retryable: false,
	}
}

func submitErrorEnvelope(err error) ToolErrorEnvelope {
	switch {
	case errors.Is(err, work.ErrInvalidWorkRequest):
		return ToolErrorEnvelope{
			Code:      errorCodeAdmissionInvalid,
			Message:   errorMessageAdmissionInvalid,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, work.ErrWorkRequestConflict):
		return ToolErrorEnvelope{
			Code:      errorCodeAdmissionConflict,
			Message:   errorMessageAdmissionConflict,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, work.ErrWorkRequestRejected):
		return ToolErrorEnvelope{
			Code:      errorCodeAdmissionRejected,
			Message:   errorMessageAdmissionRejected,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	default:
		return executionErrorEnvelope(err)
	}
}

func stateAccessErrorEnvelope(err error) ToolErrorEnvelope {
	switch {
	case errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied):
		return ToolErrorEnvelope{
			Code:      errorCodeStateAccessAlreadyApplied,
			Message:   errorMessageStateAccessAlreadyApplied,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, work.ErrWorkNotFound):
		return ToolErrorEnvelope{
			Code:      errorCodeStateAccessNotFound,
			Message:   errorMessageStateAccessNotFound,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	default:
		var validationErr *work.ValidationError
		if errors.As(err, &validationErr) {
			details := map[string]any{
				"reason": err.Error(),
			}
			if field := strings.TrimSpace(validationErr.Field); field != "" {
				details["field"] = field
			}
			return ToolErrorEnvelope{
				Code:      errorCodeStateAccessInvalid,
				Message:   strings.TrimSpace(validationErr.Error()),
				Retryable: false,
				Details:   details,
			}
		}
		return executionErrorEnvelope(err)
	}
}

func executionErrorEnvelope(err error) ToolErrorEnvelope {
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
		return envelope
	}
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   strings.TrimSpace(err.Error()),
		Retryable: false,
		Details: map[string]any{
			"reason": err.Error(),
		},
	}
}
