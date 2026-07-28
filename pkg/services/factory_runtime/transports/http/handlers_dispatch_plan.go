package http

import (
	"context"
	"errors"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
	if a.writeRuntimeRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.PlanDispatch(ctx, req)
	if err != nil {
		a.writeRootOrInternalError(w, ctx, runtimeHTTPOperationDispatchPlan, "failed to plan dispatch", err)
		return
	}
	a.writeJSON(w, http.StatusOK, dispatchPlanResponseFromPlanResult(result))
}

func (a *Adapter) invokeAcceptDispatchResult(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.AcceptDispatchResultRequest,
) {
	if a.writeRuntimeRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.AcceptDispatchResult(ctx, req)
	if err != nil {
		a.writeRootOrInternalError(w, ctx, runtimeHTTPOperationDispatchPlan, "failed to accept dispatch result", err)
		return
	}
	a.writeJSON(w, http.StatusOK, dispatchPlanResponseFromAcceptResult(result))
}

