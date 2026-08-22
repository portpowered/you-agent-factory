package replay_contracts_test

import (
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestComposedVisualizationObserveUsesRecordingsProjection proves the public
// Factory Visualization read path requests dashboard projection data from the
// Recordings projection service after root.BuildProcess composition and
// lifecycle activation.
func TestComposedVisualizationObserveUsesRecordingsProjection(t *testing.T) {
	var (
		visualizationRoot factoryvisualization.Root
		rootOnce          sync.Once
	)
	edges := serviceedges.Edges{
		FactoryVisualizationSink: factoryvisualization.SinkFunc(func(factoryvisualization.View) {}),
		FactoryVisualizationRootObserver: func(root factoryvisualization.Root) {
			rootOnce.Do(func() { visualizationRoot = root })
		},
	}
	dir := support.ScaffoldFactory(t, replayContractFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	t.Cleanup(func() { server.Stop(t) })

	support.WaitForStatus(t, server.URL(), 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	if visualizationRoot == nil {
		t.Fatal("FactoryVisualizationRootObserver was not invoked")
	}
	activated, err := visualizationRoot.Activate(t.Context(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Factory Visualization Activate: %v", err)
	}
	if activated.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("Factory Visualization Activate state = %q, want %q", activated.State, factoryvisualization.LifecycleStateStarted)
	}
	observed, err := visualizationRoot.Observe(t.Context(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Factory Visualization Observe: %v", err)
	}
	if observed.View.ObservedAt.IsZero() {
		t.Fatal("Factory Visualization Observe returned zero timestamp")
	}
	if observed.View.TickCount < 0 || observed.View.RetainedEventCount < 0 {
		t.Fatalf("Factory Visualization Observe view = %#v, want non-negative counters", observed.View)
	}
	afterSequence := 0
	if _, err := visualizationRoot.Observe(t.Context(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
		Reconnect: &factoryvisualization.ObserveReconnectCursor{
			AfterSequence: &afterSequence,
		},
	}); err != nil {
		t.Fatalf("Factory Visualization reconnect Observe: %v", err)
	}
	status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtime status after projection")
	}
	if _, err := visualizationRoot.StopDrain(t.Context(), factoryvisualization.StopDrainRequest{}); err != nil {
		t.Fatalf("Factory Visualization StopDrain: %v", err)
	}
}
