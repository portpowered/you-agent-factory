package http

import (
	"context"
	"net/http"

	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/errors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The aliases below preserve the old package-local error-mapping vocabulary
// for callers and tests while policy is owned by the focused transport error
// package.
var errSessionObserverRequired = transporterrors.ErrSessionObserverRequired

type runtimeHTTPOperation = transporterrors.Operation

const (
	runtimeHTTPOperationObserve      = transporterrors.OperationObserve
	runtimeHTTPOperationControl      = transporterrors.OperationControl
	runtimeHTTPOperationMoveWork     = transporterrors.OperationMoveWork
	runtimeHTTPOperationDispatchPlan = transporterrors.OperationDispatchPlan
	runtimeHTTPOperationCheckpoint   = transporterrors.OperationCheckpoint
)

// RootErrorResponse preserves the Runtime HTTP adapter's compatibility
// helper while delegating all typed sentinel mapping to the error child
// package.
func RootErrorResponse(err error, operation runtimeHTTPOperation) (int, factoryapi.ErrorResponse, bool) {
	return transporterrors.RootErrorResponse(err, operation)
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
	ctx context.Context,
	operation runtimeHTTPOperation,
	internalMessage string,
	err error,
) {
	transporterrors.WriteRootOrInternalError(w, ctx, operation, internalMessage, err)
}
