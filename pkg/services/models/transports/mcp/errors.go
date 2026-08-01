package modelmcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

var (
	errEmptyModelName      = fmt.Errorf("%w: empty model name", models.ErrNotFound)
	errEmptyLeaseHolder    = fmt.Errorf("%w: empty lease holder", models.ErrHostInvalidHolder)
	errEmptyModelOperation = fmt.Errorf("%w: empty model operation", models.ErrUnsupportedModelOperation)
)

const (
	errorCodeBadRequest                      = "BAD_REQUEST"
	errorCodeRuntimeScopeInvalid             = "model.runtime_scope.invalid"
	errorCodeRuntimeScopeStale               = "model.runtime_scope.stale"
	errorCodeRuntimeScopeForeign             = "model.runtime_scope.foreign"
	errorCodeRuntimeScopeClosed              = "model.runtime_scope.closed"
	errorCodeCatalogUnavailable              = "model.catalog.unavailable"
	errorCodeAssetSourceMissing              = "model.asset.source_missing"
	errorCodeAssetSourceUnsupported          = "model.asset.source_unsupported"
	errorCodeAssetIntegrityFailed            = "model.asset.integrity_failed"
	errorCodeAssetPreparationInterrupted     = "model.asset.preparation_interrupted"
	errorCodeAssetUnavailable                = "model.asset.unavailable"
	errorCodeLeaseCapacityExhausted          = "model.lease.capacity_exhausted"
	errorCodeLeaseCapacityContended          = "model.lease.capacity_contended"
	errorCodeLeaseExpired                    = "model.lease.expired"
	errorCodeLeaseInvalidHolder              = "model.lease.invalid_holder"
	errorCodeLeaseNotFound                   = "model.lease.not_found"
	errorCodeHostRuntimeNotReady             = "model.host.runtime_not_ready"
	errorCodeInferenceFailed                 = "model.inference.failed"
	errorCodeInferenceTimeout                = "model.inference.timeout"
	errorCodeModelOperationUnsupported       = "model.operation.unsupported"
	errorCodeInferenceResponseUnsupported    = "model.inference.response_mode_unsupported"
	errorCodeInternalExecution               = "model.execution.internal"
	errorCodeRequestCanceled                 = "model.request.canceled"
	errorCodeRequestTimedOut                 = "model.request.timed_out"
	errorMessageRuntimeScopeInvalid          = "models runtime scope is invalid"
	errorMessageRuntimeScopeStale            = "models runtime scope is stale"
	errorMessageRuntimeScopeForeign          = "models runtime scope is foreign"
	errorMessageRuntimeScopeClosed           = "models runtime scope is closed"
	errorMessageCatalogUnavailable           = "models catalog is unavailable"
	errorMessageAssetSourceMissing           = "models asset source is missing"
	errorMessageAssetSourceUnsupported       = "models asset source is unsupported"
	errorMessageAssetIntegrityFailed         = "models asset integrity verification failed"
	errorMessageAssetPreparationInterrupted  = "models asset preparation was interrupted"
	errorMessageAssetUnavailable             = "models assets are unavailable"
	errorMessageLeaseCapacityExhausted       = "models lease capacity is exhausted"
	errorMessageLeaseCapacityContended       = "models lease capacity is contended"
	errorMessageLeaseExpired                 = "models lease has expired"
	errorMessageLeaseInvalidHolder           = "models lease holder is invalid"
	errorMessageLeaseNotFound                = "models lease was not found"
	errorMessageHostRuntimeNotReady          = "models host runtime is not ready"
	errorMessageInferenceFailed              = "models inference failed"
	errorMessageInferenceTimeout             = "models inference timed out"
	errorMessageModelOperationUnsupported    = "models operation is not supported"
	errorMessageInferenceResponseUnsupported = "models inference response mode is not supported"
	errorMessageInternalExecution            = "models execution failed"
	errorMessageRequestCanceled              = "models request was canceled"
	errorMessageRequestTimedOut              = "models request timed out"
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
		Code:      "model.service.unavailable",
		Message:   "models service is unavailable",
		Retryable: false,
	}
}

func prepareAssetsErrorEnvelope(err error) ToolErrorEnvelope {
	if envelope, ok := runtimeScopeErrorEnvelope(err); ok {
		return envelope
	}
	switch {
	case errors.Is(err, models.ErrAssetSourceMissing):
		return ToolErrorEnvelope{
			Code:      errorCodeAssetSourceMissing,
			Message:   errorMessageAssetSourceMissing,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, models.ErrAssetSourceUnsupported):
		return ToolErrorEnvelope{
			Code:      errorCodeAssetSourceUnsupported,
			Message:   errorMessageAssetSourceUnsupported,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, models.ErrAssetIntegrityFailed):
		return ToolErrorEnvelope{
			Code:      errorCodeAssetIntegrityFailed,
			Message:   errorMessageAssetIntegrityFailed,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, models.ErrAssetPreparationInterrupted):
		return ToolErrorEnvelope{
			Code:      errorCodeAssetPreparationInterrupted,
			Message:   errorMessageAssetPreparationInterrupted,
			Retryable: true,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	default:
		return executionErrorEnvelope(err)
	}
}

func acquireLeaseErrorEnvelope(err error) ToolErrorEnvelope {
	if envelope, ok := runtimeScopeErrorEnvelope(err); ok {
		return envelope
	}
	switch {
	case errors.Is(err, models.ErrHostCapacityExhausted):
		return ToolErrorEnvelope{
			Code:      errorCodeLeaseCapacityExhausted,
			Message:   errorMessageLeaseCapacityExhausted,
			Retryable: true,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, models.ErrHostCapacityContended):
		return ToolErrorEnvelope{
			Code:      errorCodeLeaseCapacityContended,
			Message:   errorMessageLeaseCapacityContended,
			Retryable: true,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, models.ErrHostRuntimeNotReady):
		return ToolErrorEnvelope{
			Code:      errorCodeHostRuntimeNotReady,
			Message:   errorMessageHostRuntimeNotReady,
			Retryable: true,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	case errors.Is(err, models.ErrHostInvalidHolder):
		return ToolErrorEnvelope{
			Code:      errorCodeLeaseInvalidHolder,
			Message:   errorMessageLeaseInvalidHolder,
			Retryable: false,
			Details: map[string]any{
				"reason": err.Error(),
			},
		}
	default:
		return executionErrorEnvelope(err)
	}
}

func invokeWithLeaseErrorEnvelope(err error) ToolErrorEnvelope {
	if envelope, ok := runtimeScopeErrorEnvelope(err); ok {
		return envelope
	}
	if envelope, ok := invokeWithLeaseHostErrorEnvelope(err); ok {
		return envelope
	}
	if envelope, ok := invokeWithLeaseInferenceErrorEnvelope(err); ok {
		return envelope
	}
	return executionErrorEnvelope(err)
}

func invokeWithLeaseHostErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	switch {
	case errors.Is(err, models.ErrHostCapacityExhausted):
		return reasonDetailEnvelope(errorCodeLeaseCapacityExhausted, errorMessageLeaseCapacityExhausted, true, err), true
	case errors.Is(err, models.ErrHostCapacityContended):
		return reasonDetailEnvelope(errorCodeLeaseCapacityContended, errorMessageLeaseCapacityContended, true, err), true
	case errors.Is(err, models.ErrHostRuntimeNotReady):
		return reasonDetailEnvelope(errorCodeHostRuntimeNotReady, errorMessageHostRuntimeNotReady, true, err), true
	case errors.Is(err, models.ErrHostLeaseExpired):
		return reasonDetailEnvelope(errorCodeLeaseExpired, errorMessageLeaseExpired, false, err), true
	case errors.Is(err, models.ErrHostLeaseNotFound):
		return reasonDetailEnvelope(errorCodeLeaseNotFound, errorMessageLeaseNotFound, false, err), true
	case errors.Is(err, models.ErrHostInvalidHolder):
		return reasonDetailEnvelope(errorCodeLeaseInvalidHolder, errorMessageLeaseInvalidHolder, false, err), true
	default:
		return ToolErrorEnvelope{}, false
	}
}

func invokeWithLeaseInferenceErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	switch {
	case errors.Is(err, models.ErrAssetUnavailable):
		return reasonDetailEnvelope(errorCodeAssetUnavailable, errorMessageAssetUnavailable, true, err), true
	case errors.Is(err, models.ErrInferenceTimeout):
		return reasonDetailEnvelope(errorCodeInferenceTimeout, errorMessageInferenceTimeout, true, err), true
	case errors.Is(err, models.ErrInferenceFailed):
		return reasonDetailEnvelope(errorCodeInferenceFailed, errorMessageInferenceFailed, false, err), true
	case errors.Is(err, models.ErrUnsupportedModelOperation):
		return reasonDetailEnvelope(errorCodeModelOperationUnsupported, errorMessageModelOperationUnsupported, false, err), true
	case errors.Is(err, models.ErrUnsupportedResponseMode):
		return reasonDetailEnvelope(errorCodeInferenceResponseUnsupported, errorMessageInferenceResponseUnsupported, false, err), true
	default:
		return ToolErrorEnvelope{}, false
	}
}

func reasonDetailEnvelope(code, message string, retryable bool, err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Details: map[string]any{
			"reason": err.Error(),
		},
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
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
		return envelope
	}
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
