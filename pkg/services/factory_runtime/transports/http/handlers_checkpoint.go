package http

import (
	"context"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// CaptureCheckpoint handles Runtime root checkpoint capture through the
// accepted root slice.
func (a *Adapter) CaptureCheckpoint(w http.ResponseWriter, r *http.Request) {
	req, err := decodeCaptureCheckpointRequest(r.Body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	a.invokeCaptureCheckpoint(w, r.Context(), req)
}

// LoadCheckpoint handles Runtime root checkpoint load/inspect through the
// accepted root slice.
func (a *Adapter) LoadCheckpoint(w http.ResponseWriter, r *http.Request) {
	req, err := decodeLoadCheckpointRequest(r.Body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	a.invokeLoadCheckpoint(w, r.Context(), req)
}

// RestoreCheckpoint handles Runtime root checkpoint restore through the
// accepted root slice.
func (a *Adapter) RestoreCheckpoint(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRestoreCheckpointRequest(r.Body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	a.invokeRestoreCheckpoint(w, r.Context(), req)
}

func (a *Adapter) invokeCaptureCheckpoint(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.CaptureCheckpointRequest,
) {
	if a.writeRuntimeRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.CaptureCheckpoint(ctx, req)
	if err != nil {
		a.writeRootOrInternalError(w, ctx, runtimeHTTPOperationCheckpoint, "failed to capture checkpoint", err)
		return
	}
	a.writeJSON(w, http.StatusOK, captureCheckpointResponseFromResult(result))
}

func (a *Adapter) invokeLoadCheckpoint(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.LoadCheckpointRequest,
) {
	if a.writeRuntimeRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.LoadCheckpoint(ctx, req)
	if err != nil {
		a.writeRootOrInternalError(w, ctx, runtimeHTTPOperationCheckpoint, "failed to load checkpoint", err)
		return
	}
	a.writeJSON(w, http.StatusOK, loadCheckpointResponseFromResult(result))
}

func (a *Adapter) invokeRestoreCheckpoint(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.RestoreCheckpointRequest,
) {
	if a.writeRuntimeRequestContextOutcome(w, ctx.Err()) {
		return
	}
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.RestoreCheckpoint(ctx, req)
	if err != nil {
		a.writeRootOrInternalError(w, ctx, runtimeHTTPOperationCheckpoint, "failed to restore checkpoint", err)
		return
	}
	a.writeJSON(w, http.StatusOK, restoreCheckpointResponseFromResult(result))
}

