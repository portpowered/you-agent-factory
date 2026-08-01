// Package dispatch owns Runtime HTTP dispatch-plan and result-ingress
// operations. PlanDispatch remains available here until the Runtime root
// cutover removes its external command surface.
package dispatch

import (
	"context"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	common "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/errors"
)

// Handler adapts Runtime dispatch operations through one injected root.
type Handler struct {
	root factoryruntime.Service
}

// NewHandler binds dispatch operations to the already-constructed Runtime root.
func NewHandler(root factoryruntime.Service) *Handler {
	if root == nil {
		return nil
	}
	return &Handler{root: root}
}

// PlanDispatch handles Runtime dispatch intent planning.
func (h *Handler) PlanDispatch(w http.ResponseWriter, r *http.Request) {
	req, err := decodePlanDispatchRequest(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	h.invokePlanDispatch(w, r.Context(), req)
}

// AcceptDispatchResult handles correlated worker-result acceptance.
func (h *Handler) AcceptDispatchResult(w http.ResponseWriter, r *http.Request) {
	req, err := decodeAcceptDispatchResultRequest(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	h.invokeAcceptDispatchResult(w, r.Context(), req)
}

func (h *Handler) invokePlanDispatch(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.PlanDispatchRequest,
) {
	if common.WriteRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.PlanDispatch(ctx, req)
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, ctx, transporterrors.OperationDispatchPlan, "failed to plan dispatch", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, dispatchPlanResponseFromPlanResult(result))
}

func (h *Handler) invokeAcceptDispatchResult(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.AcceptDispatchResultRequest,
) {
	if common.WriteRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.AcceptDispatchResult(ctx, req)
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, ctx, transporterrors.OperationDispatchPlan, "failed to accept dispatch result", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, dispatchPlanResponseFromAcceptResult(result))
}
