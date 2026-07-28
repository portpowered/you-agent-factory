package factory_visualization_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestVisualizationActivatesThroughPublicRootAfterLifecycle proves explicit
// public Activate leaves the inert constructed state with a started lifecycle
// outcome, while Join before Activate remains a typed not-activated failure
// without subscription or presentation side effects.
func TestVisualizationActivatesThroughPublicRootAfterLifecycle(t *testing.T) {
	t.Parallel()

	tracker := &visualizationEffectTracker{}
	var (
		visualizationRoot factoryvisualization.Root
		rootOnce          sync.Once
	)
	edges := serviceedges.Edges{
		FactoryVisualizationSink: factoryvisualization.SinkFunc(tracker.PresentFactoryView),
		FactoryVisualizationRootObserver: func(root factoryvisualization.Root) {
			rootOnce.Do(func() {
				visualizationRoot = root
			})
		},
	}

	_ = support.BuildProcess(t, edges)

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

	if visualizationRoot == nil {
		t.Fatal("FactoryVisualizationRootObserver was not invoked, want composed public Root")
	}

	_, err := visualizationRoot.Join(context.Background(), factoryvisualization.JoinRequest{})
	requireLifecycleError(
		t,
		err,
		factoryvisualization.LifecycleErrorNotActivated,
		"Join before Activate",
	)
	if tracker.presentations() != 0 {
		t.Fatalf(
			"projected-view presentations before Activate = %d, want 0",
			tracker.presentations(),
		)
	}

	result, err := visualizationRoot.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate() error = %v, want started lifecycle outcome", err)
	}
	if result.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("Activate() state = %q, want %q", result.State, factoryvisualization.LifecycleStateStarted)
	}

	_, err = visualizationRoot.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	requireLifecycleError(
		t,
		err,
		factoryvisualization.LifecycleErrorAlreadyActivated,
		"Activate after started lifecycle",
	)

	if _, err := visualizationRoot.StopDrain(context.Background(), factoryvisualization.StopDrainRequest{}); err != nil {
		t.Fatalf("StopDrain() error = %v, want clean shutdown before server stop", err)
	}
}

func requireLifecycleError(
	t *testing.T,
	err error,
	kind factoryvisualization.LifecycleErrorKind,
	label string,
) {
	t.Helper()
	var lifeErr *factoryvisualization.LifecycleError
	if !errors.As(err, &lifeErr) || lifeErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}
