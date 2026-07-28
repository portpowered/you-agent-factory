package http

import (
	"context"
	"errors"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"go.uber.org/zap"
)

// VisualizationRoot is the accepted Factory Visualization root contract used by
// the HTTP adapter. Adapter-owned operations invoke this surface rather than
// Visualization internal packages.
type VisualizationRoot = factoryvisualization.Root

// RootBinding binds the HTTP adapter to one injected Visualization root.
type RootBinding struct {
	Visualization VisualizationRoot
}

// NewHandlerFromRoot constructs an HTTP adapter that calls through the supplied
// Visualization root. Tests inject a focused fake implementing VisualizationRoot
// without constructing real activation_lifecycle, live_view_projection,
// response_event_presentation, or service-local Wire graphs.
func NewHandlerFromRoot(binding RootBinding, logger *zap.Logger) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return NewHandler(Dependencies{
		VisualizationRoot: binding.Visualization,
	}, logger)
}

func (a *Adapter) requireVisualizationRoot() (VisualizationRoot, error) {
	if a == nil || a.visualization == nil {
		return nil, errors.New("Factory visualization API is unavailable")
	}
	return a.visualization, nil
}

// Activate invokes the Visualization root Activate slice. HTTP decode and encode
// for lifecycle operations arrive in later adapter stories.
func (a *Adapter) Activate(
	ctx context.Context,
	req factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	root, err := a.requireVisualizationRoot()
	if err != nil {
		return factoryvisualization.ActivateResult{}, err
	}
	return root.Activate(ctx, req)
}
