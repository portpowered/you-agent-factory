package factory_visualization_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestRuntimeSourceOpeningIsInertWithoutRuntime(t *testing.T) {
	t.Parallel()
	source := internalservice.NewRuntimeSourceOpening(nil)("selected-session")
	if source == nil {
		t.Fatal("opening returned nil source")
	}
	if facts, err := source.GetRuntimeSnapshotFacts(t.Context()); facts != nil || !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("unavailable runtime facts=(%+v,%v)", facts, err)
	}
}

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
	source := internalservice.NewRuntimeSourceOpening(reader)("selected-session")

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
	source := internalservice.NewRuntimeSourceOpening(reader)("selected-session")

	_, err := source.SubscribeFactoryEvents(
		context.Background(),
		nil,
		factorydefinitions.FactoryEventReconnectScope{},
	)
	if !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("SubscribeFactoryEvents error = %v, want ErrRuntimeNotAvailable", err)
	}
}

// Subscription activation is the runtime source's boundary. Lifecycle state
// and emitted views are covered separately beside activation_lifecycle.
func TestSelectedRuntimeSourceSubscriptionIsInertUntilRequested(t *testing.T) {
	t.Parallel()
	subscriptions := 0
	stream := &factorydefinitions.FactoryEventStream{Events: make(chan factorydefinitions.FactoryEvent)}
	runtimeFactory := &sessionBoundRuntimeFactory{stream: stream, subscribeHook: func() { subscriptions++ }}
	source := internalservice.NewRuntimeSourceOpening(sessionRuntimeReaderStub{withRuntimeReadForSession: func(id string, read func(*factorysessions.LiveRuntime) error) error {
		if id != "selected-session" {
			t.Fatalf("selected id=%q", id)
		}
		return read(&factorysessions.LiveRuntime{Factory: runtimeFactory, WorkAndEventIngress: runtimeFactory})
	}})("selected-session")
	if subscriptions != 0 {
		t.Fatal("opening subscribed before explicit request")
	}
	got, err := source.SubscribeFactoryEvents(t.Context(), nil, factorydefinitions.FactoryEventReconnectScope{})
	if err != nil || got != stream || subscriptions != 1 {
		t.Fatalf("subscription=(%+v,%v), calls=%d", got, err, subscriptions)
	}
}

func TestSelectedRuntimeSourceUnavailableSessionDoesNotSubscribe(t *testing.T) {
	t.Parallel()
	source := internalservice.NewRuntimeSourceOpening(sessionRuntimeReaderStub{withRuntimeReadForSession: func(string, func(*factorysessions.LiveRuntime) error) error {
		return factorysessions.ErrRuntimeNotAvailable
	}})("selected-session")
	stream, err := source.SubscribeFactoryEvents(t.Context(), nil, factorydefinitions.FactoryEventReconnectScope{})
	if stream != nil || !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("subscription=(%+v,%v), want unavailable and no stream", stream, err)
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
