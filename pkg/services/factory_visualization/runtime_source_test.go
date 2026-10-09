package factory_visualization_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	. "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestCurrentRuntimeSourceBindsThroughSessionRuntimeReader(t *testing.T) {
	t.Parallel()

	runtimeReadCalls := 0
	reader := sessionRuntimeReaderStub{
		withRuntimeRead: func(fn func(*factorysessions.LiveRuntime) error) error {
			runtimeReadCalls++
			runtimeFactory := &sessionBoundRuntimeFactory{
				stream: &factorydefinitions.FactoryEventStream{
					Events: make(chan factorydefinitions.FactoryEvent),
				},
				observation: factoryruntime.Observation{
					Progress: factoryruntime.ObservationProgress{TickCount: 5},
				},
			}
			return fn(&factorysessions.LiveRuntime{
				Factory:             runtimeFactory,
				WorkAndEventIngress: runtimeFactory,
			})
		},
	}
	source := factoryvisualizationwire.NewRuntimeSourceOpening(reader)("selected-session")

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
	source := factoryvisualizationwire.NewRuntimeSourceOpening(reader)("selected-session")

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
			runtimeFactory := &sessionBoundRuntimeFactory{
				subscribeHook: func() { subscribeCalls++ },
				stream: &factorydefinitions.FactoryEventStream{
					Events: make(chan factorydefinitions.FactoryEvent),
				},
				observation: factoryruntime.Observation{
					Progress: factoryruntime.ObservationProgress{TickCount: 2},
				},
			}
			return fn(&factorysessions.LiveRuntime{
				Factory:             runtimeFactory,
				WorkAndEventIngress: runtimeFactory,
			})
		},
	}
	service, err := openVisualizationScope(
		factoryvisualizationwire.NewRuntimeSourceOpening(reader)("selected-session"),
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
	service, err := openVisualizationScope(
		factoryvisualizationwire.NewRuntimeSourceOpening(reader)("selected-session"),
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
	withRuntimeRead           func(func(*factorysessions.LiveRuntime) error) error
	withRuntimeReadForSession func(string, func(*factorysessions.LiveRuntime) error) error
}

func (s sessionRuntimeReaderStub) WithRuntimeRead(
	fn func(*factorysessions.LiveRuntime) error,
) error {
	if s.withRuntimeRead == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return s.withRuntimeRead(fn)
}

func (s sessionRuntimeReaderStub) WithRuntimeReadForSession(id string, fn func(*factorysessions.LiveRuntime) error) error {
	if s.withRuntimeReadForSession != nil {
		return s.withRuntimeReadForSession(id, fn)
	}
	return s.WithRuntimeRead(fn)
}

func TestRuntimeSourceOpeningKeepsConcurrentSessionObservationsIsolated(t *testing.T) {
	t.Parallel()
	runtimes := map[string]*sessionBoundRuntimeFactory{
		"selected": {stream: &factorydefinitions.FactoryEventStream{History: []factorydefinitions.FactoryEvent{{Id: "selected-retained"}}}, observation: factoryruntime.Observation{Progress: factoryruntime.ObservationProgress{TickCount: 4}}},
		"peer":     {stream: &factorydefinitions.FactoryEventStream{History: []factorydefinitions.FactoryEvent{{Id: "peer-retained"}}}, observation: factoryruntime.Observation{Progress: factoryruntime.ObservationProgress{TickCount: 9}}},
	}
	open := internalservice.NewRuntimeSourceOpening(sessionRuntimeReaderStub{
		withRuntimeRead: func(func(*factorysessions.LiveRuntime) error) error {
			return errors.New("unexpected Current Factory read")
		},
		withRuntimeReadForSession: func(id string, read func(*factorysessions.LiveRuntime) error) error {
			runtime := runtimes[id]
			if runtime == nil {
				return factorysessions.ErrSessionNotFound
			}
			return read(&factorysessions.LiveRuntime{Factory: runtime, WorkAndEventIngress: runtime})
		},
	})
	for id, runtime := range runtimes {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			source := open(id)
			stream, err := source.SubscribeFactoryEvents(t.Context(), nil, factorydefinitions.FactoryEventReconnectScope{})
			if err != nil || stream != runtime.stream || stream.History[0].Id != id+"-retained" {
				t.Fatalf("selected stream = (%#v, %v)", stream, err)
			}
			facts, err := source.GetRuntimeSnapshotFacts(t.Context())
			if err != nil || facts == nil || facts.TickCount != runtime.observation.Progress.TickCount {
				t.Fatalf("selected observation = (%#v, %v)", facts, err)
			}
		})
	}
	for _, id := range []string{"", "missing"} {
		source := open(id)
		if stream, err := source.SubscribeFactoryEvents(t.Context(), nil, factorydefinitions.FactoryEventReconnectScope{}); stream != nil || !errors.Is(err, factorysessions.ErrSessionNotFound) {
			t.Fatalf("missing %q stream = (%#v, %v)", id, stream, err)
		}
		if facts, err := source.GetRuntimeSnapshotFacts(t.Context()); facts != nil || !errors.Is(err, factorysessions.ErrSessionNotFound) {
			t.Fatalf("missing %q observation = (%#v, %v)", id, facts, err)
		}
	}
}

type sessionBoundRuntimeFactory struct {
	factoryruntime.Service
	subscribeHook   func()
	stream          *factorydefinitions.FactoryEventStream
	observation     factoryruntime.Observation
	observeRequests []factoryruntime.ObserveRequest
	observeErr      error
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
	_ context.Context,
	req factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	f.observeRequests = append(f.observeRequests, req)
	if f.observeErr != nil {
		return factoryruntime.ObserveResult{}, f.observeErr
	}
	return factoryruntime.ObserveResult{Observation: f.observation}, nil
}

func openVisualizationScope(source Source, peer recordings.Service, clock Clock, sink Sink, reportError ErrorReporter) (Service, error) {
	return factoryvisualizationwire.NewScopeOpening(
		factoryvisualizationwire.NewActivationOpening(peer),
		factoryvisualizationwire.NewProjectionOpening(peer),
		factoryvisualizationwire.NewResponsePresentation(),
		peer,
	)(source, clock, sink, reportError)
}
