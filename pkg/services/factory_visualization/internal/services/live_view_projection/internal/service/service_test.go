package service_test

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	projectionservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// TestLiveViewProjectionConformance proves the private implementation satisfies
// the accepted live-projection capability used by the Visualization root.
func TestLiveViewProjectionConformance(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 20, 10, 0, 0, 0, time.UTC)
	live := make(chan factorydefinitions.FactoryEvent, 1)
	history := event("history", 3)
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: []factorydefinitions.FactoryEvent{history},
			Events:  live,
		},
		snapshot: &liveviewprojection.RuntimeSnapshotFacts{
			RuntimeObservation: liveviewprojection.RuntimeObservation{TickCount: 3},
		},
	}
	projected := make(chan []factorydefinitions.FactoryEvent, 2)
	projections := &recordingsstub.Service{
		ReconstructWorldStateFn: func(request recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
			if request.SelectedTick != 3 {
				t.Fatalf("projection tick = %d, want 3", request.SelectedTick)
			}
			events := make([]factorydefinitions.FactoryEvent, len(request.Events))
			for index, event := range request.Events {
				events[index] = factorydefinitions.FactoryEvent{
					Id: string(event.ID),
					Context: factorydefinitions.FactoryEventContext{
						Sequence: int(event.Sequence),
					},
				}
			}
			projected <- events
			return recordings.ReconstructWorldStateResult{
				WorldState: recordings.WorldStateView{
					SchemaVersion: recordings.WorldStateViewSchemaV1,
					Payload:       `{"topology":{}}`,
				},
			}, nil
		},
	}
	rendered := make(chan liveviewprojection.View, 2)
	var svc liveviewprojection.Service
	impl, err := projectionservice.New(
		source,
		projections,
		fixedClock{now: now},
		liveviewprojection.SinkFunc(func(view liveviewprojection.View) { rendered <- view }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	svc = impl

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if got := <-projected; len(got) != 1 || got[0].Id != history.Id {
		t.Fatalf("initial projection events = %#v", got)
	}
	if got := <-rendered; !got.ObservedAt.Equal(now) || got.Runtime.TickCount != 3 {
		t.Fatalf("initial view = %#v", got)
	}

	liveEvent := event("live", 4)
	live <- liveEvent
	if got := <-projected; len(got) != 2 || got[1].Id != liveEvent.Id {
		t.Fatalf("live projection events = %#v", got)
	}
	<-rendered

	cursor := svc.ReconnectCursor()
	if cursor == nil || cursor.AfterEventID != liveEvent.Id ||
		cursor.AfterSequence == nil || *cursor.AfterSequence != 4 {
		t.Fatalf("cursor = %#v, want live event", cursor)
	}

	observed, err := svc.Observe(context.Background(), liveviewprojection.ObserveRequest{
		Mode: liveviewprojection.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if observed.View.TickCount != 3 || observed.View.RetainedEventCount != 2 {
		t.Fatalf("Observe view = %#v, want tick 3 retained 2", observed.View)
	}

	cancel()
	if err := svc.Wait(context.Background()); err != nil {
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
	stream        *factorydefinitions.FactoryEventStream
	subscribeErr  error
	subscribeHook func()
	snapshot      *liveviewprojection.RuntimeSnapshotFacts
	snapshotErr   error
}

func (s *sourceStub) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if s.subscribeHook != nil {
		s.subscribeHook()
	}
	return s.stream, s.subscribeErr
}

func (s *sourceStub) GetRuntimeSnapshotFacts(context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error) {
	return s.snapshot, s.snapshotErr
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
