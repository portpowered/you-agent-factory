package factory_visualization_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type rootObserveTracker struct {
	factoryvisualization.Root
	calls atomic.Int32
}

func (tracker *rootObserveTracker) Observe(
	ctx context.Context,
	req factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	tracker.calls.Add(1)
	return tracker.Root.Observe(ctx, req)
}

func (tracker *rootObserveTracker) observeCalls() int {
	return int(tracker.calls.Load())
}

// TestVisualizationObserveThroughPublicRootAfterLifecycle proves public Observe
// is not performed as a side effect of root.BuildProcess construction alone and
// returns observable projected-view facts through the published Root once the
// composed runtime snapshot is available. Visualization activation is not
// required for detached Observe on the published contract exercised here.
func TestVisualizationObserveThroughPublicRootAfterLifecycle(t *testing.T) {
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
				visualizationRoot = &rootObserveTracker{Root: root}
			})
		},
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

	observeTracker, ok := visualizationRoot.(*rootObserveTracker)
	if !ok || observeTracker == nil {
		t.Fatal("FactoryVisualizationRootObserver was not invoked, want composed public Root")
	}
	if observeTracker.observeCalls() != 0 {
		t.Fatalf(
			"Observe calls after runtime host startup = %d, want 0 before explicit Observe",
			observeTracker.observeCalls(),
		)
	}

	_, err := observeTracker.Observe(context.Background(), factoryvisualization.ObserveRequest{})
	requireProjectionError(
		t,
		err,
		factoryvisualization.ProjectionErrorInvalidInput,
		"Observe with missing parameters",
	)
	if observeTracker.observeCalls() != 1 {
		t.Fatalf(
			"Observe calls after invalid Observe request = %d, want 1 explicit call",
			observeTracker.observeCalls(),
		)
	}

	observed, err := observeTracker.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Observe on running runtime before Visualization Activate: error = %v", err)
	}
	if observed.View.ObservedAt.IsZero() {
		t.Fatal("Observe: ObservedAt is zero, want observable projected-view timestamp")
	}
	if observed.View.TickCount < 0 {
		t.Fatalf("Observe: TickCount = %d, want non-negative runtime fact", observed.View.TickCount)
	}
	if observed.View.RetainedEventCount < 0 {
		t.Fatalf(
			"Observe: RetainedEventCount = %d, want non-negative retained-event fact",
			observed.View.RetainedEventCount,
		)
	}
	if observeTracker.observeCalls() != 2 {
		t.Fatalf(
			"Observe calls after successful Observe = %d, want 2 explicit calls",
			observeTracker.observeCalls(),
		)
	}
}

func requireProjectionError(
	t *testing.T,
	err error,
	kind factoryvisualization.ProjectionErrorKind,
	label string,
) {
	t.Helper()
	var projErr *factoryvisualization.ProjectionError
	if !errors.As(err, &projErr) || projErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}
