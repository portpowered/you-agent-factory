package cli_test

import (
	"bytes"
	"os/exec"
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
func TestPresentationSinkPackageDoesNotImportFactoryRuntime(t *testing.T) {
	t.Parallel()

	const packagePath = "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == "github.com/portpowered/infinite-you/pkg/services/factory_runtime" {
			t.Fatalf("%s must not import Factory Runtime; use Visualization-owned RuntimeObservation facts", packagePath)
		}
	}
}
