package http

import (
	"context"
	"io"
	"net/http"
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
	result, err := a.Observe(ctx, req)
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
	a.writeJSON(w, http.StatusOK, response)
}

func (a *Adapter) writeObserveRequestError(w http.ResponseWriter, err error) {
	a.writeVisualizationRequestError(w, err, "factory visualization observe request failed")
}
