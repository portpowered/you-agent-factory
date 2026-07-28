package factory_visualization

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestCurrentRuntimeSourceBindsThroughSessionRuntimeReader(t *testing.T) {
	t.Parallel()

	runtimeReadCalls := 0
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(fn func(*factorysessions.LiveRuntime) error) error {
			runtimeReadCalls++
			return fn(&factorysessions.LiveRuntime{
				Factory: &sessionBoundRuntimeFactory{
					stream: &factorydefinitions.FactoryEventStream{
						Events: make(chan factorydefinitions.FactoryEvent),
					},
					observation: factoryruntime.Observation{
						Progress: factoryruntime.ObservationProgress{TickCount: 5},
					},
				},
			})
		},
	}
	source := NewCurrentRuntimeSource(reader)

	stream, err := source.SubscribeFactoryEvents(
		context.Background(),
		nil,
		factorydefinitions.FactoryEventReconnectScope{},
	)
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: error = %v", err)
	}
	if stream == nil || stream.Events == nil {
		t.Fatal("SubscribeFactoryEvents returned invalid stream")
	}
	if runtimeReadCalls != 1 {
		t.Fatalf("WithRuntimeRead calls = %d, want 1", runtimeReadCalls)
	}

	facts, err := source.GetRuntimeSnapshotFacts(context.Background())
	if err != nil {
		t.Fatalf("GetRuntimeSnapshotFacts: error = %v", err)
	}
	if facts == nil || facts.RuntimeObservation.TickCount != 5 {
		t.Fatalf("GetRuntimeSnapshotFacts = %#v, want tick 5", facts)
	}
	if runtimeReadCalls != 2 {
		t.Fatalf("WithRuntimeRead calls after snapshot = %d, want 2", runtimeReadCalls)
	}
}

func TestCurrentRuntimeSourceUnavailableRuntimeDoesNotSubscribe(t *testing.T) {
	t.Parallel()

	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(func(*factorysessions.LiveRuntime) error) error {
			return factorysessions.ErrRuntimeNotAvailable
		},
	}
	source := NewCurrentRuntimeSource(reader)

	_, err := source.SubscribeFactoryEvents(
		context.Background(),
		nil,
		factorydefinitions.FactoryEventReconnectScope{},
	)
	if !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("SubscribeFactoryEvents error = %v, want ErrRuntimeNotAvailable", err)
	}
}

func TestActivateThroughSessionBoundSourceReachesStarted(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(fn func(*factorysessions.LiveRuntime) error) error {
			return fn(&factorysessions.LiveRuntime{
				Factory: &sessionBoundRuntimeFactory{
					subscribeHook: func() { subscribeCalls++ },
					stream: &factorydefinitions.FactoryEventStream{
						Events: make(chan factorydefinitions.FactoryEvent),
					},
					observation: factoryruntime.Observation{
						Progress: factoryruntime.ObservationProgress{TickCount: 2},
					},
				},
			})
		},
	}
	service, err := New(
		NewCurrentRuntimeSource(reader),
		&recordingsstub.Service{},
		fixedClock{now: time.Unix(1, 0)},
		SinkFunc(func(View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() with session-bound source: error = %v", err)
	}
	if subscribeCalls != 0 {
		t.Fatalf("construction subscribe calls = %d, want inert", subscribeCalls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := service.Activate(ctx, ActivateRequest{Mode: ActivateModeRetainedThenLive})
	if err != nil {
		t.Fatalf("Activate through session-bound source: error = %v", err)
	}
	if result.State != LifecycleStateStarted {
		t.Fatalf("Activate state = %q, want %q", result.State, LifecycleStateStarted)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls after Activate = %d, want 1", subscribeCalls)
	}
}

func TestActivateWithUnavailableSessionRuntimeDoesNotSubscribe(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(func(*factorysessions.LiveRuntime) error) error {
			return factorysessions.ErrRuntimeNotAvailable
		},
	}
	service, err := New(
		NewCurrentRuntimeSource(reader),
		&recordingsstub.Service{},
		fixedClock{now: time.Unix(1, 0)},
		SinkFunc(func(View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() with unavailable session runtime: error = %v", err)
	}

	_, err = service.Activate(context.Background(), ActivateRequest{
		Mode: ActivateModeRetainedThenLive,
	})
	if err == nil {
		t.Fatal("Activate with unavailable runtime: error = nil, want bind failure")
	}
	if !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("Activate bind failure = %v, want ErrRuntimeNotAvailable", err)
	}
	if subscribeCalls != 0 {
		t.Fatalf("subscribe calls after failed Activate = %d, want no subscription", subscribeCalls)
	}
}

type sessionRuntimeReaderStub struct {
	withRuntimeRead func(func(*factorysessions.LiveRuntime) error) error
}

func (s sessionRuntimeReaderStub) WithRuntimeRead(
	fn func(*factorysessions.LiveRuntime) error,
) error {
	if s.withRuntimeRead == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return s.withRuntimeRead(fn)
}

type sessionBoundRuntimeFactory struct {
	factoryruntime.Service
	subscribeHook func()
	stream        *factorydefinitions.FactoryEventStream
	observation   factoryruntime.Observation
}

func (f *sessionBoundRuntimeFactory) SubmitWorkRequest(
	context.Context,
	work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}

func (f *sessionBoundRuntimeFactory) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if f.subscribeHook != nil {
		f.subscribeHook()
	}
	return f.stream, nil
}

func (f *sessionBoundRuntimeFactory) Observe(
	context.Context,
	factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{Observation: f.observation}, nil
}
