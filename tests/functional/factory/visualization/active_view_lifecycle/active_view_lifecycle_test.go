package active_view_lifecycle_test

import (
	"context"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestVisualizationActiveViewLifecycleThroughPublicProcess exercises one
// customer Factory Session through Process.Execute, then observes the
// explicitly activated retained-then-live Visualization through both its
// published root and its process-edge sink before stopping and joining it.
func TestVisualizationActiveViewLifecycleThroughPublicProcess(t *testing.T) {
	viewPresentations := make(chan factoryvisualization.View, 16)
	rootPublished := make(chan factoryvisualization.Root, 1)
	edges := serviceedges.Edges{
		FactoryVisualizationSink: factoryvisualization.SinkFunc(func(view factoryvisualization.View) {
			viewPresentations <- view
		}),
		FactoryVisualizationRootObserver: func(root factoryvisualization.Root) {
			rootPublished <- root
		},
	}

	dir := support.ScaffoldFactory(t, visualizationInertFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	var visualizationRoot factoryvisualization.Root
	select {
	case visualizationRoot = <-rootPublished:
	case <-time.After(5 * time.Second):
		t.Fatal("FactoryVisualizationRootObserver did not publish a root")
	}

	if _, err := visualizationRoot.Join(context.Background(), factoryvisualization.JoinRequest{}); err == nil {
		t.Fatal("Join before Activate returned nil error, want not-activated lifecycle failure")
	}
	if _, err := visualizationRoot.Observe(context.Background(), factoryvisualization.ObserveRequest{}); err == nil {
		t.Fatal("Observe with missing parameters returned nil error, want invalid-input projection failure")
	}
	select {
	case view := <-viewPresentations:
		t.Fatalf("Visualization presented a view before Activate: %#v", view)
	default:
	}

	activated, err := visualizationRoot.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate retained-then-live: %v", err)
	}
	if activated.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("Activate state = %q, want %q", activated.State, factoryvisualization.LifecycleStateStarted)
	}

	// The edge channel is the observable synchronization point; this bounded
	// timeout is only a guard against a hung process lifecycle.
	select {
	case view := <-viewPresentations:
		if view.ObservedAt.IsZero() {
			t.Fatal("sink view ObservedAt is zero")
		}
		if view.Runtime.TickCount < 0 || view.RetainedEventCount < 0 || view.RenderData.InFlightDispatchCount < 0 {
			t.Fatalf("sink view has invalid runtime/projection counts: %#v", view)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("activated Visualization did not present a projected view to its sink")
	}

	observed, err := visualizationRoot.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Observe retained-then-live: %v", err)
	}
	if observed.View.ObservedAt.IsZero() {
		t.Fatal("detached view ObservedAt is zero")
	}
	if observed.View.TickCount < 0 || observed.View.RetainedEventCount < 0 {
		t.Fatalf("detached view has invalid runtime/projection counts: %#v", observed.View)
	}

	stopped, err := visualizationRoot.StopDrain(context.Background(), factoryvisualization.StopDrainRequest{})
	if err != nil {
		t.Fatalf("StopDrain: %v", err)
	}
	if stopped.State != factoryvisualization.LifecycleStateStopped {
		t.Fatalf("StopDrain state = %q, want %q", stopped.State, factoryvisualization.LifecycleStateStopped)
	}
	select {
	case finalView := <-viewPresentations:
		if finalView.ObservedAt.IsZero() {
			t.Fatal("stop/drain final sink view ObservedAt is zero")
		}
		if finalView.Runtime.TickCount < 0 || finalView.RetainedEventCount < 0 || finalView.RenderData.InFlightDispatchCount < 0 {
			t.Fatalf("stop/drain final sink view has invalid runtime/projection counts: %#v", finalView)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StopDrain did not present a final projected view to its sink")
	}
	if _, err := visualizationRoot.Join(context.Background(), factoryvisualization.JoinRequest{}); err != nil {
		t.Fatalf("Join after StopDrain: %v", err)
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
