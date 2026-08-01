// Package checkpoint owns Runtime HTTP checkpoint capture, inspection, and
// restore operations.
package checkpoint

import (
	"context"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	common "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/errors"
)

// Handler adapts Runtime checkpoint operations through one injected root.
type Handler struct {
	root factoryruntime.Service
}

// NewHandler binds checkpoint operations to the already-constructed Runtime
// root.
func NewHandler(root factoryruntime.Service) *Handler {
	if root == nil {
		return nil
	}
	return &Handler{root: root}
}

// CaptureCheckpoint handles Runtime checkpoint capture.
func (h *Handler) CaptureCheckpoint(w http.ResponseWriter, r *http.Request) {
	req, err := decodeCaptureCheckpointRequest(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	h.invokeCaptureCheckpoint(w, r.Context(), req)
}

// LoadCheckpoint handles Runtime checkpoint load/inspect.
func (h *Handler) LoadCheckpoint(w http.ResponseWriter, r *http.Request) {
	req, err := decodeLoadCheckpointRequest(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	h.invokeLoadCheckpoint(w, r.Context(), req)
}

// RestoreCheckpoint handles Runtime checkpoint restore.
func (h *Handler) RestoreCheckpoint(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRestoreCheckpointRequest(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	h.invokeRestoreCheckpoint(w, r.Context(), req)
}

func (h *Handler) invokeCaptureCheckpoint(w http.ResponseWriter, ctx context.Context, req factoryruntime.CaptureCheckpointRequest) {
	if common.WriteRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.CaptureCheckpoint(ctx, req)
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, ctx, transporterrors.OperationCheckpoint, "failed to capture checkpoint", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, captureCheckpointResponseFromResult(result))
}

func (h *Handler) invokeLoadCheckpoint(w http.ResponseWriter, ctx context.Context, req factoryruntime.LoadCheckpointRequest) {
	if common.WriteRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.LoadCheckpoint(ctx, req)
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, ctx, transporterrors.OperationCheckpoint, "failed to load checkpoint", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, loadCheckpointResponseFromResult(result))
}

func (h *Handler) invokeRestoreCheckpoint(w http.ResponseWriter, ctx context.Context, req factoryruntime.RestoreCheckpointRequest) {
	if common.WriteRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := common.RequireRuntimeRoot(h.root)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.RestoreCheckpoint(ctx, req)
	if err != nil {
		transporterrors.WriteRootOrInternalError(w, ctx, transporterrors.OperationCheckpoint, "failed to restore checkpoint", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, restoreCheckpointResponseFromResult(result))
}
