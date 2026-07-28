package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ControlPause handles Runtime root pause control through the accepted root slice.
func (a *Adapter) ControlPause(w http.ResponseWriter, r *http.Request) {
	a.invokeControlPause(w, r.Context())
}

// ControlResume handles Runtime root resume control through the accepted root slice.
func (a *Adapter) ControlResume(w http.ResponseWriter, r *http.Request) {
	a.invokeControlResume(w, r.Context())
}

// ControlTerminate handles Runtime root terminate control through the accepted root slice.
func (a *Adapter) ControlTerminate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeTerminateControlRequest(r.Body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	a.invokeControlTerminate(w, r.Context(), req)
}

func (a *Adapter) invokeControlPause(w http.ResponseWriter, ctx context.Context) {
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.ControlPause(ctx, factoryruntime.PauseRequest{})
	if err != nil {
		a.writeRootOrInternalError(w, runtimeHTTPOperationControl, "failed to pause factory runtime", err)
		return
	}
	a.writeJSON(w, http.StatusOK, controlResponseFromPauseResult(result))
}

func (a *Adapter) invokeControlResume(w http.ResponseWriter, ctx context.Context) {
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.ControlResume(ctx, factoryruntime.ResumeRequest{})
	if err != nil {
		a.writeRootOrInternalError(w, runtimeHTTPOperationControl, "failed to resume factory runtime", err)
		return
	}
	a.writeJSON(w, http.StatusOK, controlResponseFromResumeResult(result))
}

func (a *Adapter) invokeControlTerminate(
	w http.ResponseWriter,
	ctx context.Context,
	req factoryruntime.TerminateRequest,
) {
	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.ControlTerminate(ctx, req)
	if err != nil {
		a.writeRootOrInternalError(w, runtimeHTTPOperationControl, "failed to terminate factory runtime", err)
		return
	}
	a.writeJSON(w, http.StatusOK, controlResponseFromTerminateResult(result))
}

func decodeTerminateControlRequest(body io.Reader) (factoryruntime.TerminateRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryruntime.TerminateRequest{}, err
	}
	if len(payload) == 0 {
		return factoryruntime.TerminateRequest{}, nil
	}
	var req factoryapi.FactorySessionLifecycleControlRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryruntime.TerminateRequest{}, err
	}
	return terminateRequestFromAPI(req), nil
}

func decodeMoveWorkRequestBody(body io.Reader) (factoryapi.MoveWorkRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	if len(payload) == 0 {
		return factoryapi.MoveWorkRequest{}, errors.New("request body is required")
	}
	var req factoryapi.MoveWorkRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	return req, nil
}

// MoveWorkBySessionId handles operator move-work through the accepted Runtime root slice.
// Session identity is preserved for generated-contract compatibility; root invocation
// uses the published Runtime move vocabulary only.
func (a *Adapter) MoveWorkBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	_ factoryapi.SessionID,
	workID factoryapi.WorkOrTokenID,
) {
	req, err := decodeMoveWorkRequestBody(r.Body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	moveReq := moveWorkRequestFromAPI(string(workID), req)
	if moveReq.StateName == "" {
		a.writeError(w, http.StatusBadRequest, "stateName is required", "BAD_REQUEST")
		return
	}

	root, err := a.runtimeRoot()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	result, err := root.ControlMoveWork(r.Context(), moveReq)
	if err != nil {
		a.writeRootOrInternalError(w, runtimeHTTPOperationMoveWork, "failed to move work", err)
		return
	}
	a.writeJSON(w, http.StatusOK, workResponseFromMoveResult(result))
}
