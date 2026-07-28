// Package cli defines the Factory Visualization service-owned CLI adapter.
package cli

import (
	"context"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// InvocationOutputResponseStream enables live Factory Event stream presentation.
const InvocationOutputResponseStream = "response-stream"

// Service exposes Visualization-owned CLI presentation surfaces to composition.
type Service interface {
	BuildVisualizationSink(SinkConfig) factoryvisualization.Sink
	OpenFactoryEventRenderer(FactoryEventRendererConfig) (FactoryEventRenderer, error)
	OpenPresentationSession(
		context.Context,
		factoryvisualization.OpenPresentationRequest,
	) (factoryvisualization.OpenPresentationResult, error)
	PresentPresentationProgress(
		context.Context,
		factoryvisualization.PresentProgressRequest,
	) (factoryvisualization.PresentProgressResult, error)
	FinalizePresentationSession(
		context.Context,
		factoryvisualization.FinalizePresentationRequest,
	) (factoryvisualization.FinalizePresentationResult, error)
	ClosePresentationSession(
		context.Context,
		factoryvisualization.ClosePresentationRequest,
	) (factoryvisualization.ClosePresentationResult, error)
}

type service struct {
	root           factoryvisualization.Root
	presentation   factoryvisualization.ResponsePresentation
}

// New constructs the Visualization CLI adapter from the accepted Visualization
// root and response-presentation collaborator.
func New(
	root factoryvisualization.Root,
	presentation factoryvisualization.ResponsePresentation,
) Service {
	if root == nil || presentation == nil {
		return nil
	}
	return &service{root: root, presentation: presentation}
}

func (s *service) OpenPresentationSession(
	ctx context.Context,
	req factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	return s.root.OpenPresentation(ctx, req)
}

func (s *service) PresentPresentationProgress(
	ctx context.Context,
	req factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	return s.root.PresentProgress(ctx, req)
}

func (s *service) FinalizePresentationSession(
	ctx context.Context,
	req factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	return s.root.FinalizePresentation(ctx, req)
}

func (s *service) ClosePresentationSession(
	ctx context.Context,
	req factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	return s.root.ClosePresentation(ctx, req)
}
