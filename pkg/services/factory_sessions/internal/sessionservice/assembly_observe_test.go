package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
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
		waiter, release := authority.WaitSession(context.Background(), id)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := waiter(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("fallback waiter(%q): %v", id, err)
		}
		release()
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
	assertInvocationQueryWait(t, authority, id, runtime)

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

func TestSessionScopeControlStopPreservesSelectedClockErrorsAndRetry(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	a := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning)}
	b := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning)}
	core, logs := observer.New(zap.InfoLevel)
	registerScopeControlRuntime(state, "a", a, zap.New(core))
	registerScopeControlRuntime(state, "b", b, zap.NewNop())
	bound := runtimebinding.SessionStateFrom(state.Resolve("a"))
	bound.Clock = clockwork.NewFakeClockAt(time.Date(2004, 5, 6, 0, 0, 0, 0, time.UTC))
	wantError := errors.New("injected artifact failure")
	failure := wantError
	control := NewScopeControl(state, func(run factoryruntime.RuntimeRun, clock factoryruntime.Clock) error {
		if run != bound.Handle || !clock.Now().Equal(bound.Clock.Now()) {
			t.Fatal("stop selected a foreign run or fact time")
		}
		return failure
	}, zap.New(core))
	if err := control.StopLiveSession(context.Background(), " a "); !errors.Is(err, wantError) {
		t.Fatalf("failed stop = %v", err)
	}
	if state.Resolve("a") == nil || state.Resolve("b") == nil {
		t.Fatal("failed stop retired a session")
	}
	failure = nil
	if err := control.StopLiveSession(context.Background(), "a"); err != nil {
		t.Fatalf("retry stop = %v", err)
	}
	for _, terminal := range []error{context.Canceled, factoryruntime.ErrAlreadyStopped, factoryruntime.ErrNotRunning} {
		failure = terminal
		if err := control.StopLiveSession(context.Background(), "a"); err != nil {
			t.Fatalf("terminal stop %v = %v", terminal, err)
		}
	}
	if state.Resolve("a") == nil || b.controls != 0 {
		t.Fatal("stop changed retirement ownership or controlled peer")
	}
	if logs.FilterMessage("stop live Factory Session runtime failed").Len() != 1 {
		t.Fatal("missing structured stop failure")
	}
	if err := control.StopLiveSession(context.Background(), "missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing stop = %v", err)
	}
	var detached *factorysessions.DetachedRequestError
	if err := control.StopLiveSession(context.Background(), " "); !errors.As(err, &detached) {
		t.Fatalf("empty selector = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := control.StopLiveSession(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled stop = %v", err)
	}
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
func (r scopeControlRecord) RecordingLedger() recordings.Ledger            { return nil }

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
	control := NewScopeControl(state, nil, zap.NewNop())
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
	assertScopedControlDedupAndPeer(t, assembly, request, result, a, b, metrics, logs, state)

}

func TestSessionScopeControlPreservesErrorsAndAllowsRetry(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	failure := errors.New("owned runtime control failed")
	runtime := &scopedControlRuntime{status: string(interfaces.FactoryStateRunning), failure: failure}
	registerScopeControlRuntime(state, "a", runtime, zap.NewNop())
	control := NewScopeControl(state, nil, zap.NewNop())
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
	result, err := NewScopeControl(state, nil, zap.NewNop()).CancelLiveFactorySession(context.Background(), "a", factorysessions.ControlRequest{RequestID: "first-cancel"})
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
	control := NewScopeControl(state, nil, zap.NewNop())
	request := factorysessions.ControlRequest{RequestID: "same-request", TurnID: "same-turn"}

	aCompleted := make(chan scopedControlCompletion, 1)
	go func() {
		defer close(aDone)
		result, err := control.CancelLiveFactorySession(ctx, "a", request)
		aCompleted <- scopedControlCompletion{result: result, err: err}
	}()
	t.Cleanup(func() {
		unblock()
		assertScopedCancellationJoined(t, ctx, aDone, "addressed")
	})
	select {
	case <-aStarted:
	case <-ctx.Done():
		t.Fatal("A did not reach its owned Runtime effect")
	}
	bDone := make(chan struct{})
	bCompleted := make(chan scopedControlCompletion, 1)
	go func() {
		defer close(bDone)
		result, err := control.CancelLiveFactorySession(ctx, "b", request)
		bCompleted <- scopedControlCompletion{result: result, err: err}
	}()
	t.Cleanup(func() {
		unblock()
		assertScopedCancellationJoined(t, ctx, bDone, "peer")
	})
	select {
	case peer := <-bCompleted:
		assertScopedControlCompletion(t, peer, "b")
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
		assertScopedControlCompletion(t, own, "a")
	case <-ctx.Done():
		t.Fatal("A did not complete after its owned effect was released")
	}
	assertIndependentScopedControlEffects(t, a, b, aMetrics, bMetrics, request)

}

func TestInvocationAuthorityMissingSubscriptionUsesSelectedFallbackTimer(t *testing.T) {
	t.Parallel()
	scheduler := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
	state := newWorkResolverSessionState()
	selected := invocationTimerReady{TimerSource: scheduler, ready: make(chan struct{}, 1)}
	authority := NewInvocationAuthority(state, selected, nil)
	waiter, release := authority.WaitSession(t.Context(), "missing")
	defer release()
	done := make(chan error, 1)
	go func() { done <- waiter(t.Context()) }()
	select {
	case <-selected.ready:
	case <-t.Context().Done():
		t.Fatal("selected fallback timer did not open")
	}
	scheduler.SetTick(250)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fallback: %v", err)
		}
	case <-t.Context().Done():
		t.Fatal("selected timer did not wake fallback")
	}
}

func assertInvocationQueryWait(t *testing.T, authority InvocationAuthority, id string, runtime *invocationQueryRuntime) {
	t.Helper()
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

func assertScopedControlDedupAndPeer(t *testing.T, assembly *Assembly, request factorysessions.SessionControlRequest, result factorysessions.SessionControlResult, a, b *scopedControlRuntime, metrics *scopeControlMetrics, logs *observer.ObservedLogs, state *sessionruntime.Service) {
	t.Helper()
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

func assertScopedCancellationJoined(t *testing.T, ctx context.Context, done <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-done:
	case <-ctx.Done():
		t.Errorf("%s cancellation did not join", label)
	}
}

func assertIndependentScopedControlEffects(t *testing.T, a, b *scopedControlRuntime, aMetrics, bMetrics *scopeControlMetrics, request factorysessions.ControlRequest) {
	t.Helper()
	if a.controls != 1 || b.controls != 1 || a.last.ControlID != request.RequestID || b.last.ControlID != request.RequestID || len(aMetrics.outcomes) != 1 || len(bMetrics.outcomes) != 1 {
		t.Fatalf("independent correlation/effects: A=%#v B=%#v metrics=%v/%v", a.last, b.last, aMetrics.outcomes, bMetrics.outcomes)
	}
}

type scopedControlCompletion struct {
	result factorysessions.LifecycleControlResult
	err    error
}

func assertScopedControlCompletion(t *testing.T, completed scopedControlCompletion, id string) {
	t.Helper()
	if completed.err != nil || completed.result.SessionID != id || completed.result.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("session %s cancel = %#v, %v", id, completed.result, completed.err)
	}
}

// The operator-authorized F04 compound observation belongs at the cleanup
// owner: controlled Runtime effects hold Work while real registration and
// release code exercise the reused-generation fence. Public replacement and
// response-cursor journeys remain in the functional suite.
func TestRegisterOpeningRepeatedStaleReleaseDuringReplacementAndPeerWork(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state := newWorkResolverSessionState()
	clock := projectionClockStub{now: time.Unix(1234, 0)}
	stores := map[string]*responseeventstore.SessionResponseEventStore{}
	for _, id := range []string{"candidate", "peer"} {
		stores[id] = responseeventstore.NewSessionResponseEventStore(id, clock, func() string { return "event" })
	}
	assembly := &Assembly{state: state, registry: state.Registry(), scopeActivation: NewScopeActivation(state),
		sessionIDs: func() string { return "generated" }, eventIDs: func() string { return "event" },
		responseStreams: staleReleaseResponseStreams{stores: stores},
	}
	old, _, release := registerStaleReleaseWork(t, assembly, "candidate", "old", nil, clock)
	oldResult, err := old.SubmitWorkRequestForSession(ctx, "candidate", work.WorkRequest{RequestID: "old-work", Works: []work.Work{{WorkID: "old-work"}}})
	if err != nil || oldResult.RequestID != "old-work" || oldResult.WorkID != "old-work" || oldResult.TraceID != "old" {
		t.Fatalf("old Work = %+v, %v", oldResult, err)
	}
	proceed := make(chan struct{})
	var unblock sync.Once
	newRuntime, newEffect, _ := registerStaleReleaseWork(t, assembly, "candidate", "new", proceed, clock)
	peerRuntime, peerEffect, _ := registerStaleReleaseWork(t, assembly, "peer", "peer", proceed, clock)
	var joined sync.WaitGroup
	defer joined.Wait()
	// Unblock before joining even when an assertion exits early.
	defer unblock.Do(func() { close(proceed) })
	results := make(chan staleReleaseWorkResult, 2)
	startStaleReleaseWork(ctx, &joined, results, newRuntime, "candidate", "new-work")
	startStaleReleaseWork(ctx, &joined, results, peerRuntime, "peer", "peer-work")
	for _, effect := range []*staleReleaseWorkEffect{newEffect, peerEffect} {
		select {
		case <-effect.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for sequence := int64(1); sequence <= 2; sequence++ {
		if err := release(ctx); err != nil {
			t.Fatal(err)
		}
		assertStaleReleaseObservations(t, ctx, assembly, stores, clock, sequence)
	}
	unblock.Do(func() { close(proceed) })
	joined.Wait()
	close(results)
	assertStaleReleaseWorkResults(t, results)
	assertStaleReleaseObservations(t, ctx, assembly, stores, clock, 3)
}

func assertStaleReleaseWorkResults(t *testing.T, results <-chan staleReleaseWorkResult) {
	t.Helper()
	count := 0
	for outcome := range results {
		count++
		if outcome.err != nil || !outcome.result.Accepted || outcome.result.RequestID != outcome.requestID ||
			outcome.result.WorkID != outcome.requestID ||
			outcome.result.TraceID != map[string]string{"new-work": "new", "peer-work": "peer"}[outcome.requestID] {
			t.Fatalf("continuing Work = %+v", outcome)
		}
	}
	if count != 2 {
		t.Fatalf("completed Work requests = %d, want replacement and peer", count)
	}
}

type staleReleaseResponseStreams struct {
	responsestreamservice.Service
	stores map[string]*responseeventstore.SessionResponseEventStore
}

func (streams staleReleaseResponseStreams) NewEventStore(id string, _ factoryruntime.Clock) (*responseeventstore.SessionResponseEventStore, error) {
	return streams.stores[id], nil
}
func (streams staleReleaseResponseStreams) Complete(store *responseeventstore.SessionResponseEventStore) {
	store.Complete()
}

type staleReleaseWorkRecord struct {
	registrationRuntimeRecord
	effect *staleReleaseWorkEffect
}

func (record staleReleaseWorkRecord) RuntimeService() factoryruntime.Service { return record.effect }

type staleReleaseWorkEffect struct {
	submitWorkFactory
	name    string
	entered chan struct{}
	proceed <-chan struct{}
}

func (effect *staleReleaseWorkEffect) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if len(request.Works) != 1 || request.Works[0].WorkID != request.RequestID {
		return work.WorkRequestSubmitResult{}, errors.New("one identified Work is required")
	}
	if effect.proceed != nil {
		close(effect.entered)
		select {
		case <-effect.proceed:
		case <-ctx.Done():
			return work.WorkRequestSubmitResult{}, ctx.Err()
		}
	}
	return work.WorkRequestSubmitResult{RequestID: request.RequestID, WorkID: request.Works[0].WorkID, TraceID: effect.name, Accepted: true}, nil
}
func (effect *staleReleaseWorkEffect) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{Observation: factoryruntime.Observation{Health: factoryruntime.ObservationHealth{FactoryState: effect.name}}}, nil
}

func registerStaleReleaseWork(t *testing.T, assembly *Assembly, id, name string, proceed <-chan struct{}, clock factoryruntime.Clock) (*SessionRuntime, *staleReleaseWorkEffect, func(context.Context) error) {
	t.Helper()
	effect := &staleReleaseWorkEffect{name: name, entered: make(chan struct{}), proceed: proceed}
	facts := roles.SessionOpeningFacts{FactorySessionID: id, RuntimeID: name, GenerationID: "reused-generation"}
	record := staleReleaseWorkRecord{effect: effect}
	runtime, selected, _, _, release, err := assembly.RegisterOpening(context.Background(), facts, record, factoryruntime.RuntimeInitialCompletion{}, nil, nil, nil, clock, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	// A controlled acquired run supplies the same handle boundary as startup.
	runtimebinding.SessionStateFrom(selected).Handle = invocationQueryRun{record: record}
	if err := runtime.(*SessionRuntime).BindRuntime(id, factoryruntime.RuntimeBinding{}.New(name, effect)); err != nil {
		t.Fatal(err)
	}
	return runtime.(*SessionRuntime), effect, release
}

type staleReleaseWorkResult struct {
	requestID string
	result    work.WorkRequestSubmitResult
	err       error
}

func startStaleReleaseWork(ctx context.Context, joined *sync.WaitGroup, results chan<- staleReleaseWorkResult, runtime *SessionRuntime, id, requestID string) {
	joined.Add(1)
	go func() {
		defer joined.Done()
		result, err := runtime.SubmitWorkRequestForSession(ctx, id, work.WorkRequest{RequestID: requestID, Works: []work.Work{{WorkID: requestID}}})
		results <- staleReleaseWorkResult{requestID: requestID, result: result, err: err}
	}()
}

func assertStaleReleaseObservations(t *testing.T, ctx context.Context, assembly *Assembly, stores map[string]*responseeventstore.SessionResponseEventStore, clock factoryruntime.Clock, sequence int64) {
	t.Helper()
	for id, name := range map[string]string{"candidate": "new", "peer": "peer"} {
		observation, err := assembly.ObserveForSession(ctx, id, factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeHealth})
		if err != nil || observation.Observation.Health.FactoryState != name {
			t.Fatalf("%s observation = %+v, %v", id, observation, err)
		}
		selected := assembly.Resolve(id)
		if selected == nil || selected.ResponseEvents != stores[id] {
			t.Fatalf("%s lost selected response history", id)
		}
		event, err := selected.ResponseEvents.Publish(responseevents.FactoryResponseEvent{
			RunID: name, Kind: responseevents.KindMessage, Phase: responseevents.PhaseDelta,
			Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"continuing"}`),
		})
		if err != nil || event.FactorySessionID != id || event.Sequence != sequence || !event.RecordedAt.Equal(clock.Now()) {
			t.Fatalf("%s continuing response = %+v, %v", id, event, err)
		}
	}
}

type registrationRuntimeConfig struct {
	interfaces.LoadedFactorySource
}

func (registrationRuntimeConfig) RuntimeBaseDir() string { return "/factory" }

type registrationRuntimeRecord struct{ factoryruntime.RuntimeRecord }

func (registrationRuntimeRecord) LoadedRuntimeConfig() factoryruntime.LoadedConfig {
	return registrationRuntimeConfig{}
}
func (registrationRuntimeRecord) Directory() string                                      { return "/factory" }
func (registrationRuntimeRecord) FolderDirectory() string                                { return "/factory" }
func (registrationRuntimeRecord) BackendScope() string                                   { return "backend" }
func (registrationRuntimeRecord) RuntimeService() factoryruntime.Service                 { return nil }
func (registrationRuntimeRecord) RuntimeLogger() *zap.Logger                             { return nil }
func (registrationRuntimeRecord) RecordingLedger() recordings.Ledger                     { return nil }
func (registrationRuntimeRecord) AddEventTypeRecorder(func(interfaces.FactoryEventType)) {}

type registrationResponseStreams struct {
	responsestreamservice.Service
	open     func() (*responseeventstore.SessionResponseEventStore, error)
	complete func(*responseeventstore.SessionResponseEventStore)
}

func (streams registrationResponseStreams) NewEventStore(string, factoryruntime.Clock) (*responseeventstore.SessionResponseEventStore, error) {
	if streams.open != nil {
		return streams.open()
	}
	return nil, nil
}

func (streams registrationResponseStreams) Complete(store *responseeventstore.SessionResponseEventStore) {
	if streams.complete != nil {
		streams.complete(store)
	}
}

type registrationActivation struct {
	SessionScopeActivation
	retire func(context.Context, SessionScope) error
}

func (activation registrationActivation) Retire(ctx context.Context, scope SessionScope) error {
	return activation.retire(ctx, scope)
}

func TestRegisterOpeningFailureCancellationAndRetryPreservePeer(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"failure", "partial", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			state := newWorkResolverSessionState()
			peer := &livesession.LiveSession{ID: "peer"}
			state.Registry().Upsert(peer, true)
			cause := errors.New("response registration failed")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failed := true
			completions := 0
			store := &responseeventstore.SessionResponseEventStore{}
			assembly := &Assembly{state: state, registry: state.Registry(), invoker: registrationInvoker{result: factorysessions.ResolvedInvocationInput{Source: "registered"}},
				sessionIDs: func() string { return "candidate" }, eventIDs: func() string { return "event" },
				responseStreams: registrationResponseStreams{open: func() (*responseeventstore.SessionResponseEventStore, error) {
					if failed && mode == "failure" {
						return nil, cause
					}
					if failed && mode == "partial" {
						return store, cause
					}
					if failed {
						cancel()
					}
					return store, nil
				}, complete: func(got *responseeventstore.SessionResponseEventStore) {
					if got != store {
						t.Fatal("released peer response store")
					}
					completions++
				}},
				scopeActivation: registrationActivation{retire: func(_ context.Context, scope SessionScope) error {
					state.UnregisterGeneration(scope.Session)
					return nil
				}},
			}
			facts := roles.SessionOpeningFacts{FactorySessionID: "candidate"}
			initial := &factoryruntime.RuntimeInitialOpening{Record: registrationRuntimeRecord{}}
			_, _, _, _, release, err := assembly.RegisterOpening(ctx, facts, initial.Record, initial.Completion, initial.ReplacementBuilder, initial.Lifecycle, initial.Sidecars, platformclock.Real{}, zap.NewNop())
			expected := map[string]error{"failure": cause, "partial": cause, "cancel": context.Canceled}[mode]
			if !errors.Is(err, expected) || state.Resolve("candidate") != nil || state.Current() != peer {
				t.Fatalf("failed registration = %v; candidate or selection changed", err)
			}
			if release != nil {
				if err := release(context.WithoutCancel(ctx)); err != nil {
					t.Fatal(err)
				}
				if err := release(context.WithoutCancel(ctx)); err != nil || completions != 1 {
					t.Fatalf("repeated release = %v, completions = %d", err, completions)
				}
			}
			failed = false
			assertRegistrationRetryPreservesPeer(t, assembly, facts, initial, peer)

		})
	}
}

func assertRegistrationRetryPreservesPeer(t *testing.T, assembly *Assembly, facts roles.SessionOpeningFacts, initial *factoryruntime.RuntimeInitialOpening, peer *livesession.LiveSession) {
	t.Helper()
	_, selected, _, _, release, err := assembly.RegisterOpening(context.Background(), facts, initial.Record, initial.Completion, initial.ReplacementBuilder, initial.Lifecycle, initial.Sidecars, platformclock.Real{}, zap.NewNop())
	if err != nil || selected == nil || assembly.state.Resolve("candidate") != selected || assembly.state.Resolve("peer") != peer {
		t.Fatalf("same-ID retry = %v", err)
	}
	if err := release(context.Background()); err != nil || assembly.state.Resolve("candidate") != nil || assembly.state.Resolve("peer") != peer {
		t.Fatalf("retry release = %v", err)
	}
}

func TestRegisterOpeningReleaseRetriesAndPreservesReplacementHistory(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	store := &responseeventstore.SessionResponseEventStore{}
	cause := errors.New("registration retirement failed")
	retireErr := cause
	completions := 0
	assembly := &Assembly{state: state, registry: state.Registry(), invoker: registrationInvoker{result: factorysessions.ResolvedInvocationInput{Source: "registered"}},
		sessionIDs: func() string { return "candidate" }, eventIDs: func() string { return "event" },
		responseStreams: registrationResponseStreams{open: func() (*responseeventstore.SessionResponseEventStore, error) { return store, nil },
			complete: func(*responseeventstore.SessionResponseEventStore) { completions++ }},
		scopeActivation: registrationActivation{retire: func(_ context.Context, scope SessionScope) error {
			if retireErr != nil {
				return retireErr
			}
			state.UnregisterGeneration(scope.Session)
			return nil
		}},
	}
	facts := roles.SessionOpeningFacts{FactorySessionID: "candidate", RuntimeID: "runtime", GenerationID: "reused-generation"}
	_, _, _, _, release, err := assembly.RegisterOpening(context.Background(), facts, registrationRuntimeRecord{}, factoryruntime.RuntimeInitialCompletion{}, nil, nil, nil, platformclock.Real{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	previous := state.Resolve("candidate")
	if err := release(context.Background()); !errors.Is(err, cause) || state.Resolve("candidate") != previous || completions != 0 {
		t.Fatalf("failed release = %v", err)
	}
	replacement := &livesession.LiveSession{ID: previous.ID, ResponseEvents: store}
	state.Registry().Upsert(replacement, true)
	retireErr = nil
	if err := release(context.Background()); err != nil || state.Resolve("candidate") != replacement || completions != 0 {
		t.Fatalf("stale release = %v, completions = %d", err, completions)
	}
	if err := release(context.Background()); err != nil || completions != 0 {
		t.Fatalf("repeated stale release = %v", err)
	}
}

type registrationInvoker struct {
	roles.InvocationService
	result factorysessions.ResolvedInvocationInput
	err    error
}

func (r registrationInvoker) ResolveInvocationInput(_ *interfaces.FactoryConfig, _ factorysessions.InvocationRequest) (factorysessions.ResolvedInvocationInput, error) {
	return r.result, r.err
}

type registrationObserver struct {
	sessionregistry.Service
	publish func(*livesession.LiveSession)
}

func (r registrationObserver) Upsert(session *livesession.LiveSession, _ bool) { r.publish(session) }

func TestRegisterOpeningPublishesSessionWithFixedInputResolver(t *testing.T) {
	t.Parallel()
	publications := 0
	var assembly *Assembly
	assembly = &Assembly{
		state: &sessionruntime.Service{}, invoker: registrationInvoker{result: factorysessions.ResolvedInvocationInput{Source: "registered"}},
		responseStreams: registrationResponseStreams{},
		sessionIDs:      func() string { return "session" }, eventIDs: func() string { return "event" },
		resolveHome:         func() (string, error) { return "/home", nil },
		directoryInspection: scaffoldDirectories{}, namedPaths: scaffoldNamedPaths{},
		initialWorkFiles:        fileeffects.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
		sessionResultProjection: &canonicalInspectionResultProjectionFake{}, identity: scopedIdentityStub{},
		registry: registrationObserver{publish: func(session *livesession.LiveSession) {
			publications++
			bound := runtimebinding.SessionStateFrom(session)
			if bound == nil {
				t.Fatal("session published without its scoped runtime state")
			}
			got, err := assembly.ResolveInvocationInput(nil, factorysessions.InvocationRequest{})
			if err != nil || got.Source != "registered" {
				t.Fatalf("published input = %+v, %v", got, err)
			}
		}},
	}
	_, _, _, _, _, err := assembly.RegisterOpening(context.Background(), roles.SessionOpeningFacts{FactorySessionID: "session", FactoryRootDir: "/factory", Directory: "/factory", ExecutionBaseDir: "/factory", BackendScopeID: "backend"}, registrationRuntimeRecord{}, factoryruntime.RuntimeInitialCompletion{}, nil, nil, nil, platformclock.Real{}, zap.NewNop())
	if err != nil || publications != 1 {
		t.Fatalf("RegisterOpening = %v, publications = %d", err, publications)
	}
}

type scaffoldNamedPaths struct {
	interfaces.NamedPathResolver
}

func (scaffoldNamedPaths) ResolveCurrentDir(root string) (string, error) {
	return filepath.Join(root, "current"), nil
}
