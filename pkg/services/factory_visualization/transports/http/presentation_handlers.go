package http

import (
	"context"
	"io"
	"net/http"
)

// OpenPresentationHTTP decodes an owned presentation-open HTTP request, invokes
// the Visualization root OpenPresentation slice, and encodes the success shape.
func (a *Adapter) OpenPresentationHTTP(
	ctx context.Context,
	body io.Reader,
) (OpenPresentationHTTPResponse, error) {
	req, err := decodeOpenPresentationHTTPRequest(body)
	if err != nil {
		return OpenPresentationHTTPResponse{}, err
	}
	if err := visualizationContextBeforeRoot(ctx); err != nil {
		return OpenPresentationHTTPResponse{}, err
	}
	result, err := a.OpenPresentation(ctx, req)
	if err != nil {
		return OpenPresentationHTTPResponse{}, err
	}
	return openPresentationHTTPResponseFromResult(result), nil
}

// PresentProgressHTTP decodes an owned presentation-progress HTTP request,
// invokes the Visualization root PresentProgress slice, and encodes the success
// shape.
func (a *Adapter) PresentProgressHTTP(
	ctx context.Context,
	body io.Reader,
) (PresentProgressHTTPResponse, error) {
	req, err := decodePresentProgressHTTPRequest(body)
	if err != nil {
		return PresentProgressHTTPResponse{}, err
	}
	if err := visualizationContextBeforeRoot(ctx); err != nil {
		return PresentProgressHTTPResponse{}, err
	}
	result, err := a.PresentProgress(ctx, req)
	if err != nil {
		return PresentProgressHTTPResponse{}, err
	}
	return presentProgressHTTPResponseFromResult(result), nil
}

// FinalizePresentationHTTP decodes an owned presentation-finalize HTTP request,
// invokes the Visualization root FinalizePresentation slice, and encodes the
// success shape.
func (a *Adapter) FinalizePresentationHTTP(
	ctx context.Context,
	body io.Reader,
) (FinalizePresentationHTTPResponse, error) {
	req, err := decodeFinalizePresentationHTTPRequest(body)
	if err != nil {
		return FinalizePresentationHTTPResponse{}, err
	}
	if err := visualizationContextBeforeRoot(ctx); err != nil {
		return FinalizePresentationHTTPResponse{}, err
	}
	result, err := a.FinalizePresentation(ctx, req)
	if err != nil {
		return FinalizePresentationHTTPResponse{}, err
	}
	return finalizePresentationHTTPResponseFromResult(result), nil
}

// ClosePresentationHTTP decodes an owned presentation-close HTTP request,
// invokes the Visualization root ClosePresentation slice, and encodes the
// success shape.
func (a *Adapter) ClosePresentationHTTP(
	ctx context.Context,
	body io.Reader,
) (ClosePresentationHTTPResponse, error) {
	req, err := decodeClosePresentationHTTPRequest(body)
	if err != nil {
		return ClosePresentationHTTPResponse{}, err
	}
	if err := visualizationContextBeforeRoot(ctx); err != nil {
		return ClosePresentationHTTPResponse{}, err
	}
	result, err := a.ClosePresentation(ctx, req)
	if err != nil {
		return ClosePresentationHTTPResponse{}, err
	}
	return closePresentationHTTPResponseFromResult(result), nil
}

// HandleOpenPresentation serves POST /factory-visualization/presentation/open.
func (a *Adapter) HandleOpenPresentation(w http.ResponseWriter, r *http.Request) {
	response, err := a.OpenPresentationHTTP(r.Context(), r.Body)
	if err != nil {
		a.writePresentationRequestError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// HandlePresentProgress serves POST /factory-visualization/presentation/progress.
func (a *Adapter) HandlePresentProgress(w http.ResponseWriter, r *http.Request) {
	response, err := a.PresentProgressHTTP(r.Context(), r.Body)
	if err != nil {
		a.writePresentationRequestError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// HandleFinalizePresentation serves POST /factory-visualization/presentation/finalize.
func (a *Adapter) HandleFinalizePresentation(w http.ResponseWriter, r *http.Request) {
	response, err := a.FinalizePresentationHTTP(r.Context(), r.Body)
	if err != nil {
		a.writePresentationRequestError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// HandleClosePresentation serves POST /factory-visualization/presentation/close.
func (a *Adapter) HandleClosePresentation(w http.ResponseWriter, r *http.Request) {
	response, err := a.ClosePresentationHTTP(r.Context(), r.Body)
	if err != nil {
		a.writePresentationRequestError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

func (a *Adapter) writePresentationRequestError(w http.ResponseWriter, err error) {
	a.writeVisualizationRequestError(w, err, "factory visualization presentation request failed")
}
