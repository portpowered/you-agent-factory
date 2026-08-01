package binding

import (
	"context"
	"errors"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// Root is the peer-facing Factory Visualization contract accepted by the HTTP
// transport binding.
type Root = factoryvisualization.Root

// Handler binds transport operations to one injected Visualization root. It
// contains no HTTP representation policy; those concerns remain in owner
// operation adapters.
type Handler struct {
	visualization Root
}

// New constructs a root binding for an already-selected Visualization root.
func New(visualization Root) *Handler {
	return &Handler{visualization: visualization}
}

// Require returns the bound root or the stable unavailable error used by the
// existing HTTP adapter contract.
func (h *Handler) Require() (Root, error) {
	if h == nil || h.visualization == nil {
		return nil, errors.New("Factory visualization API is unavailable")
	}
	return h.visualization, nil
}

// Activate invokes the Visualization root Activate slice.
func (h *Handler) Activate(
	ctx context.Context,
	req factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	root, err := h.Require()
	if err != nil {
		return factoryvisualization.ActivateResult{}, err
	}
	return root.Activate(ctx, req)
}

// Join invokes the Visualization root Join slice.
func (h *Handler) Join(
	ctx context.Context,
	req factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	root, err := h.Require()
	if err != nil {
		return factoryvisualization.JoinResult{}, err
	}
	return root.Join(ctx, req)
}

// StopDrain invokes the Visualization root StopDrain slice.
func (h *Handler) StopDrain(
	ctx context.Context,
	req factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	root, err := h.Require()
	if err != nil {
		return factoryvisualization.StopDrainResult{}, err
	}
	return root.StopDrain(ctx, req)
}

// Observe invokes the Visualization root Observe slice.
func (h *Handler) Observe(
	ctx context.Context,
	req factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	root, err := h.Require()
	if err != nil {
		return factoryvisualization.ObserveResult{}, err
	}
	return root.Observe(ctx, req)
}

// OpenPresentation invokes the Visualization root OpenPresentation slice.
func (h *Handler) OpenPresentation(
	ctx context.Context,
	req factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	root, err := h.Require()
	if err != nil {
		return factoryvisualization.OpenPresentationResult{}, err
	}
	return root.OpenPresentation(ctx, req)
}

// PresentProgress invokes the Visualization root PresentProgress slice.
func (h *Handler) PresentProgress(
	ctx context.Context,
	req factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	root, err := h.Require()
	if err != nil {
		return factoryvisualization.PresentProgressResult{}, err
	}
	return root.PresentProgress(ctx, req)
}

// FinalizePresentation invokes the Visualization root FinalizePresentation slice.
func (h *Handler) FinalizePresentation(
	ctx context.Context,
	req factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	root, err := h.Require()
	if err != nil {
		return factoryvisualization.FinalizePresentationResult{}, err
	}
	return root.FinalizePresentation(ctx, req)
}

// ClosePresentation invokes the Visualization root ClosePresentation slice.
func (h *Handler) ClosePresentation(
	ctx context.Context,
	req factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	root, err := h.Require()
	if err != nil {
		return factoryvisualization.ClosePresentationResult{}, err
	}
	return root.ClosePresentation(ctx, req)
}
