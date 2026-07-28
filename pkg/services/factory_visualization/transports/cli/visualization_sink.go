package cli

import (
	"fmt"
	"io"

	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/transports/cli/dashboard"
)

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
		dashboardEngineSnapshotHeader(input.Runtime),
		input.RenderData,
		input.ObservedAt,
	))
}

func dashboardEngineSnapshotHeader(
	runtime factoryvisualization.RuntimeObservation,
) state.RuntimeEngineStateSnapshot {
	return state.RuntimeEngineStateSnapshot{
		TickCount:     runtime.TickCount,
		FactoryState:  runtime.FactoryState,
		RuntimeStatus: runtime.RuntimeStatus,
		Uptime:        runtime.Uptime,
	}
}
