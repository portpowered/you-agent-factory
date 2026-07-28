package http

import (
	"context"
	"errors"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var errRequestBodyRequired = errors.New("request body is required")

// PlanDispatch handles Runtime root dispatch intent planning through the
// accepted root slice.
func (a *Adapter) PlanDispatch(w http.ResponseWriter, r *http.Request) {
	req, err := decodePlanDispatchRequest(r.Body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	a.invokePlanDispatch(w, r.Context(), req)
}

// AcceptDispatchResult handles correlated worker result acceptance through the
// accepted Runtime root slice.
func (a *Adapter) AcceptDispatchResult(w http.ResponseWriter, r *http.Request) {
	req, err := decodeAcceptDispatchResultRequest(r.Body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	a.invokeAcceptDispatchResult(w, r.Context(), req)
}

func (a *Adapter) invokePlanDispatch(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.PlanDispatchRequest,
) {
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.PlanDispatch(ctx, req)
	if err != nil {
		if a.writeDispatchPlanError(w, err) {
			return
		}
		a.writeError(w, http.StatusInternalServerError, "failed to plan dispatch", "INTERNAL_ERROR")
		return
	}
	a.writeJSON(w, http.StatusOK, dispatchPlanResponseFromPlanResult(result))
}

func (a *Adapter) invokeAcceptDispatchResult(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.AcceptDispatchResultRequest,
) {
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.AcceptDispatchResult(ctx, req)
	if err != nil {
		if a.writeDispatchPlanError(w, err) {
			return
		}
		a.writeError(w, http.StatusInternalServerError, "failed to accept dispatch result", "INTERNAL_ERROR")
		return
	}
	a.writeJSON(w, http.StatusOK, dispatchPlanResponseFromAcceptResult(result))
}

func (a *Adapter) writeDispatchPlanError(w http.ResponseWriter, err error) bool {
	if status, response, ok := dispatchPlanErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func dispatchPlanErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}
	switch {
	case errors.Is(err, factoryruntime.ErrDuplicateDispatchIntent):
		return conflictErrorResponse("dispatch intent conflicts with an existing plan")
	case errors.Is(err, factoryruntime.ErrUnknownDispatchCorrelation):
		return notFoundErrorResponse("dispatch correlation not found")
	case errors.Is(err, factoryruntime.ErrInvalidDispatchResultBoundary):
		return badRequestErrorResponse("invalid dispatch result boundary")
	case errors.Is(err, factoryruntime.ErrNotFound):
		return notFoundErrorResponse("factory runtime target not found")
	case errors.Is(err, factoryruntime.ErrNotRunning):
		return serviceUnavailableErrorResponse("factory runtime is not running")
	case errors.Is(err, factoryruntime.ErrCapabilityUnavailable):
		return serviceUnavailableErrorResponse("factory runtime capability is unavailable")
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}
