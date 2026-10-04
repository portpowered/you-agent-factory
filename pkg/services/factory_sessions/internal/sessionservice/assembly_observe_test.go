package service

import (
	"context"
	"errors"
	"sync"
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
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

// The control double implements runtime outcomes, not opening-owner callbacks.
type scopedControlRuntime struct {
	invocationQueryRuntime
	status          string
	failure         error
	observedFailure error
	onObserve       func()
	last            factoryruntime.TerminateRequest
	controls        int
}

func (r *scopedControlRuntime) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	if r.onObserve != nil {
		r.onObserve()
	}
	return factoryruntime.ObserveResult{Observation: factoryruntime.Observation{Health: factoryruntime.ObservationHealth{FactoryState: r.status}}}, r.observedFailure
}
func (r *scopedControlRuntime) ControlTerminate(ctx context.Context, req factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	if err := ctx.Err(); err != nil {
		return factoryruntime.TerminateResult{}, err
	}
	r.last = req
	r.controls++
	if r.failure != nil {
		return factoryruntime.TerminateResult{}, r.failure
	}
	r.status = string(interfaces.FactoryStateCompleted)
	return factoryruntime.TerminateResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}

type scopeControlMetrics struct {
	factoryruntime.NoopEmitter
	outcomes []factoryruntime.Fields
}

func (m *scopeControlMetrics) Counter(_ context.Context, name string, _ float64, fields factoryruntime.Fields) error {
	if name == factoryruntime.RuntimeLifecycleControl {
		m.outcomes = append(m.outcomes, fields)
	}
	return nil
}

type scopeControlRecord struct {
	factoryruntime.RuntimeRecord
	metrics *scopeControlMetrics
}

func (r scopeControlRecord) RuntimeMetrics() factoryruntime.MetricsEmitter { return r.metrics }

func registerScopeControlRuntime(state *sessionruntime.Service, id string, runtime *scopedControlRuntime, logger *zap.Logger) *scopeControlMetrics {
	metrics := &scopeControlMetrics{}
	state.Register(sessionruntime.Registration{
		SessionID: id,
		Handle:    &runtimebinding.SessionState{Handle: invocationQueryRun{record: scopeControlRecord{metrics: metrics}}, Logger: logger},
		Runtime:   &factorysessions.LiveRuntime{Factory: runtime, WorkAndEventIngress: runtime},
	})
	return metrics
}

func TestSessionScopeControlPreservesLifecycleAndPeerIsolation(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	core, logs := observer.New(zap.InfoLevel)
	a := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning)}
	b := &scopedControlRuntime{invocationQueryRuntime: invocationQueryRuntime{name: "b"}, status: string(interfaces.FactoryStateRunning)}
	metrics := registerScopeControlRuntime(state, "a", a, zap.New(core))
	registerScopeControlRuntime(state, "b", b, zap.NewNop())
	control := NewScopeControl(state)
	assembly := &Assembly{state: state, scopeControl: control}
	request := factorysessions.SessionControlRequest{SessionID: "a", Operation: factorysessions.SessionControlCancel,
		Control:     factorysessions.ControlRequest{Reason: "owned cancellation"},
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "original-request", TurnID: "original-turn"}}
	result, err := assembly.ApplyLiveControl(context.Background(), request)
	if err != nil || result.SessionID != "a" || result.Status != factorysessions.LifecycleStatusSucceeded || result.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("cancel = %#v, %v", result, err)
	}
	if a.last.ControlID != "original-request" || a.last.TurnID != "original-turn" || a.last.WorkerSessionAction != factoryruntime.WorkerSessionControlActionCancel || a.last.Reason != "owned cancellation" {
		t.Fatalf("runtime control correlation = %#v", a.last)
	}
	replay, err := assembly.ApplyLiveControl(context.Background(), request)
	if err != nil || replay.Status != result.Status || a.controls != 1 || len(metrics.outcomes) != 1 || logs.Len() != 1 {
		t.Fatalf("dedup = %#v, %v; controls=%d metrics=%v logs=%v", replay, err, a.controls, metrics.outcomes, logs.All())
	}
	fields := logs.All()[0].ContextMap()
	if fields["session_id"] != "a" || fields["request_id"] != "original-request" {
		t.Fatalf("log correlation = %#v", fields)
	}
	peer, err := NewInvocationAuthority(state, platformclock.Real{}, nil).SubmitWork(context.Background(), "b", work.SubmitRequest{RequestID: "peer-next", WorkID: "peer-work"})
	if err != nil || peer.RequestID != "b" || b.controls != 0 || b.status != string(interfaces.FactoryStateRunning) {
		t.Fatalf("peer next admission = %#v, %v; controls=%d status=%s", peer, err, b.controls, b.status)
	}
}

func TestSessionScopeControlPreservesErrorsAndAllowsRetry(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	failure := errors.New("owned runtime control failed")
	runtime := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning), failure: failure}
	registerScopeControlRuntime(state, "a", runtime, zap.NewNop())
	control := NewScopeControl(state)
	request := factorysessions.ControlRequest{RequestID: "retryable"}
	if _, err := control.CancelLiveFactorySession(context.Background(), "missing", request); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := control.CancelLiveFactorySession(ctx, "a", request); !errors.Is(err, context.Canceled) || runtime.controls != 0 {
		t.Fatalf("canceled: %v", err)
	}
	runtime.observedFailure = failure
	if _, err := control.CancelLiveFactorySession(context.Background(), "a", request); !errors.Is(err, failure) || runtime.controls != 0 {
		t.Fatalf("observe: %v", err)
	}
	runtime.observedFailure = nil
	if _, err := control.CancelLiveFactorySession(context.Background(), "a", request); !errors.Is(err, failure) {
		t.Fatalf("control: %v", err)
	}
	runtime.failure = nil
	if result, err := control.CancelLiveFactorySession(context.Background(), "a", request); err != nil || result.Outcome != factorysessions.LifecycleControlOutcomeAccepted || runtime.controls != 2 {
		t.Fatalf("retry: %#v, %v", result, err)
	}
	if _, err := control.CancelLiveFactorySession(context.Background(), "a", factorysessions.ControlRequest{RequestID: "new-control"}); err == nil {
		t.Fatal("terminal generation accepted a new cancel")
	}
	if state.Resolve("a") == nil {
		t.Fatal("cancel removed inspection history")
	}
}

func TestSessionScopeControlKeepsSelectedGenerationDuringReplacement(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	first := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning)}
	replacement := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning)}
	firstMetrics := registerScopeControlRuntime(state, "a", first, zap.NewNop())
	var replacementMetrics *scopeControlMetrics
	first.onObserve = func() { replacementMetrics = registerScopeControlRuntime(state, "a", replacement, zap.NewNop()) }
	result, err := NewScopeControl(state).CancelLiveFactorySession(context.Background(), "a", factorysessions.ControlRequest{RequestID: "first-cancel"})
	if err != nil || result.Status != factorysessions.LifecycleStatusSucceeded || first.controls != 1 || replacement.controls != 0 || replacement.status != string(interfaces.FactoryStateRunning) {
		t.Fatalf("generation control = %#v, %v; first=%d replacement=%d", result, err, first.controls, replacement.controls)
	}
	if len(firstMetrics.outcomes) != 1 || len(replacementMetrics.outcomes) != 0 {
		t.Fatalf("generation metrics: first=%v replacement=%v", firstMetrics.outcomes, replacementMetrics.outcomes)
	}
}

// A slow addressed Runtime effect must not hold a process-wide control lock.
// The shared request ID is deliberately valid in each independent session.
func TestSessionScopeControlPeerProgressWhileCancellationIsBlocked(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	state := newWorkResolverSessionState()
	aStarted, releaseA, aDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(releaseA) }) }
	a := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning)}
	a.onObserve = func() {
		close(aStarted)
		select {
		case <-releaseA:
		case <-ctx.Done():
		}
	}
	b := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning)}
	aMetrics := registerScopeControlRuntime(state, "a", a, zap.NewNop())
	bMetrics := registerScopeControlRuntime(state, "b", b, zap.NewNop())
	control := NewScopeControl(state)
	request := factorysessions.ControlRequest{RequestID: "same-request", TurnID: "same-turn"}
	type completion struct {
		result factorysessions.LifecycleControlResult
		err    error
	}
	aCompleted := make(chan completion, 1)
	go func() {
		defer close(aDone)
		result, err := control.CancelLiveFactorySession(ctx, "a", request)
		aCompleted <- completion{result: result, err: err}
	}()
	t.Cleanup(func() {
		unblock()
		select {
		case <-aDone:
		case <-ctx.Done():
			t.Error("addressed cancellation did not join")
		}
	})
	select {
	case <-aStarted:
	case <-ctx.Done():
		t.Fatal("A did not reach its owned Runtime effect")
	}
	bDone := make(chan struct{})
	bCompleted := make(chan completion, 1)
	go func() {
		defer close(bDone)
		result, err := control.CancelLiveFactorySession(ctx, "b", request)
		bCompleted <- completion{result: result, err: err}
	}()
	t.Cleanup(func() {
		unblock()
		select {
		case <-bDone:
		case <-ctx.Done():
			t.Error("peer cancellation did not join")
		}
	})
	select {
	case peer := <-bCompleted:
		if peer.err != nil || peer.result.SessionID != "b" || peer.result.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
			t.Fatalf("peer cancel = %#v, %v", peer.result, peer.err)
		}
	case <-ctx.Done():
		t.Fatal("blocked A prevented B from completing its own cancellation")
	}
	select {
	case <-aDone:
		t.Fatal("A completed before its owned effect was released")
	default:
	}
	unblock()
	select {
	case own := <-aCompleted:
		if own.err != nil || own.result.SessionID != "a" || own.result.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
			t.Fatalf("addressed cancel = %#v, %v", own.result, own.err)
		}
	case <-ctx.Done():
		t.Fatal("A did not complete after its owned effect was released")
	}
	if a.controls != 1 || b.controls != 1 || a.last.ControlID != request.RequestID || b.last.ControlID != request.RequestID || len(aMetrics.outcomes) != 1 || len(bMetrics.outcomes) != 1 {
		t.Fatalf("independent correlation/effects: A=%#v B=%#v metrics=%v/%v", a.last, b.last, aMetrics.outcomes, bMetrics.outcomes)
	}
}
