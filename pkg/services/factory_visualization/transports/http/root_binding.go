package http

import (
	"context"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/binding"
	"go.uber.org/zap"
)

// VisualizationRoot is the accepted Factory Visualization root contract used
// by the HTTP adapter. Adapter-owned operations invoke this surface rather than
// Visualization internal packages.
type VisualizationRoot = factoryvisualization.Root

// RootBinding binds the HTTP adapter to one injected Visualization root.
type RootBinding struct {
	Visualization VisualizationRoot
}

// NewHandlerFromRoot constructs an HTTP adapter that calls through the supplied
// Visualization root. Tests inject a focused fake implementing VisualizationRoot
// without constructing real Visualization subservice or Wire graphs.
func NewHandlerFromRoot(root RootBinding, logger *zap.Logger) *Adapter {
	return NewHandler(Dependencies{VisualizationRoot: root.Visualization}, logger)
}

func (a *Adapter) boundRoot() *binding.Handler {
	if a == nil {
		return nil
	}
	return a.rootBinding
}

// Activate invokes the Visualization root Activate slice.
func (a *Adapter) Activate(
	ctx context.Context,
	req factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	return a.boundRoot().Activate(ctx, req)
}

// Join invokes the Visualization root Join slice.
func (a *Adapter) Join(
	ctx context.Context,
	req factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	return a.boundRoot().Join(ctx, req)
}

// StopDrain invokes the Visualization root StopDrain slice.
func (a *Adapter) StopDrain(
	ctx context.Context,
	req factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	return a.boundRoot().StopDrain(ctx, req)
}

// Observe invokes the Visualization root Observe slice.
func (a *Adapter) Observe(
	ctx context.Context,
	req factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	return a.boundRoot().Observe(ctx, req)
}

// OpenPresentation invokes the Visualization root OpenPresentation slice.
func (a *Adapter) OpenPresentation(
	ctx context.Context,
	req factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	return a.boundRoot().OpenPresentation(ctx, req)
}

// PresentProgress invokes the Visualization root PresentProgress slice.
func (a *Adapter) PresentProgress(
	ctx context.Context,
	req factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	return a.boundRoot().PresentProgress(ctx, req)
}

// FinalizePresentation invokes the Visualization root FinalizePresentation
// slice.
func (a *Adapter) FinalizePresentation(
	ctx context.Context,
	req factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	return a.boundRoot().FinalizePresentation(ctx, req)
}

// ClosePresentation invokes the Visualization root ClosePresentation slice.
func (a *Adapter) ClosePresentation(
	ctx context.Context,
	req factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	return a.boundRoot().ClosePresentation(ctx, req)
}
