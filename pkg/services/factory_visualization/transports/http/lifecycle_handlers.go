package http

import (
	"context"
	"io"
	"net/http"
)

// ActivateLifecycleHTTP decodes an owned Activate HTTP request, invokes the
// Visualization root Activate slice, and encodes the lifecycle success shape.
func (a *Adapter) ActivateLifecycleHTTP(
	ctx context.Context,
	body io.Reader,
) (LifecycleHTTPResponse, error) {
	req, diagnostics, err := decodeActivateHTTPRequest(body)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	if err := visualizationContextBeforeRoot(ctx); err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := a.Activate(ctx, req)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	response := lifecycleHTTPResponseFromActivateResult(result)
	response.ignoredJSONPaths = diagnostics.Paths()
	return response, nil
}

// JoinLifecycleHTTP decodes an owned Join HTTP request, invokes the
// Visualization root Join slice, and encodes the lifecycle success shape.
func (a *Adapter) JoinLifecycleHTTP(
	ctx context.Context,
	body io.Reader,
) (LifecycleHTTPResponse, error) {
	req, diagnostics, err := decodeJoinHTTPRequest(body)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	if err := visualizationContextBeforeRoot(ctx); err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := a.Join(ctx, req)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	response := lifecycleHTTPResponseFromJoinResult(result)
	response.ignoredJSONPaths = diagnostics.Paths()
	return response, nil
}

// StopDrainLifecycleHTTP decodes an owned StopDrain HTTP request, invokes the
// Visualization root StopDrain slice, and encodes the lifecycle success shape.
func (a *Adapter) StopDrainLifecycleHTTP(
	ctx context.Context,
	body io.Reader,
) (LifecycleHTTPResponse, error) {
	req, diagnostics, err := decodeStopDrainHTTPRequest(body)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	if err := visualizationContextBeforeRoot(ctx); err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := a.StopDrain(ctx, req)
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	response := lifecycleHTTPResponseFromStopDrainResult(result)
	response.ignoredJSONPaths = diagnostics.Paths()
	return response, nil
}

// HandleActivateLifecycle serves POST /factory-visualization/lifecycle/activate.
func (a *Adapter) HandleActivateLifecycle(w http.ResponseWriter, r *http.Request) {
	response, err := a.ActivateLifecycleHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeLifecycleRequestError(w, err)
		return
	}
	a.writeCompatibilityWarning(w, "activate_lifecycle", response.ignoredJSONPaths)
	a.writeJSON(w, http.StatusOK, response)
}

// HandleJoinLifecycle serves POST /factory-visualization/lifecycle/join.
func (a *Adapter) HandleJoinLifecycle(w http.ResponseWriter, r *http.Request) {
	response, err := a.JoinLifecycleHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeLifecycleRequestError(w, err)
		return
	}
	a.writeCompatibilityWarning(w, "join_lifecycle", response.ignoredJSONPaths)
	a.writeJSON(w, http.StatusOK, response)
}

// HandleStopDrainLifecycle serves POST /factory-visualization/lifecycle/stop-drain.
func (a *Adapter) HandleStopDrainLifecycle(w http.ResponseWriter, r *http.Request) {
	response, err := a.StopDrainLifecycleHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeLifecycleRequestError(w, err)
		return
	}
	a.writeCompatibilityWarning(w, "stop_drain_lifecycle", response.ignoredJSONPaths)
	a.writeJSON(w, http.StatusOK, response)
}

func (a *Adapter) writeLifecycleRequestError(w http.ResponseWriter, err error) {
	a.writeVisualizationRequestError(w, err, "factory visualization lifecycle request failed")
}
