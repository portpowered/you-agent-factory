package http

import (
	"context"
	"errors"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.CaptureCheckpoint(ctx, req)
	if err != nil {
		if a.writeCheckpointError(w, err) {
			return
		}
		a.writeError(w, http.StatusInternalServerError, "failed to capture checkpoint", "INTERNAL_ERROR")
		return
	}
	a.writeJSON(w, http.StatusOK, captureCheckpointResponseFromResult(result))
}

func (a *Adapter) invokeLoadCheckpoint(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.LoadCheckpointRequest,
) {
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.LoadCheckpoint(ctx, req)
	if err != nil {
		if a.writeCheckpointError(w, err) {
			return
		}
		a.writeError(w, http.StatusInternalServerError, "failed to load checkpoint", "INTERNAL_ERROR")
		return
	}
	a.writeJSON(w, http.StatusOK, loadCheckpointResponseFromResult(result))
}

func (a *Adapter) invokeRestoreCheckpoint(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.RestoreCheckpointRequest,
) {
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.RestoreCheckpoint(ctx, req)
	if err != nil {
		if a.writeCheckpointError(w, err) {
			return
		}
		a.writeError(w, http.StatusInternalServerError, "failed to restore checkpoint", "INTERNAL_ERROR")
		return
	}
	a.writeJSON(w, http.StatusOK, restoreCheckpointResponseFromResult(result))
}

func (a *Adapter) writeCheckpointError(w http.ResponseWriter, err error) bool {
	if status, response, ok := checkpointErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func checkpointErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}
	switch {
	case errors.Is(err, factoryruntime.ErrCheckpointNotFound):
		return notFoundErrorResponse("factory runtime checkpoint not found")
	case errors.Is(err, factoryruntime.ErrCorruptCheckpoint):
		return badRequestErrorResponse("factory runtime checkpoint is corrupt")
	case errors.Is(err, factoryruntime.ErrIncompatibleCheckpoint):
		return conflictErrorResponse("factory runtime checkpoint is incompatible")
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
