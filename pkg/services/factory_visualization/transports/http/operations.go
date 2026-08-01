package http

import (
	"context"
	"io"
	"net/http"
)

// ActivateLifecycleHTTP forwards to the focused lifecycle adapter.
func (a *Adapter) ActivateLifecycleHTTP(ctx context.Context, body io.Reader) (LifecycleHTTPResponse, error) {
	return a.lifecycle.ActivateLifecycleHTTP(ctx, body)
}

// JoinLifecycleHTTP forwards to the focused lifecycle adapter.
func (a *Adapter) JoinLifecycleHTTP(ctx context.Context, body io.Reader) (LifecycleHTTPResponse, error) {
	return a.lifecycle.JoinLifecycleHTTP(ctx, body)
}

// StopDrainLifecycleHTTP forwards to the focused lifecycle adapter.
func (a *Adapter) StopDrainLifecycleHTTP(ctx context.Context, body io.Reader) (LifecycleHTTPResponse, error) {
	return a.lifecycle.StopDrainLifecycleHTTP(ctx, body)
}

// HandleActivateLifecycle forwards the raw HTTP request to lifecycle ownership.
func (a *Adapter) HandleActivateLifecycle(w http.ResponseWriter, r *http.Request) {
	a.lifecycle.HandleActivateLifecycle(w, r)
}

// HandleJoinLifecycle forwards the raw HTTP request to lifecycle ownership.
func (a *Adapter) HandleJoinLifecycle(w http.ResponseWriter, r *http.Request) {
	a.lifecycle.HandleJoinLifecycle(w, r)
}

// HandleStopDrainLifecycle forwards the raw HTTP request to lifecycle ownership.
func (a *Adapter) HandleStopDrainLifecycle(w http.ResponseWriter, r *http.Request) {
	a.lifecycle.HandleStopDrainLifecycle(w, r)
}

// ObserveHTTP forwards to the focused observation adapter.
func (a *Adapter) ObserveHTTP(ctx context.Context, body io.Reader) (ObserveHTTPResponse, error) {
	return a.observe.ObserveHTTP(ctx, body)
}

// HandleObserve forwards the raw HTTP request to observation ownership.
func (a *Adapter) HandleObserve(w http.ResponseWriter, r *http.Request) {
	a.observe.HandleObserve(w, r)
}

// OpenPresentationHTTP forwards to the focused presentation adapter.
func (a *Adapter) OpenPresentationHTTP(ctx context.Context, body io.Reader) (OpenPresentationHTTPResponse, error) {
	return a.presentation.OpenPresentationHTTP(ctx, body)
}

// PresentProgressHTTP forwards to the focused presentation adapter.
func (a *Adapter) PresentProgressHTTP(ctx context.Context, body io.Reader) (PresentProgressHTTPResponse, error) {
	return a.presentation.PresentProgressHTTP(ctx, body)
}

// FinalizePresentationHTTP forwards to the focused presentation adapter.
func (a *Adapter) FinalizePresentationHTTP(ctx context.Context, body io.Reader) (FinalizePresentationHTTPResponse, error) {
	return a.presentation.FinalizePresentationHTTP(ctx, body)
}

// ClosePresentationHTTP forwards to the focused presentation adapter.
func (a *Adapter) ClosePresentationHTTP(ctx context.Context, body io.Reader) (ClosePresentationHTTPResponse, error) {
	return a.presentation.ClosePresentationHTTP(ctx, body)
}

// HandleOpenPresentation forwards the raw HTTP request to presentation ownership.
func (a *Adapter) HandleOpenPresentation(w http.ResponseWriter, r *http.Request) {
	a.presentation.HandleOpenPresentation(w, r)
}

// HandlePresentProgress forwards the raw HTTP request to presentation ownership.
func (a *Adapter) HandlePresentProgress(w http.ResponseWriter, r *http.Request) {
	a.presentation.HandlePresentProgress(w, r)
}

// HandleFinalizePresentation forwards the raw HTTP request to presentation ownership.
func (a *Adapter) HandleFinalizePresentation(w http.ResponseWriter, r *http.Request) {
	a.presentation.HandleFinalizePresentation(w, r)
}

// HandleClosePresentation forwards the raw HTTP request to presentation ownership.
func (a *Adapter) HandleClosePresentation(w http.ResponseWriter, r *http.Request) {
	a.presentation.HandleClosePresentation(w, r)
}
