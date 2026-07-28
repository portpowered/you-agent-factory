package modelmcp

import (
	"errors"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

const (
	errorCodeBadRequest            = "BAD_REQUEST"
	errorCodeRuntimeScopeInvalid   = "model.runtime_scope.invalid"
	errorCodeRuntimeScopeStale     = "model.runtime_scope.stale"
	errorCodeRuntimeScopeForeign   = "model.runtime_scope.foreign"
	errorCodeRuntimeScopeClosed    = "model.runtime_scope.closed"
	errorCodeCatalogUnavailable    = "model.catalog.unavailable"
	errorCodeInternalExecution     = "model.execution.internal"
	errorMessageRuntimeScopeInvalid = "models runtime scope is invalid"
	errorMessageRuntimeScopeStale   = "models runtime scope is stale"
	errorMessageRuntimeScopeForeign = "models runtime scope is foreign"
	errorMessageRuntimeScopeClosed  = "models runtime scope is closed"
	errorMessageCatalogUnavailable  = "models catalog is unavailable"
	errorMessageInternalExecution   = "models execution failed"
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
		Code:      "model.service.unavailable",
		Message:   "models service is unavailable",
		Retryable: false,
	}
}

func listCatalogErrorEnvelope(err error) ToolErrorEnvelope {
	if envelope, ok := runtimeScopeErrorEnvelope(err); ok {
		return envelope
	}
	if errors.Is(err, models.ErrUnavailable) {
		return ToolErrorEnvelope{
			Code:      errorCodeCatalogUnavailable,
			Message:   errorMessageCatalogUnavailable,
			Retryable: true,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	}
	return executionErrorEnvelope(err)
}

func runtimeScopeErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	switch {
	case errors.Is(err, models.ErrRuntimeScopeInvalid):
		return ToolErrorEnvelope{
			Code:      errorCodeRuntimeScopeInvalid,
			Message:   errorMessageRuntimeScopeInvalid,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}, true
	case errors.Is(err, models.ErrRuntimeScopeStale):
		return ToolErrorEnvelope{
			Code:      errorCodeRuntimeScopeStale,
			Message:   errorMessageRuntimeScopeStale,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}, true
	case errors.Is(err, models.ErrRuntimeScopeForeign):
		return ToolErrorEnvelope{
			Code:      errorCodeRuntimeScopeForeign,
			Message:   errorMessageRuntimeScopeForeign,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}, true
	case errors.Is(err, models.ErrRuntimeScopeClosed):
		return ToolErrorEnvelope{
			Code:      errorCodeRuntimeScopeClosed,
			Message:   errorMessageRuntimeScopeClosed,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}, true
	default:
		return ToolErrorEnvelope{}, false
	}
}

func executionErrorEnvelope(err error) ToolErrorEnvelope {
	message := errorMessageInternalExecution
	details := map[string]any{}
	if err != nil {
		if trimmed := strings.TrimSpace(err.Error()); trimmed != "" {
			message = trimmed
		}
		details["reason"] = err.Error()
	}
	return ToolErrorEnvelope{
		Code:      errorCodeInternalExecution,
		Message:   message,
		Retryable: false,
		Details:   details,
	}
}
