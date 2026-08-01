package observe

import (
	"context"
	"io"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/common"
	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/errors"
)

// ObserveHTTP decodes an owned Observe HTTP request, invokes the Visualization
// root Observe slice, and encodes the projected-view success shape.
func (a *Adapter) ObserveHTTP(
	ctx context.Context,
	body io.Reader,
) (ObserveHTTPResponse, error) {
	req, err := decodeObserveHTTPRequest(body)
	if err != nil {
		return ObserveHTTPResponse{}, err
	}
	if err := common.ContextBeforeRoot(ctx); err != nil {
		return ObserveHTTPResponse{}, err
	}
	root, err := a.root()
	if err != nil {
		return ObserveHTTPResponse{}, err
	}
	result, err := root.Observe(ctx, req)
	if err != nil {
		return ObserveHTTPResponse{}, err
	}
	return observeHTTPResponseFromResult(result), nil
}

// HandleObserve serves POST /factory-visualization/observe.
func (a *Adapter) HandleObserve(w http.ResponseWriter, r *http.Request) {
	response, err := a.ObserveHTTP(r.Context(), r.Body)
	if err != nil {
		a.writeObserveRequestError(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, response, a.logger)
}

func (a *Adapter) writeObserveRequestError(w http.ResponseWriter, err error) {
	transporterrors.WriteRequestError(w, a.logger, err, "factory visualization observe request failed")
}
