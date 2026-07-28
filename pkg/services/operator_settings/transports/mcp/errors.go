package operatorsettingsmcp

import (
	"context"
	"errors"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

const (
	errorCodeBadRequest            = "BAD_REQUEST"
	errorCodeServiceUnavailable    = "operator_settings.service.unavailable"
	errorCodeDocumentMalformed     = "operator_settings.document.malformed"
	errorCodeDocumentNotFound      = "operator_settings.document.not_found"
	errorCodeDocumentUnsupported   = "operator_settings.document.unsupported"
	errorCodeDocumentConflict      = "operator_settings.document.conflict"
	errorCodeInternalExecution     = "operator_settings.execution.internal"
	errorCodeRequestCanceled       = "operator_settings.request.canceled"
	errorCodeRequestTimedOut       = "operator_settings.request.timed_out"
	errorMessageServiceUnavailable = "operator settings service is unavailable"
	errorMessageDocumentMalformed  = "operator document is malformed"
	errorMessageDocumentNotFound   = "operator document not found"
	errorMessageDocumentUnsupported = "operator document update is unsupported"
	errorMessageDocumentConflict   = "operator document persist conflict"
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

func applyDocumentUpdateErrorEnvelope(path string, err error) ToolErrorEnvelope {
	var failure operatorsettings.DocumentFailure
	if errors.As(err, &failure) {
		switch failure.Kind {
		case operatorsettings.DocumentFailureKindMalformed:
			return documentMalformedErrorEnvelope(documentFailurePath(path, failure), err)
		case operatorsettings.DocumentFailureKindUnsupported:
			return documentUnsupportedErrorEnvelope(documentFailurePath(path, failure), err)
		case operatorsettings.DocumentFailureKindConflict:
			return documentConflictErrorEnvelope(documentFailurePath(path, failure), err)
		}
	}
	if errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		return documentMalformedErrorEnvelope(path, err)
	}
	if errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
		return documentUnsupportedErrorEnvelope(path, err)
	}
	if errors.Is(err, operatorsettings.ErrDocumentConflict) {
		return documentConflictErrorEnvelope(path, err)
	}
	return executionErrorEnvelope(err)
}

func loadDocumentErrorEnvelope(path string, err error) ToolErrorEnvelope {
	var failure operatorsettings.DocumentFailure
	if errors.As(err, &failure) {
		switch failure.Kind {
		case operatorsettings.DocumentFailureKindMalformed:
			return documentMalformedErrorEnvelope(documentFailurePath(path, failure), err)
		case operatorsettings.DocumentFailureKindNotFound:
			return documentNotFoundErrorEnvelope(documentFailurePath(path, failure), err)
		}
	}
	if errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		return documentMalformedErrorEnvelope(path, err)
	}
	if errors.Is(err, operatorsettings.ErrDocumentNotFound) {
		return documentNotFoundErrorEnvelope(path, err)
	}
	return executionErrorEnvelope(err)
}

func documentFailurePath(requestPath string, failure operatorsettings.DocumentFailure) string {
	if trimmed := strings.TrimSpace(failure.Path); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(requestPath)
}

func documentMalformedErrorEnvelope(path string, err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeDocumentMalformed,
		Message:   errorMessageDocumentMalformed,
		Retryable: false,
		Details:   documentFailureDetails(path, err),
	}
}

func documentNotFoundErrorEnvelope(path string, err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeDocumentNotFound,
		Message:   errorMessageDocumentNotFound,
		Retryable: false,
		Details:   documentFailureDetails(path, err),
	}
}

func documentUnsupportedErrorEnvelope(path string, err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeDocumentUnsupported,
		Message:   errorMessageDocumentUnsupported,
		Retryable: false,
		Details:   documentFailureDetails(path, err),
	}
}

func documentConflictErrorEnvelope(path string, err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeDocumentConflict,
		Message:   errorMessageDocumentConflict,
		Retryable: false,
		Details:   documentFailureDetails(path, err),
	}
}

func documentFailureDetails(path string, err error) map[string]any {
	details := map[string]any{
		"reason": err.Error(),
	}
	if trimmed := strings.TrimSpace(path); trimmed != "" {
		details["path"] = trimmed
	}
	return details
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
