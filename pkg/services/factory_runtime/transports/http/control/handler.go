// Package control owns Factory Runtime HTTP lifecycle and operator work-move
// operations.
package control

import (
	"context"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	common "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/errors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Handler adapts Runtime lifecycle controls and the compatibility move-work
// route through one injected Runtime root.
type Handler struct {
	root factoryruntime.Service
}

// NewHandler binds control operations to the already-constructed Runtime root.
func NewHandler(root factoryruntime.Service) *Handler {
	if root == nil {
		return nil
	}
	return &Handler{root: root}
}

// ControlPause handles Runtime root pause control.
func (h *Handler) ControlPause(w http.ResponseWriter, r *http.Request) {
	h.invokeControlPause(w, r.Context())
}

// ControlResume handles Runtime root resume control.
func (h *Handler) ControlResume(w http.ResponseWriter, r *http.Request) {
	h.invokeControlResume(w, r.Context())
}

// InvokeResume exposes the context-only resume path to the parent façade's
// package-local compatibility helper without exposing the handler's internals.
func (h *Handler) InvokeResume(w http.ResponseWriter, ctx context.Context) {
	h.invokeControlResume(w, ctx)
}

// ControlTerminate handles Runtime root terminate control.
func (h *Handler) ControlTerminate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeTerminateControlRequest(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	h.invokeControlTerminate(w, r.Context(), req)
}

func (h *Handler) invokeControlPause(w http.ResponseWriter, ctx context.Context) {
	if common.WriteRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.ControlPause(ctx, factoryruntime.PauseRequest{})
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, ctx, transporterrors.OperationControl, "failed to pause factory runtime", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, controlResponseFromPauseResult(result))
}

func (h *Handler) invokeControlResume(w http.ResponseWriter, ctx context.Context) {
	if common.WriteRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.ControlResume(ctx, factoryruntime.ResumeRequest{})
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, ctx, transporterrors.OperationControl, "failed to resume factory runtime", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, controlResponseFromResumeResult(result))
}

func (h *Handler) invokeControlTerminate(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.TerminateRequest,
) {
	if common.WriteRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.ControlTerminate(ctx, req)
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, ctx, transporterrors.OperationControl, "failed to terminate factory runtime", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, controlResponseFromTerminateResult(result))
}

// MoveWorkBySessionId keeps the generated route's session parameter for API
// compatibility while the Runtime root owns the actual move operation.
func (h *Handler) MoveWorkBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	_ factoryapi.SessionID,
	workID factoryapi.WorkOrTokenID,
) {
	req, err := decodeMoveWorkRequestBody(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	moveReq := moveWorkRequestFromAPI(string(workID), req)
	if moveReq.StateName == "" {
		common.WriteError(w, http.StatusBadRequest, "stateName is required", "BAD_REQUEST")
		return
	}
	if common.GuardRequestContext(w, r) {
		return
	}

	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.ControlMoveWork(r.Context(), moveReq)
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, r.Context(), transporterrors.OperationMoveWork, "failed to move work", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, workResponseFromMoveResult(result))
}
