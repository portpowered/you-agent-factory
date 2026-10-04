package service

import (
	"context"
	"errors"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type observeStubRuntime struct {
	factoryruntime.Service
	result factoryruntime.ObserveResult
	err    error
}

func (r *observeStubRuntime) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return r.result, r.err
}

func TestAssemblyObserveForSessionUsesCanonicalRegistry(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	want := factoryruntime.ObserveResult{}
	state.Register(sessionruntime.Registration{
		SessionID: "sess-observe",
		Handle:    struct{}{},
		Select:    true,
		Runtime: &factorysessions.LiveRuntime{
			Factory: &observeStubRuntime{result: want},
		},
	})
	assembly := &Assembly{state: state}

	got, err := assembly.ObserveForSession(context.Background(), "sess-observe", factoryruntime.ObserveRequest{})
	if err != nil {
		t.Fatalf("ObserveForSession: %v", err)
	}
	_ = got

	if _, err := assembly.ObserveForSession(context.Background(), "missing", factoryruntime.ObserveRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing error = %v, want ErrSessionNotFound", err)
	}
}

// These doubles expose only the query adapter's consumed runtime boundaries.
type invocationQueryConfig struct {
	interfaces.LoadedFactorySource
	config *interfaces.FactoryConfig
}

func (c invocationQueryConfig) FactoryConfig() *interfaces.FactoryConfig { return c.config }

type invocationQueryRun struct {
	factoryruntime.RuntimeRun
	record factoryruntime.RuntimeRecord
}

func (r invocationQueryRun) RuntimeInstance() factoryruntime.RuntimeRecord { return r.record }

type invocationQueryRuntime struct {
	factoryruntime.Service
	name      string
	submitted work.WorkRequest
	events    chan interfaces.FactoryEvent
	scope     interfaces.FactoryEventReconnectScope
	stopped   chan struct{}
}

func (r *invocationQueryRuntime) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	r.submitted = request
	return work.WorkRequestSubmitResult{RequestID: r.name}, nil
}
func (r *invocationQueryRuntime) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, nil
}
func (r *invocationQueryRuntime) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return []interfaces.FactoryEvent{{Type: interfaces.FactoryEventTypeWorkRequest, Id: r.name}}, nil
}
func (r *invocationQueryRuntime) SubscribeFactoryEvents(ctx context.Context, _ *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	r.scope = scope
	r.events = make(chan interfaces.FactoryEvent)
	r.stopped = make(chan struct{})
	go func() { <-ctx.Done(); close(r.events); close(r.stopped) }()
	return &interfaces.FactoryEventStream{Events: r.events}, nil
}

func registerInvocationQueryGeneration(state *sessionruntime.Service, id string, runtime *invocationQueryRuntime, config *interfaces.FactoryConfig) {
	record := &generationRuntimeRecord{service: runtime}
	state.Register(sessionruntime.Registration{
		SessionID: id,
		Handle:    &runtimebinding.SessionState{Handle: invocationQueryRun{record: record}},
		Runtime: &factorysessions.LiveRuntime{
			Factory: runtime, WorkAndEventIngress: runtime,
			RuntimeConfig: invocationQueryConfig{config: config},
		},
	})
}

func TestInvocationAuthoritySelectsAddressedGenerationAndPreservesTypedErrors(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	var projectedSession string
	authority := NewInvocationAuthority(state, platformclock.Real{}, func(events []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
		projectedSession = events[0].Id
		return interfaces.FactoryWorldState{}, nil
	})
	first := &invocationQueryRuntime{name: "a-first"}
	peer := &invocationQueryRuntime{name: "b"}
	replacement := &invocationQueryRuntime{name: "a-replacement"}
	firstConfig, peerConfig, replacementConfig := &interfaces.FactoryConfig{}, &interfaces.FactoryConfig{}, &interfaces.FactoryConfig{}
	registerInvocationQueryGeneration(state, "a", first, firstConfig)
	registerInvocationQueryGeneration(state, "b", peer, peerConfig)
	assertInvocationQueryGeneration(t, authority, "a", first, firstConfig)
	if projectedSession != first.name {
		t.Fatalf("projected session = %q", projectedSession)
	}
	registerInvocationQueryGeneration(state, "a", replacement, replacementConfig)
	assertInvocationQueryGeneration(t, authority, "a", replacement, replacementConfig)
	if projectedSession != replacement.name {
		t.Fatalf("projected session = %q", projectedSession)
	}
	assertInvocationQueryGeneration(t, authority, "b", peer, peerConfig)
	if projectedSession != peer.name {
		t.Fatalf("projected peer = %q", projectedSession)
	}
	for _, id := range []string{"missing", ""} {
		if _, err := authority.FactoryConfig(id); !errors.Is(err, factorysessions.ErrSessionNotFound) {
			t.Fatalf("config(%q): %v", id, err)
		}
		if _, err := authority.SubmitWork(context.Background(), id, work.SubmitRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
			t.Fatalf("submit(%q): %v", id, err)
		}
		if _, err := authority.Observe(context.Background(), id, sessioninvocation.SessionInvocationWaitInput{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
			t.Fatalf("observe(%q): %v", id, err)
		}
		if waiter, release := authority.WaitSession(context.Background(), id); waiter != nil || release != nil {
			t.Fatalf("missing waiter(%q) allocated", id)
		}
	}
}

func assertInvocationQueryGeneration(t *testing.T, authority InvocationAuthority, id string, runtime *invocationQueryRuntime, config *interfaces.FactoryConfig) {
	t.Helper()
	gotConfig, err := authority.FactoryConfig(id)
	if err != nil || gotConfig != config {
		t.Fatalf("config(%s) = %v, %v", id, gotConfig, err)
	}
	result, err := authority.SubmitWork(context.Background(), id, work.SubmitRequest{RequestID: id + "-request", WorkID: id + "-work"})
	if err != nil || result.RequestID != runtime.name || runtime.submitted.RequestID != id+"-request" || len(runtime.submitted.Works) != 1 || runtime.submitted.Works[0].WorkID != id+"-work" {
		t.Fatalf("submit(%s) = %#v, %v; admitted %#v", id, result, err, runtime.submitted)
	}
	if _, err := authority.Observe(context.Background(), id, sessioninvocation.SessionInvocationWaitInput{}); err != nil {
		t.Fatalf("observe(%s): %v", id, err)
	}
	waiter, release := authority.WaitSession(context.Background(), id)
	if waiter == nil || release == nil {
		t.Fatalf("wait(%s) unavailable", id)
	}
	defer func() { release(); <-runtime.stopped }()
	if runtime.scope.SessionID != id || runtime.scope.HistoryLimit != 1 {
		t.Fatalf("subscription scope = %#v", runtime.scope)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case runtime.events <- interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeWorkRequest}:
	case <-ctx.Done():
		t.Fatal("event relay did not consume event")
	}
	if err := waiter(ctx); err != nil {
		t.Fatalf("wait(%s): %v", id, err)
	}
}

func TestInvocationAuthorityPreservesObservationFailure(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	runtime := &invocationQueryRuntime{name: "a"}
	registerInvocationQueryGeneration(state, "a", runtime, &interfaces.FactoryConfig{})
	sentinel := errors.New("projection unavailable")
	authority := NewInvocationAuthority(state, platformclock.Real{}, func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
		return interfaces.FactoryWorldState{}, sentinel
	})
	if _, err := authority.Observe(context.Background(), "a", sessioninvocation.SessionInvocationWaitInput{}); !errors.Is(err, sentinel) {
		t.Fatalf("observe error = %v", err)
	}
}

type invocationTimerReady struct {
	platformclock.TimerSource
	ready chan struct{}
}

func (s invocationTimerReady) NewTimer(duration time.Duration) platformclock.Timer {
	timer := s.TimerSource.NewTimer(duration)
	s.ready <- struct{}{}
	return timer
}

func TestInvocationAuthorityWaiterUsesSelectedScheduler(t *testing.T) {
	t.Parallel()
	scheduler := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
	wake := make(chan struct{}, 1)
	wake <- struct{}{}
	selected := invocationTimerReady{TimerSource: scheduler, ready: make(chan struct{}, 1)}
	waiter := newEventDrivenInvocationWaiter(wake, selected)
	if err := waiter(context.Background()); err != nil {
		t.Fatalf("wake: %v", err)
	}
	<-selected.ready
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- waiter(ctx) }()
	select {
	case <-selected.ready:
	case <-ctx.Done():
		t.Fatal("selected scheduler was not used")
	}
	scheduler.SetTick(250)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("scheduled wake: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("selected timer did not wake waiter")
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := waiter(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
}
