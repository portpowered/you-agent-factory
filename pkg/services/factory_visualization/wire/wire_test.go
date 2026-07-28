package wire_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type wireSourceStub struct {
	subscribeHook func()
}

func (s wireSourceStub) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if s.subscribeHook != nil {
		s.subscribeHook()
	}
	return &factorydefinitions.FactoryEventStream{
		Events: make(chan factorydefinitions.FactoryEvent),
	}, nil
}

func (s wireSourceStub) GetRuntimeSnapshotFacts(context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error) {
	return &liveviewprojection.RuntimeSnapshotFacts{}, nil
}

type wireProjectionStub struct{}

func (wireProjectionStub) ReconstructFactoryWorldState(
	[]factorydefinitions.FactoryEvent,
	int,
) (factorydefinitions.FactoryWorldState, error) {
	return factorydefinitions.FactoryWorldState{}, nil
}

func (wireProjectionStub) SimpleDashboardRenderData(
	factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (wireProjectionStub) ProjectActiveThrottlePauses(
	factorydefinitions.InitialStructurePayload,
	[]factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return nil
}

func (wireProjectionStub) ProjectWorkstationRequests(
	factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func (wireProjectionStub) ValidateReconnectReplay(
	[]factorydefinitions.FactoryEvent,
	factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) error {
	return nil
}

type wireClock struct{}

func (wireClock) Now() time.Time { return time.Unix(1, 0) }

func TestNewRootRejectsMissingConstructionPorts(t *testing.T) {
	t.Parallel()

	clock := wireClock{}
	sink := factoryvisualization.SinkFunc(func(factoryvisualization.View) {})
	projections := wireProjectionStub{}
	source := wireSourceStub{}
	tests := []struct {
		name string
		new  func() (factoryvisualization.Root, error)
		want string
	}{
		{
			name: "source",
			new: func() (factoryvisualization.Root, error) {
				return factoryvisualizationwire.NewRoot(nil, projections, clock, sink, nil)
			},
			want: "event source is required",
		},
		{
			name: "projections",
			new: func() (factoryvisualization.Root, error) {
				return factoryvisualizationwire.NewRoot(source, nil, clock, sink, nil)
			},
			want: "projection service is required",
		},
		{
			name: "clock",
			new: func() (factoryvisualization.Root, error) {
				return factoryvisualizationwire.NewRoot(source, projections, nil, sink, nil)
			},
			want: "clock is required",
		},
		{
			name: "sink",
			new: func() (factoryvisualization.Root, error) {
				return factoryvisualizationwire.NewRoot(source, projections, clock, nil, nil)
			},
			want: "presentation sink is required",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, err := test.new()
			if root != nil {
				t.Fatal("NewRoot() returned non-nil root, want nil")
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewRoot() error = %v, want %q", err, test.want)
			}
		})
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

func requirePresentationError(
	t *testing.T,
	err error,
	kind factoryvisualization.PresentationErrorKind,
	label string,
) {
	t.Helper()
	var presErr *factoryvisualization.PresentationError
	if !errors.As(err, &presErr) || presErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}

func mustNewWireRoot(t *testing.T) factoryvisualization.Root {
	t.Helper()
	root, err := factoryvisualizationwire.NewRoot(
		wireSourceStub{},
		wireProjectionStub{},
		wireClock{},
		factoryvisualization.SinkFunc(func(factoryvisualization.View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewRoot() returned nil root")
	}
	return root
}

func TestNewRootServesPublishedPeerBehavior(t *testing.T) {
	t.Parallel()

	var root factoryvisualization.Root = mustNewWireRoot(t)

	_, err := root.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{})
	requirePresentationError(
		t,
		err,
		factoryvisualization.PresentationErrorInvalidInput,
		"OpenPresentation missing parameters",
	)

	opened, err := root.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("OpenPresentation best-effort: error = %v", err)
	}
	if opened.SessionID == "" {
		t.Fatalf("OpenPresentation result = %#v, want non-empty session id", opened)
	}
	if opened.Mode != factoryvisualization.PresentationDeliveryBestEffort {
		t.Fatalf("OpenPresentation mode = %q, want %q", opened.Mode, factoryvisualization.PresentationDeliveryBestEffort)
	}
}

func TestNewRootConstructsInertRoot(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	presentCalls := 0
	projectionReads := 0
	source := wireSourceStub{
		subscribeHook: func() {
			subscribeCalls++
			panic("event subscription started during inert construction")
		},
	}
	projections := wireRecordingProjectionStub{
		reconstructHook: func() {
			projectionReads++
			panic("projection read during inert construction")
		},
	}
	sink := factoryvisualization.SinkFunc(func(factoryvisualization.View) {
		presentCalls++
		panic("presentation sink invoked during inert construction")
	})

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	root, err := factoryvisualizationwire.NewRoot(
		source,
		projections,
		wireClock{},
		sink,
		nil,
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewRoot() returned nil root")
	}
	var peer factoryvisualization.Root = root
	if subscribeCalls != 0 || presentCalls != 0 || projectionReads != 0 {
		t.Fatalf(
			"NewRoot() side effects: subscribe=%d present=%d projection=%d, want inert construction",
			subscribeCalls, presentCalls, projectionReads,
		)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d, want no lifecycle goroutines",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}

	_, err = peer.Join(context.Background(), factoryvisualization.JoinRequest{})
	requireLifecycleError(t, err, factoryvisualization.LifecycleErrorNotActivated, "Join before Activate")
	if subscribeCalls != 0 || presentCalls != 0 || projectionReads != 0 {
		t.Fatal("Join before Activate must not subscribe, present, or read projections")
	}
}

type wireRecordingProjectionStub struct {
	wireProjectionStub
	reconstructHook func()
}

func (s wireRecordingProjectionStub) ReconstructFactoryWorldState(
	events []factorydefinitions.FactoryEvent,
	tick int,
) (factorydefinitions.FactoryWorldState, error) {
	if s.reconstructHook != nil {
		s.reconstructHook()
	}
	return s.wireProjectionStub.ReconstructFactoryWorldState(events, tick)
}
