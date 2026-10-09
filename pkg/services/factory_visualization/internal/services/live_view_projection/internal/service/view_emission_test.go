package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	projectionservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestViewEmissionShapeIsVisualizationOwned(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"StateSnapshot",
		"FactoryEvent",
		"FactoryWorldState",
	}
	typ := reflect.TypeOf(liveviewprojection.View{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		typeName := field.Type.String()
		for _, needle := range forbidden {
			if strings.Contains(typeName, needle) {
				t.Fatalf("View.%s has forbidden type %s", field.Name, typeName)
			}
		}
	}
}

func TestLiveViewProjectionContractDoesNotImportRuntimeSnapshot(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"pkg/services/factory_runtime",
		"StateSnapshot",
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "service.go"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	for _, needle := range forbidden {
		if strings.Contains(string(source), needle) {
			t.Fatalf("live_view_projection contract contains forbidden reference %q", needle)
		}
	}
}

func TestStartEmitsSanitizedVisualizationOwnedView(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 27, 23, 0, 0, 0, time.UTC)
	live := make(chan factorydefinitions.FactoryEvent)
	retained := []factorydefinitions.FactoryEvent{event("retained", 1)}
	renderData := recordings.SimpleDashboardRenderData{InFlightDispatchCount: 2}
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: retained,
			Events:  live,
		},
		snapshot: &liveviewprojection.RuntimeSnapshotFacts{
			RuntimeObservation: liveviewprojection.RuntimeObservation{
				TickCount:     5,
				FactoryState:  "running",
				RuntimeStatus: factorydefinitions.RuntimeStatusActive,
				Uptime:        3 * time.Second,
			},
		},
	}
	projections := newProjectionStubWithDashboard(renderData)
	rendered := make(chan liveviewprojection.View, 1)
	svc, err := projectionservice.New(nil,
		source,
		projections,
		fixedClock{now: now},
		liveviewprojection.SinkFunc(func(view liveviewprojection.View) { rendered <- view }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	view := <-rendered
	if !view.ObservedAt.Equal(now) {
		t.Fatalf("ObservedAt = %v, want %v", view.ObservedAt, now)
	}
	if view.RetainedEventCount != 1 {
		t.Fatalf("RetainedEventCount = %d, want 1", view.RetainedEventCount)
	}
	if view.Runtime.TickCount != 5 || view.Runtime.FactoryState != "running" ||
		view.Runtime.RuntimeStatus != factorydefinitions.RuntimeStatusActive ||
		view.Runtime.Uptime != 3*time.Second {
		t.Fatalf("Runtime facts = %#v, want sanitized observation", view.Runtime)
	}
	if view.RenderData.InFlightDispatchCount != renderData.InFlightDispatchCount {
		t.Fatalf("RenderData = %#v, want recordings projection facts", view.RenderData)
	}
}

func TestObserveSnapshotUnavailableDoesNotEmitView(t *testing.T) {
	t.Parallel()

	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: []factorydefinitions.FactoryEvent{event("retained", 1)},
			Events:  make(chan factorydefinitions.FactoryEvent),
		},
		snapshotErr: errors.New("snapshot unavailable"),
	}
	presented := make(chan liveviewprojection.View, 1)
	svc, err := projectionservice.New(nil,
		source,
		newProjectionStub(),
		fixedClock{now: time.Unix(1, 0)},
		liveviewprojection.SinkFunc(func(view liveviewprojection.View) { presented <- view }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case view := <-presented:
		t.Fatalf("view emitted after snapshot failure = %#v", view)
	default:
	}

	_, err = svc.Observe(context.Background(), liveviewprojection.ObserveRequest{
		Mode: liveviewprojection.ObserveModeRetainedThenLive,
	})
	var projErr *liveviewprojection.ProjectionError
	if !errors.As(err, &projErr) ||
		projErr.Kind != liveviewprojection.ProjectionErrorSnapshotUnavailable {
		t.Fatalf("Observe() error = %v, want snapshot unavailable", err)
	}
}

func TestObserveReconstructionFailedDoesNotReturnSuccessView(t *testing.T) {
	t.Parallel()

	reconstructErr := errors.New("reconstruction failed")
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: []factorydefinitions.FactoryEvent{event("retained", 1)},
			Events:  make(chan factorydefinitions.FactoryEvent),
		},
		snapshot: snapshotFacts(3),
	}
	projections := newFailingProjectionStub(reconstructErr)
	svc, err := projectionservice.New(nil,
		source,
		projections,
		fixedClock{now: time.Unix(1, 0)},
		liveviewprojection.SinkFunc(func(liveviewprojection.View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	_, err = svc.Observe(context.Background(), liveviewprojection.ObserveRequest{
		Mode: liveviewprojection.ObserveModeRetainedThenLive,
	})
	var projErr *liveviewprojection.ProjectionError
	if !errors.As(err, &projErr) ||
		projErr.Kind != liveviewprojection.ProjectionErrorReconstructionFailed {
		t.Fatalf("Observe() error = %v, want reconstruction failed", err)
	}
}

func TestProjectionFailureDrainsSelectedSubscriptionAndLeavesPeerPresenting(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"snapshot", "projection"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("controlled " + failure + " failure")
			selectedEvents := make(chan factorydefinitions.FactoryEvent)
			selectedSource := &sourceStub{
				stream:   &factorydefinitions.FactoryEventStream{Events: selectedEvents},
				snapshot: snapshotFacts(3),
			}
			projections := newProjectionStub()
			kind := liveviewprojection.ProjectionErrorReconstructionFailed
			if failure == "snapshot" {
				selectedSource.snapshotErr = cause
				kind = liveviewprojection.ProjectionErrorSnapshotUnavailable
			} else {
				projections = newFailingProjectionStub(cause)
			}
			errorsReported := make(chan error, 3)
			selectedViews := make(chan liveviewprojection.View, 1)
			selected, err := projectionservice.New(nil, selectedSource, projections, fixedClock{now: time.Unix(1, 0)},
				liveviewprojection.SinkFunc(func(view liveviewprojection.View) { selectedViews <- view }),
				func(err error) { errorsReported <- err })
			if err != nil {
				t.Fatal(err)
			}
			peerEvents := make(chan factorydefinitions.FactoryEvent)
			peerViews := make(chan liveviewprojection.View, 3)
			peer, err := projectionservice.New(nil, &sourceStub{
				stream: &factorydefinitions.FactoryEventStream{Events: peerEvents}, snapshot: snapshotFacts(8),
			}, newProjectionStub(), fixedClock{now: time.Unix(2, 0)},
				liveviewprojection.SinkFunc(func(view liveviewprojection.View) { peerViews <- view }), nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			t.Cleanup(func() { _ = selected.Stop(ctx); _ = peer.Stop(ctx) })
			if err := selected.Start(ctx); err != nil {
				t.Fatal(err)
			}
			if err := peer.Start(ctx); err != nil {
				t.Fatal(err)
			}
			<-peerViews // Initial peer view proves readiness before selected cleanup.
			if reported := <-errorsReported; !errors.Is(reported, cause) {
				t.Fatalf("reported error = %v, want controlled failure", reported)
			}
			result, err := selected.Observe(ctx, liveviewprojection.ObserveRequest{Mode: liveviewprojection.ObserveModeRetainedThenLive})
			var projectionErr *liveviewprojection.ProjectionError
			if !errors.As(err, &projectionErr) || projectionErr.Kind != kind || !errors.Is(err, cause) || result.View.TickCount != 0 {
				t.Fatalf("Observe = %+v, %v, want typed failure without success view", result, err)
			}
			assertFailedProjectionDrainsAlongsidePeer(t, ctx, selected, selectedViews, peerEvents, peerViews)
		})
	}
}

func assertFailedProjectionDrainsAlongsidePeer(t *testing.T, ctx context.Context, selected liveviewprojection.Service,
	selectedViews <-chan liveviewprojection.View, peerEvents chan<- factorydefinitions.FactoryEvent, peerViews <-chan liveviewprojection.View,
) {
	t.Helper()
	if err := selected.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := selected.Wait(ctx); err != nil {
		t.Fatalf("Wait after drain: %v", err)
	}
	select {
	case view := <-selectedViews:
		t.Fatalf("failed projection emitted a view: %+v", view)
	default:
	}
	select {
	case peerEvents <- event("peer-after-selected-close", 9):
	case <-ctx.Done():
		t.Fatal("selected cleanup stopped peer subscription")
	}
	select {
	case view := <-peerViews:
		if view.Runtime.TickCount != 8 || view.RetainedEventCount != 1 {
			t.Fatalf("peer view = %+v", view)
		}
	case <-ctx.Done():
		t.Fatal("peer did not present after selected drain")
	}
}
