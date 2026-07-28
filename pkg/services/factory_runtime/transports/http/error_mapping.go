package http

import (
	"errors"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

var errSessionObserverRequired = errors.New("factory session observation is required for session-scoped status reads")

type runtimeHTTPOperation int

const (
	runtimeHTTPOperationObserve runtimeHTTPOperation = iota
	runtimeHTTPOperationControl
	runtimeHTTPOperationMoveWork
	runtimeHTTPOperationDispatchPlan
	runtimeHTTPOperationCheckpoint
)

// RootErrorResponse maps published Runtime root sentinel failures to HTTP status
// and the public ErrorResponse shape. It returns false when err is not a known
// mapped typed failure for the operation.
func RootErrorResponse(err error, operation runtimeHTTPOperation) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	switch operation {
	case runtimeHTTPOperationObserve:
		if errors.Is(err, errSessionObserverRequired) {
			return serviceUnavailableErrorResponse("factory status is unavailable")
		}
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return notFoundErrorResponse("factory session not found")
		}
		if errors.Is(err, factoryruntime.ErrInvalidObservationScope) {
			return badRequestErrorResponse("invalid observation scope")
		}
	case runtimeHTTPOperationControl:
		if errors.Is(err, factoryruntime.ErrAlreadyStopped) {
			return conflictErrorResponse("factory runtime is already stopped")
		}
		if errors.Is(err, factoryruntime.ErrInvalidLifecycleTransition) {
			return badRequestErrorResponse("factory runtime invalid lifecycle transition")
		}
	case runtimeHTTPOperationMoveWork:
		if errors.Is(err, factoryruntime.ErrMoveWorkNotFound) {
			return notFoundErrorResponse("work not found")
		}
		if errors.Is(err, factoryruntime.ErrMoveWorkInvalidState) {
			return badRequestErrorResponse("invalid target state for work type")
		}
		if errors.Is(err, factoryruntime.ErrMoveWorkInFlightDispatch) {
			return badRequestErrorResponse("work is in an active dispatch")
		}
		if errors.Is(err, factoryruntime.ErrMoveWorkEngineTerminated) {
			return badRequestErrorResponse("engine has terminated")
		}
		if errors.Is(err, factoryruntime.ErrMoveWorkRequestConflict) {
			return moveWorkRequestConflictErrorResponse()
		}
	case runtimeHTTPOperationDispatchPlan:
		if errors.Is(err, factoryruntime.ErrDuplicateDispatchIntent) {
			return conflictErrorResponse("dispatch intent conflicts with an existing plan")
		}
		if errors.Is(err, factoryruntime.ErrUnknownDispatchCorrelation) {
			return notFoundErrorResponse("dispatch correlation not found")
		}
		if errors.Is(err, factoryruntime.ErrInvalidDispatchResultBoundary) {
			return badRequestErrorResponse("invalid dispatch result boundary")
		}
	case runtimeHTTPOperationCheckpoint:
		if errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
			return notFoundErrorResponse("factory runtime checkpoint not found")
		}
		if errors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
			return badRequestErrorResponse("factory runtime checkpoint is corrupt")
		}
		if errors.Is(err, factoryruntime.ErrIncompatibleCheckpoint) {
			return conflictErrorResponse("factory runtime checkpoint is incompatible")
		}
	}

	if errors.Is(err, factoryruntime.ErrCapabilityUnavailable) {
		return serviceUnavailableErrorResponse("factory runtime capability is unavailable")
	}
	if errors.Is(err, factoryruntime.ErrNotRunning) {
		return serviceUnavailableErrorResponse("factory runtime is not running")
	}
	if errors.Is(err, factoryruntime.ErrNotFound) {
		return notFoundErrorResponse("factory runtime target not found")
	}

	return 0, factoryapi.ErrorResponse{}, false
}

func (a *Adapter) writeRootError(w http.ResponseWriter, operation runtimeHTTPOperation, err error) bool {
	if status, response, ok := RootErrorResponse(err, operation); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func (a *Adapter) writeRootOrInternalError(
	w http.ResponseWriter,
	operation runtimeHTTPOperation,
	internalMessage string,
	err error,
) {
	if a.writeRootError(w, operation, err) {
		return
	}
	a.writeError(w, http.StatusInternalServerError, internalMessage, "INTERNAL_ERROR")
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
