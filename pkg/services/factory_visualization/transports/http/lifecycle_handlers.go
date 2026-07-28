package http

import (
	"context"
	"errors"
	"io"
	"net/http"

	"go.uber.org/zap"
)

// ActivateLifecycleHTTP decodes an owned Activate HTTP request, invokes the
// Visualization root Activate slice, and encodes the lifecycle success shape.
func (a *Adapter) ActivateLifecycleHTTP(
	ctx context.Context,
	body io.Reader,
) (LifecycleHTTPResponse, error) {
	req, err := decodeActivateHTTPRequest(body)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := a.Activate(ctx, req)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	return lifecycleHTTPResponseFromActivateResult(result), nil
}

// JoinLifecycleHTTP decodes an owned Join HTTP request, invokes the
// Visualization root Join slice, and encodes the lifecycle success shape.
func (a *Adapter) JoinLifecycleHTTP(
	ctx context.Context,
	body io.Reader,
) (LifecycleHTTPResponse, error) {
	req, err := decodeJoinHTTPRequest(body)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := a.Join(ctx, req)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	return lifecycleHTTPResponseFromJoinResult(result), nil
}

// StopDrainLifecycleHTTP decodes an owned StopDrain HTTP request, invokes the
// Visualization root StopDrain slice, and encodes the lifecycle success shape.
func (a *Adapter) StopDrainLifecycleHTTP(
	ctx context.Context,
	body io.Reader,
) (LifecycleHTTPResponse, error) {
	req, err := decodeStopDrainHTTPRequest(body)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := a.StopDrain(ctx, req)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	return lifecycleHTTPResponseFromStopDrainResult(result), nil
}

// HandleActivateLifecycle serves POST /factory-visualization/lifecycle/activate.
func (a *Adapter) HandleActivateLifecycle(w http.ResponseWriter, r *http.Request) {
	response, err := a.ActivateLifecycleHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeLifecycleRequestError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// HandleJoinLifecycle serves POST /factory-visualization/lifecycle/join.
func (a *Adapter) HandleJoinLifecycle(w http.ResponseWriter, r *http.Request) {
	response, err := a.JoinLifecycleHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeLifecycleRequestError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// HandleStopDrainLifecycle serves POST /factory-visualization/lifecycle/stop-drain.
func (a *Adapter) HandleStopDrainLifecycle(w http.ResponseWriter, r *http.Request) {
	response, err := a.StopDrainLifecycleHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeLifecycleRequestError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

func (a *Adapter) writeLifecycleRequestError(w http.ResponseWriter, err error) {
	if message, ok := requestFieldValidationMessage(err); ok {
		a.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
		return
	}
	var decodeErr lifecycleHTTPDecodeError
	if errors.As(err, &decodeErr) {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	a.logger.Error("factory visualization lifecycle request failed", zap.Error(err))
	a.writeError(w, http.StatusInternalServerError, "factory visualization lifecycle request failed", "INTERNAL_ERROR")
}
