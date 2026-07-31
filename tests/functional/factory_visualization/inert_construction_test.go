package factory_visualization_test

import (
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type visualizationEffectTracker struct {
	presentCount atomic.Int32
}

func (tracker *visualizationEffectTracker) PresentFactoryView(factoryvisualization.View) {
	tracker.presentCount.Add(1)
}

func (tracker *visualizationEffectTracker) presentations() int {
	return int(tracker.presentCount.Load())
}

// TestVisualizationRemainsInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Factory Visualization without starting event
// subscriptions, projected-view presentations, or presentation-session opens
// before explicit public activation.
func TestVisualizationRemainsInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	tracker := &visualizationEffectTracker{}
	edges := serviceedges.Edges{
		FactoryVisualizationSink: factoryvisualization.SinkFunc(tracker.PresentFactoryView),
	}

	process := support.BuildProcess(t, edges)
	if process == nil {
		t.Fatal("BuildProcess() returned nil process, want inert composition")
	}
	if tracker.presentations() != 0 {
		t.Fatalf(
			"projected-view presentations during BuildProcess = %d, want 0",
			tracker.presentations(),
		)
	}

	dir := support.ScaffoldFactory(t, visualizationInertFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	support.WaitForStatus(t, server.URL(), 5*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})

	if tracker.presentations() != 0 {
		t.Fatalf(
			"projected-view presentations after runtime host startup = %d, want 0 before Activate",
			tracker.presentations(),
		)
	}
}

func visualizationInertFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
