package factory_visualization

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// FND-12 captured visualization-activation typed-failure baseline: activation
// construct fails with an explicit missing-dependency error. Invoked by
// `make fnd-12-visualization-behavior-baselines`.
func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	clock := fixedClock{now: time.Unix(1, 0)}
	sink := SinkFunc(func(View) {})
	projections := projectionStub{}
	source := &sourceStub{}
	tests := []struct {
		name string
		new  func() (*Service, error)
		want string
	}{
		{"source", func() (*Service, error) {
			return New(nil, projections, clock, sink, nil)
		}, "event source"},
		{"projections", func() (*Service, error) {
			return New(source, nil, clock, sink, nil)
		}, "projection service"},
		{"clock", func() (*Service, error) {
			return New(source, projections, nil, sink, nil)
		}, "clock"},
		{"sink", func() (*Service, error) {
			return New(source, projections, clock, nil, nil)
		}, "presentation sink"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := test.new()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want %q", err, test.want)
			}
		})
	}
}

// FND-12 captured visualization-activation success baseline: Start against a
// valid event source projects retained-then-live events and emits observable
// Views. Invoked by `make fnd-12-visualization-behavior-baselines`.
func TestServiceProjectsRetainedAndLiveFactoryEvents(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 20, 10, 0, 0, 0, time.UTC)
	live := make(chan factorydefinitions.FactoryEvent, 1)
	history := event("history", 3)
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: []factorydefinitions.FactoryEvent{history},
			Events:  live,
		},
		snapshot: &factoryruntime.StateSnapshot{TickCount: 3},
	}
	projected := make(chan []factorydefinitions.FactoryEvent, 2)
	projections := projectionStub{
		reconstruct: func(events []factorydefinitions.FactoryEvent, tick int) (factorydefinitions.FactoryWorldState, error) {
			if tick != 3 {
				t.Fatalf("projection tick = %d, want 3", tick)
			}
			projected <- append([]factorydefinitions.FactoryEvent(nil), events...)
			return factorydefinitions.FactoryWorldState{}, nil
		},
	}
	rendered := make(chan View, 2)
	service, err := New(
		source,
		projections,
		fixedClock{now: now},
		SinkFunc(func(view View) { rendered <- view }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if got := <-projected; len(got) != 1 || got[0].Id != history.Id {
		t.Fatalf("initial projection events = %#v", got)
	}
	if got := <-rendered; !got.ObservedAt.Equal(now) || got.EngineState.TickCount != 3 {
		t.Fatalf("initial view = %#v", got)
	}

	liveEvent := event("live", 4)
	live <- liveEvent
	if got := <-projected; len(got) != 2 || got[1].Id != liveEvent.Id {
		t.Fatalf("live projection events = %#v", got)
	}
	<-rendered

	service.mu.Lock()
	cursor := service.cursor
	service.mu.Unlock()
	if cursor == nil || cursor.AfterEventID != liveEvent.Id ||
		cursor.AfterSequence == nil || *cursor.AfterSequence != 4 {
		t.Fatalf("cursor = %#v, want live event", cursor)
	}
	cancel()
	if err := service.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestServiceReportsProjectionReadFailureWithoutStoppingSubscription(t *testing.T) {
	t.Parallel()

	live := make(chan factorydefinitions.FactoryEvent)
	readFailure := errors.New("snapshot unavailable")
	reported := make(chan error, 1)
	service, err := New(
		&sourceStub{
			stream:      &factorydefinitions.FactoryEventStream{Events: live},
			snapshotErr: readFailure,
		},
		projectionStub{},
		fixedClock{},
		SinkFunc(func(View) { t.Fatal("sink called after snapshot failure") }),
		func(err error) { reported <- err },
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := <-reported; !errors.Is(got, readFailure) {
		t.Fatalf("reported error = %v, want %v", got, readFailure)
	}
	cancel()
	if err := service.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func event(id string, sequence int) factorydefinitions.FactoryEvent {
	return factorydefinitions.FactoryEvent{
		Id: id,
		Context: factorydefinitions.FactoryEventContext{
			Sequence: sequence,
			Tick:     sequence,
		},
	}
}

type sourceStub struct {
	stream       *factorydefinitions.FactoryEventStream
	subscribeErr error
	snapshot     *factoryruntime.StateSnapshot
	snapshotErr  error
}

func (s *sourceStub) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return s.stream, s.subscribeErr
}

func (s *sourceStub) GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error) {
	return s.snapshot, s.snapshotErr
}

type projectionStub struct {
	reconstruct func([]factorydefinitions.FactoryEvent, int) (factorydefinitions.FactoryWorldState, error)
}

func (p projectionStub) ReconstructFactoryWorldState(
	events []factorydefinitions.FactoryEvent,
	tick int,
) (factorydefinitions.FactoryWorldState, error) {
	if p.reconstruct != nil {
		return p.reconstruct(events, tick)
	}
	return factorydefinitions.FactoryWorldState{}, nil
}

func (projectionStub) SimpleDashboardRenderData(
	factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (projectionStub) ProjectActiveThrottlePauses(
	factorydefinitions.InitialStructurePayload,
	[]factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return nil
}

func (projectionStub) ProjectWorkstationRequests(
	factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func (projectionStub) ValidateReconnectReplay(
	[]factorydefinitions.FactoryEvent,
	factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) error {
	return nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
