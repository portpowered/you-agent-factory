package workmcp

import (
	"errors"
	"strings"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	errorCodeBadRequest          = "BAD_REQUEST"
	errorCodeAdmissionInvalid    = "work.admission.invalid"
	errorCodeAdmissionConflict   = "work.admission.conflict"
	errorCodeAdmissionRejected   = "work.admission.rejected"
	errorMessageAdmissionInvalid = "invalid Work Request"
	errorMessageAdmissionConflict = "Work Request admission conflict"
	errorMessageAdmissionRejected = "Work Request rejected"
)

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

func executionErrorEnvelope(err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   strings.TrimSpace(err.Error()),
		Retryable: false,
		Details: map[string]any{
			"reason": err.Error(),
		},
	}
}
