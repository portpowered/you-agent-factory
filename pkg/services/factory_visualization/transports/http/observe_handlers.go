package http

import (
	"context"
	"errors"
	"io"
	"net/http"

	"go.uber.org/zap"
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
	if message, ok := requestFieldValidationMessage(err); ok {
		a.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
		return
	}
	var decodeErr observeHTTPDecodeError
	if errors.As(err, &decodeErr) {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	a.logger.Error("factory visualization observe request failed", zap.Error(err))
	a.writeError(w, http.StatusInternalServerError, "factory visualization observe request failed", "INTERNAL_ERROR")
}
