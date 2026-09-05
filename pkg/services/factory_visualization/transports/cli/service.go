// Package cli defines the Factory Visualization service-owned CLI adapter.
package cli

import (
	"context"
	"fmt"
	"io"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/transports/cli/dashboard"
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
	root         factoryvisualization.Root
	presentation factoryvisualization.ResponsePresentation
}

// SinkConfig carries CLI inputs for live dashboard view sink construction.
type SinkConfig struct {
	Output                     io.Writer
	SuppressDashboardRendering bool
}

// BuildVisualizationSink returns a Visualization sink that renders the simple
// dashboard to Output, or nil when dashboard rendering is suppressed or Output
// is missing (accepted no-output outcome).
func (s *service) BuildVisualizationSink(cfg SinkConfig) factoryvisualization.Sink {
	if cfg.SuppressDashboardRendering || cfg.Output == nil {
		return nil
	}
	output := cfg.Output
	return factoryvisualization.SinkFunc(func(input factoryvisualization.View) {
		renderSimpleDashboard(output, input)
	})
}

func renderSimpleDashboard(output io.Writer, input factoryvisualization.View) {
	fmt.Fprint(output, dashboard.FormatSimpleDashboardWithRenderData(
		dashboard.SimpleDashboardHeader{
			TickCount:     input.Runtime.TickCount,
			FactoryState:  input.Runtime.FactoryState,
			RuntimeStatus: input.Runtime.RuntimeStatus,
			Uptime:        input.Runtime.Uptime,
		},
		input.RenderData,
		input.ObservedAt,
	))
}

// New constructs the Visualization CLI adapter from the accepted Visualization
// root and response-presentation collaborator.
func New(
	root factoryvisualization.Root,
	presentation factoryvisualization.ResponsePresentation,
) Service {
	if presentation == nil {
		return nil
	}
	return &service{root: root, presentation: presentation}
}

// NewFromPresentation constructs the Visualization CLI adapter for run
// composition paths that receive ResponsePresentation without a live Root.
// Presentation-session methods return an error until a root is supplied.
func NewFromPresentation(presentation factoryvisualization.ResponsePresentation) Service {
	return New(nil, presentation)
}

func (s *service) OpenPresentationSession(
	ctx context.Context,
	req factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	root, err := s.requireRoot()
	if err != nil {
		return factoryvisualization.OpenPresentationResult{}, err
	}
	return root.OpenPresentation(ctx, req)
}

func (s *service) PresentPresentationProgress(
	ctx context.Context,
	req factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	root, err := s.requireRoot()
	if err != nil {
		return factoryvisualization.PresentProgressResult{}, err
	}
	return root.PresentProgress(ctx, req)
}

func (s *service) FinalizePresentationSession(
	ctx context.Context,
	req factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	root, err := s.requireRoot()
	if err != nil {
		return factoryvisualization.FinalizePresentationResult{}, err
	}
	return root.FinalizePresentation(ctx, req)
}

func (s *service) ClosePresentationSession(
	ctx context.Context,
	req factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	root, err := s.requireRoot()
	if err != nil {
		return factoryvisualization.ClosePresentationResult{}, err
	}
	return root.ClosePresentation(ctx, req)
}

func (s *service) requireRoot() (factoryvisualization.Root, error) {
	if s.root == nil {
		return nil, fmt.Errorf("visualization root is required")
	}
	return s.root, nil
}
