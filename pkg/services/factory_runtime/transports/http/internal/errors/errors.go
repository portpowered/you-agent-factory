// Package errors maps Factory Runtime root failures to the public HTTP error
// contract. It owns no Runtime policy; it only translates published sentinels.
package errors

import (
	"context"
	stderrors "errors"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ErrSessionObserverRequired identifies an unavailable session-scoped Runtime
// observation peer without leaking a Sessions implementation into the handler.
var ErrSessionObserverRequired = stderrors.New("factory session observation is required for session-scoped status reads")

// Operation identifies the Runtime operation whose typed failures are being
// mapped. Keeping this value in the transport error package prevents operation
// handlers from sharing a parent-package error policy implementation.
type Operation int

const (
	OperationObserve Operation = iota
	OperationControl
	OperationMoveWork
	OperationDispatchPlan
	OperationCheckpoint
)

// RootErrorResponse maps published Runtime root sentinel failures to HTTP
// status and the generated ErrorResponse shape.
func RootErrorResponse(err error, operation Operation) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	switch operation {
	case OperationObserve:
		if stderrors.Is(err, ErrSessionObserverRequired) {
			return serviceUnavailableErrorResponse("factory status is unavailable")
		}
		if stderrors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return notFoundErrorResponse("factory session not found")
		}
		if stderrors.Is(err, factoryruntime.ErrInvalidObservationScope) {
			return badRequestErrorResponse("invalid observation scope")
		}
	case OperationControl:
		if stderrors.Is(err, factoryruntime.ErrAlreadyStopped) {
			return conflictErrorResponse("factory runtime is already stopped")
		}
		if stderrors.Is(err, factoryruntime.ErrInvalidLifecycleTransition) {
			return badRequestErrorResponse("factory runtime invalid lifecycle transition")
		}
	case OperationMoveWork:
		if stderrors.Is(err, factoryruntime.ErrMoveWorkNotFound) {
			return notFoundErrorResponse("work not found")
		}
		if stderrors.Is(err, factoryruntime.ErrMoveWorkInvalidState) {
			return badRequestErrorResponse("invalid target state for work type")
		}
		if stderrors.Is(err, factoryruntime.ErrMoveWorkInFlightDispatch) {
			return badRequestErrorResponse("work is in an active dispatch")
		}
		if stderrors.Is(err, factoryruntime.ErrMoveWorkEngineTerminated) {
			return badRequestErrorResponse("engine has terminated")
		}
		if stderrors.Is(err, factoryruntime.ErrMoveWorkRequestConflict) {
			return moveWorkRequestConflictErrorResponse()
		}
	case OperationDispatchPlan:
		if stderrors.Is(err, factoryruntime.ErrDuplicateDispatchIntent) {
			return conflictErrorResponse("dispatch intent conflicts with an existing plan")
		}
		if stderrors.Is(err, factoryruntime.ErrUnknownDispatchCorrelation) {
			return notFoundErrorResponse("dispatch correlation not found")
		}
		if stderrors.Is(err, factoryruntime.ErrInvalidDispatchResultBoundary) {
			return badRequestErrorResponse("invalid dispatch result boundary")
		}
	case OperationCheckpoint:
		if stderrors.Is(err, factoryruntime.ErrCheckpointNotFound) {
			return notFoundErrorResponse("factory runtime checkpoint not found")
		}
		if stderrors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
			return badRequestErrorResponse("factory runtime checkpoint is corrupt")
		}
		if stderrors.Is(err, factoryruntime.ErrIncompatibleCheckpoint) {
			return conflictErrorResponse("factory runtime checkpoint is incompatible")
		}
	}

	if stderrors.Is(err, factoryruntime.ErrCapabilityUnavailable) {
		return serviceUnavailableErrorResponse("factory runtime capability is unavailable")
	}
	if stderrors.Is(err, factoryruntime.ErrNotRunning) {
		return serviceUnavailableErrorResponse("factory runtime is not running")
	}
	if stderrors.Is(err, factoryruntime.ErrNotFound) {
		return notFoundErrorResponse("factory runtime target not found")
	}

	return 0, factoryapi.ErrorResponse{}, false
}

// WriteRootOrInternalError emits context, typed-root, or sanitized internal
// failure responses in that order.
func WriteRootOrInternalError(
	w http.ResponseWriter,
	ctx context.Context,
	operation Operation,
	internalMessage string,
	err error,
) {
	if common.RequestContextEnded(ctx) {
		common.WriteRequestContextOutcome(w, ctx.Err())
		return
	}
	if common.WriteRequestContextOutcome(w, err) {
		return
	}
	if status, response, ok := RootErrorResponse(err, operation); ok {
		common.WriteJSON(w, status, response)
		return
	}
	common.WriteError(w, http.StatusInternalServerError, internalMessage, "INTERNAL_ERROR")
}

func badRequestErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusBadRequest, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCodeBADREQUEST,
	}, true
}

func notFoundErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusNotFound, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyNotFound,
		Code:    factoryapi.ErrorResponseCodeNOTFOUND,
	}, true
}

func conflictErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusConflict, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyConflict,
		Code:    factoryapi.ErrorResponseCode("CONFLICT"),
	}, true
}

func serviceUnavailableErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusServiceUnavailable, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCode("SERVICE_UNAVAILABLE"),
	}, true
}

func moveWorkRequestConflictErrorResponse() (int, factoryapi.ErrorResponse, bool) {
	return http.StatusConflict, factoryapi.ErrorResponse{
		Message: "Operator move request was already applied.",
		Family:  factoryapi.ErrorFamilyConflict,
		Code:    factoryapi.ErrorResponseCodeMOVEWORKREQUESTALREADYAPPLIED,
	}, true
}
