package cli_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
)

// TestPresentationSinkUsesVisualizationOwnedRuntimeFacts proves CUT-VIS-RUN story 003:
// the CLI visualization sink maps dashboard header facts from Visualization-owned
// RuntimeObservation fields instead of Petri-shaped RuntimeEngineStateSnapshot aliases.
func TestPresentationSinkUsesVisualizationOwnedRuntimeFacts(t *testing.T) {
	t.Parallel()

	service := newTestService()
	var output bytes.Buffer
	sink := service.BuildVisualizationSink(visualizationcli.SinkConfig{Output: &output})
	if sink == nil {
		t.Fatal("sink = nil, want dashboard sink")
	}

	sink.PresentFactoryView(factoryvisualization.View{
		Runtime: factoryvisualization.RuntimeObservation{
			TickCount:     9,
			FactoryState:  "RUNNING",
			RuntimeStatus: interfaces.RuntimeStatusActive,
			Uptime:        42 * time.Second,
		},
		ObservedAt: time.Unix(100, 0).UTC(),
	})

	got := output.String()
	for _, want := range []string{"Factory: RUNNING", "Runtime: ACTIVE", "Tick: 9", "42s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dashboard output = %q, want %q", got, want)
		}
	}
}

// TestPresentationSinkPackageDoesNotImportFactoryRuntime proves the leased CLI
// presentation sink no longer depends on Factory Runtime snapshot aliases.
