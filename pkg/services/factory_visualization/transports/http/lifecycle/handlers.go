package lifecycle

import (
	"context"
	"io"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/common"
	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/errors"
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
	if err := common.ContextBeforeRoot(ctx); err != nil {
		return LifecycleHTTPResponse{}, err
	}
	root, err := a.root()
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := root.Activate(ctx, req)
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
	if err := common.ContextBeforeRoot(ctx); err != nil {
		return LifecycleHTTPResponse{}, err
	}
	root, err := a.root()
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := root.Join(ctx, req)
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
	if err := common.ContextBeforeRoot(ctx); err != nil {
		return LifecycleHTTPResponse{}, err
	}
	root, err := a.root()
	if err != nil {
		return LifecycleHTTPResponse{}, err
	}
	result, err := root.StopDrain(ctx, req)
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
	common.WriteJSON(w, http.StatusOK, response, a.logger)
}

// HandleJoinLifecycle serves POST /factory-visualization/lifecycle/join.
func (a *Adapter) HandleJoinLifecycle(w http.ResponseWriter, r *http.Request) {
	response, err := a.JoinLifecycleHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeLifecycleRequestError(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, response, a.logger)
}

// HandleStopDrainLifecycle serves POST /factory-visualization/lifecycle/stop-drain.
func (a *Adapter) HandleStopDrainLifecycle(w http.ResponseWriter, r *http.Request) {
	response, err := a.StopDrainLifecycleHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeLifecycleRequestError(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, response, a.logger)
}

func (a *Adapter) writeLifecycleRequestError(w http.ResponseWriter, err error) {
	transporterrors.WriteRequestError(w, a.logger, err, "factory visualization lifecycle request failed")
}
