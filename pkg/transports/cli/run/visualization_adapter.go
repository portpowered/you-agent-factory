package run

import (
	"fmt"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
)

func visualizationCLIService(
	presentation factoryvisualization.ResponsePresentation,
) visualizationcli.Service {
	return visualizationcli.NewFromPresentation(presentation)
}

func runVisualizationSink(
	cfg RunConfig,
	presentation factoryvisualization.ResponsePresentation,
) factoryvisualization.Sink {
	service := visualizationCLIService(presentation)
	if service == nil {
		return nil
	}
	return service.BuildVisualizationSink(visualizationcli.SinkConfig{
		Output:                     cfg.Output,
		SuppressDashboardRendering: cfg.SuppressDashboardRendering,
	})
}

func invocationFactoryEventRenderer(
	cfg RunConfig,
	presentation factoryvisualization.ResponsePresentation,
) (visualizationcli.FactoryEventRenderer, error) {
	if !isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		return nil, nil
	}
	service := visualizationCLIService(presentation)
	if service == nil {
		return nil, fmt.Errorf("construct factory invocation: response presentation operation is required")
	}
	return service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               cfg.Output,
		JSON:                 cfg.JSONOutput,
		InvocationOutputMode: cfg.InvocationOutputMode,
	})
}
