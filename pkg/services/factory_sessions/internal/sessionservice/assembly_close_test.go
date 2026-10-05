package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	platformlogging "github.com/portpowered/infinite-you/pkg/platform/logging"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	sessionidentity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"go.uber.org/goleak"
	"go.uber.org/zap"
)

type closingDurableOwner struct {
	durableexecution.Service
	closed int
	err    error
}

func (owner *closingDurableOwner) Close() error {
	owner.closed++
	return owner.err
}

func TestAssemblyCloseDrainsProcessDurableOwner(t *testing.T) {
	failure := errors.New("durable shutdown failed")
	owner := &closingDurableOwner{err: failure}
	assembly := NewAssembly(&Service{durable: owner}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, owner, nil, nil).(*Assembly)
	if err := assembly.Close(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Close error = %v, want %v", err, failure)
	}
	if owner.closed != 1 {
		t.Fatalf("durable owner close calls = %d, want one", owner.closed)
	}
}

type closeRegistryOwnerFake struct {
	registry sessionregistry.Service
	order    *[]string
	failID   string
	failErr  error
}

func (owner *closeRegistryOwnerFake) BuildSessionProjectionContext(_ context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	return factorysessions.ProjectionContext{FactorySessionID: session.ID}, nil
}

func (owner *closeRegistryOwnerFake) PrepareOwnedSessionClose(_ context.Context, session *livesession.LiveSession) error {
	sessionID := session.ID
	*owner.order = append(*owner.order, sessionID)
	if sessionID == owner.failID {
		return owner.failErr
	}
	return nil
}

func (owner *closeRegistryOwnerFake) RetireOwnedSession(_ context.Context, session *livesession.LiveSession) error {
	owner.registry.RemoveGeneration(session)
	return nil
}

func TestAssemblyCloseDrainsCanonicalRegistryAndRetainsFailedSession(t *testing.T) {
	state := newWorkResolverSessionState()
	registry := state.Registry()
	var order []string
	failure := errors.New("close first session")
	owner := &closeRegistryOwnerFake{registry: registry, order: &order, failID: "session-first", failErr: failure}
	for _, id := range []string{"session-first", "session-second", "session-third"} {
		registry.Upsert(&livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{Owner: owner}}, false)
	}
	assembly := &Assembly{state: state, registry: registry}
	if err := assembly.Close(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Close error = %v, want %v", err, failure)
	}
	if !reflect.DeepEqual(order, []string{"session-third", "session-second", "session-first"}) {
		t.Fatalf("shutdown order = %v", order)
	}
	if registry.Get("session-first") == nil || registry.Get("session-second") != nil || registry.Get("session-third") != nil {
		t.Fatalf("shutdown lost retryable failure or retained successful sessions: %v", registry.IDs())
	}
	owner.failID = ""
	if err := assembly.Close(context.Background()); err != nil || registry.Count() != 0 {
		t.Fatalf("retry close error = %v, remaining sessions = %v", err, registry.IDs())
	}
}

func TestRetireOwnedSessionUsesScopedStopAndRetainsFailedCleanup(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	registerScopeControlRuntime(state, "a", &scopedControlRuntime{status: "RUNNING"}, zap.NewNop())
	registerScopeControlRuntime(state, "b", &scopedControlRuntime{status: "RUNNING"}, zap.NewNop())
	selected := runtimebinding.SessionStateFrom(state.Resolve("a"))
	session := state.Resolve("a")
	selected.Clock = state.Clock()
	failure := errors.New("injected stop failure")
	stopErr := failure
	control := NewScopeControl(state, func(run factoryruntime.RuntimeRun, clock factoryruntime.Clock) error {
		if run != selected.Handle || clock != selected.Clock {
			t.Fatal("retirement stopped a foreign runtime or selected clock")
		}
		return stopErr
	}, zap.NewNop())
	owner := &SessionRuntime{sessionState: state, scopeControl: control, scopeActivation: NewScopeActivation(state)}
	if err := owner.RetireOwnedSession(context.Background(), session); !errors.Is(err, failure) {
		t.Fatalf("failed retirement = %v", err)
	}
	if state.Resolve("a") == nil || state.Resolve("b") == nil {
		t.Fatal("failed cleanup lost the retryable session or its peer")
	}
	stopErr = nil
	if err := owner.RetireOwnedSession(context.Background(), session); err != nil {
		t.Fatalf("retry retirement = %v", err)
	}
	if state.Resolve("a") != nil || state.Resolve("b") == nil {
		t.Fatal("successful retirement did not remove only A")
	}
}

func TestRetireOwnedSessionRetainsPartiallyActivatedScopeForCleanupRetry(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	activation := NewScopeActivation(state)
	registerScopeControlRuntime(state, "a", &scopedControlRuntime{status: "RUNNING"}, zap.NewNop())
	registerScopeControlRuntime(state, "b", &scopedControlRuntime{status: "RUNNING"}, zap.NewNop())
	session := state.Resolve("a")
	bound := runtimebinding.SessionStateFrom(session)
	bound.Clock = state.Clock()
	failure := errors.New("partial activation cleanup failed")
	cleanupErr := failure
	stopped := false
	binding := factoryruntime.RuntimeBinding{}.New("a-runtime", session.Runtime.Factory,
		func(context.Context) (factoryruntime.RuntimeDeactivationResult, error) {
			if !stopped {
				t.Fatal("deactivation ran before owned runtime stop returned")
			}
			return factoryruntime.RuntimeDeactivationResult{}, cleanupErr
		})
	if err := activation.Activate(context.Background(), SessionScope{Session: session, Binding: binding}); err != nil {
		t.Fatal(err)
	}
	control := NewScopeControl(state, func(run factoryruntime.RuntimeRun, clock factoryruntime.Clock) error {
		if run != bound.Handle || clock != bound.Clock {
			t.Fatal("partial activation cleanup stopped a foreign run")
		}
		stopped = true
		return nil
	}, zap.NewNop())
	owner := &SessionRuntime{sessionState: state, scopeControl: control, scopeActivation: activation}
	if err := owner.RetireOwnedSession(context.Background(), session); !errors.Is(err, failure) {
		t.Fatalf("partial activation cleanup = %v, want original cause", err)
	}
	if state.Resolve("a") != session || state.Resolve("b") == nil {
		t.Fatal("failed activation cleanup lost its retryable scope or peer")
	}
	cleanupErr = nil
	if err := owner.RetireOwnedSession(context.Background(), session); err != nil {
		t.Fatalf("cleanup retry = %v", err)
	}
	if state.Resolve("a") != nil || state.Resolve("b") == nil {
		t.Fatal("cleanup retry did not retire only A")
	}
}

type stubRuntimeSidecars struct {
	preseedCalls int
	startCalls   int
	stopCalls    int
}

type closedScopeRun struct {
	invocationQueryRun
	done chan struct{}
}

func (r closedScopeRun) RunDoneCh() <-chan struct{} { return r.done }

type closeScopeActivation func(context.Context) error

func (f closeScopeActivation) Close(ctx context.Context) error { return f(ctx) }

func TestAssemblyCloseKeepsCapturedGenerationAcrossActivationReplacement(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	oldRuntime := &scopedControlRuntime{status: "RUNNING"}
	registerScopeControlRuntime(state, "a", oldRuntime, zap.NewNop())
	registerScopeControlRuntime(state, "b", &scopedControlRuntime{status: "RUNNING"}, zap.NewNop())
	oldSession := state.Resolve("a")
	oldBound := runtimebinding.SessionStateFrom(oldSession)
	oldRecord := runtimebinding.BundleFromSession(oldSession)
	done := make(chan struct{})
	close(done)
	oldBound.Handle = closedScopeRun{invocationQueryRun: invocationQueryRun{record: oldRecord}, done: done}
	oldBound.Clock = state.Clock()
	var replacement *livesession.LiveSession
	replacementRuntime := &scopedControlRuntime{status: "RUNNING"}
	var stopped, retired bool
	control := NewScopeControl(state, func(run factoryruntime.RuntimeRun, clock factoryruntime.Clock) error {
		if run != oldBound.Handle || clock != oldBound.Clock {
			t.Fatal("close stopped replacement run or used its clock")
		}
		stopped = true
		return nil
	}, zap.NewNop())
	owner := &SessionRuntime{sessionState: state, scopeControl: control, scopeActivation: NewScopeActivation(state),
		retireWorkAdmissionProjection: func(id string, runtime *factorysessions.LiveRuntime, record factoryruntime.RuntimeRecord) {
			if id != "a" || runtime != oldSession.Runtime || record != oldRecord {
				t.Fatal("close retired a foreign Work projection generation")
			}
			retired = true
		},
	}
	oldBound.Owner = owner
	oldBound.Activation = closeScopeActivation(func(context.Context) error {
		registerScopeControlRuntime(state, "a", replacementRuntime, zap.NewNop())
		replacement = state.Resolve("a")
		return nil
	})
	assembly := &Assembly{state: state, registry: state.Registry()}
	if err := assembly.CloseSession(context.Background(), "a"); err != nil {
		t.Fatalf("CloseSession = %v", err)
	}
	if !stopped || !retired || oldRuntime.controls != 1 || state.Resolve("a") != replacement || state.Resolve("b") == nil {
		t.Fatal("close lost captured effects, replacement or peer")
	}
	assertReplacementControlAfterClose(t, control, replacementRuntime, oldRuntime)

}

func (s *stubRuntimeSidecars) Preseed(context.Context, factoryruntime.RuntimeRecord) error {
	s.preseedCalls++
	return nil
}

func (s *stubRuntimeSidecars) Start(context.Context, factoryruntime.RuntimeRun) error {
	s.startCalls++
	return nil
}

func (s *stubRuntimeSidecars) Stop(factoryruntime.RuntimeRun) {
	s.stopCalls++
}

type stubRuntimeLifecycle struct {
	stopSidecarsCalls int
}

func (s *stubRuntimeLifecycle) Start(context.Context, factoryruntime.RuntimeRecord) (factoryruntime.RuntimeRun, error) {
	return nil, nil
}

func (s *stubRuntimeLifecycle) WaitForStart(context.Context, factoryruntime.RuntimeRun) error {
	return nil
}

func (s *stubRuntimeLifecycle) Stop(factoryruntime.RuntimeRun) error {
	return nil
}

func (s *stubRuntimeLifecycle) StopSidecars(factoryruntime.RuntimeRun) {
	s.stopSidecarsCalls++
}

func (s *stubRuntimeLifecycle) PublishReplacement(context.Context, factoryruntime.RuntimeRun, factoryruntime.RuntimeRecord) error {
	return nil
}

func TestStopLiveRuntimeSidecars_UsesInjectedSidecarsExactlyOnce(t *testing.T) {
	t.Parallel()

	sidecars := &stubRuntimeSidecars{}
	runtime := &SessionRuntime{runtimeSidecars: sidecars}

	runtime.StopLiveRuntimeSidecars(nil)

	if sidecars.stopCalls != 1 {
		t.Fatalf("runtimeSidecars.Stop calls = %d, want 1", sidecars.stopCalls)
	}
}

func TestStopLiveRuntimeSidecars_MissingSidecarsSkipsLifecycleFallback(t *testing.T) {
	t.Parallel()

	lifecycle := &stubRuntimeLifecycle{}
	runtime := &SessionRuntime{runtimeLifecycle: lifecycle}

	runtime.StopLiveRuntimeSidecars(nil)

	if lifecycle.stopSidecarsCalls != 0 {
		t.Fatalf("runtimeLifecycle.StopSidecars calls = %d, want 0", lifecycle.stopSidecarsCalls)
	}
}

// TestMain fails the package when a test leaves goroutines running, which
// otherwise surfaces as teardown hangs and cross-test interference.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type suppliedStreamFactories struct {
	progressScope, completionScope, dispatch, payload string
	logger                                            *zap.Logger
}

func (s *suppliedStreamFactories) InferenceProgressPublisherFactory(logger *zap.Logger) func(string) factorysessions.ProgressPublisher {
	s.logger = logger
	return func(id string) factorysessions.ProgressPublisher {
		s.progressScope = id
		return func(fragment factorysessions.ProgressFragment) { s.payload = fragment.Payload }
	}
}
func (s *suppliedStreamFactories) DispatchCompletionObserverFactory() func(string) func(string) {
	return func(id string) func(string) {
		s.completionScope = id
		return func(dispatch string) { s.dispatch = dispatch }
	}
}
func TestAssemblyUsesInjectedAuthorityAndStreamFactories(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	state.Register(sessionruntime.Registration{SessionID: "supplied", Handle: struct{}{}})
	streams := &suppliedStreamFactories{}
	assembly := NewAssembly(nil, state.Registry(), state, streams, nil, nil, nil, nil, nil, state.Clock(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, NewNamedFactoryActivator(state), NewKeyedDefinitionActivationGateway(state, nil)).(*Assembly)
	if assembly.Resolve("supplied") != state.Resolve("supplied") {
		t.Fatal("assembly replaced supplied authority")
	}
	logger := zap.NewNop()
	assembly.InferenceProgressPublisherFactory(logger)("supplied")(factorysessions.ProgressFragment{Payload: "owned-output"})
	assembly.DispatchCompletionObserverFactory()("supplied")("owned-dispatch")
	if streams.logger != logger || streams.progressScope != "supplied" || streams.completionScope != "supplied" || streams.payload != "owned-output" || streams.dispatch != "owned-dispatch" {
		t.Fatalf("forwarded factories = %#v", streams)
	}
	state.Unregister("supplied")
	if assembly.Resolve("supplied") != nil {
		t.Fatal("assembly retained retired authority entry")
	}
}

func assertReplacementControlAfterClose(t *testing.T, control SessionScopeControl, replacementRuntime, oldRuntime *scopedControlRuntime) {
	t.Helper()
	result, err := control.CancelLiveFactorySession(context.Background(), "a", factorysessions.ControlRequest{RequestID: "replacement-cancel"})
	if err != nil || result.Outcome != factorysessions.LifecycleControlOutcomeAccepted || replacementRuntime.controls != 1 || oldRuntime.controls != 1 {
		t.Fatalf("replacement control after close = %+v, %v", result, err)
	}
}

// The real rolling sink rejects writes after close. Capturing zap's error
// destination exposes the quiet-stderr regression that an observer alone misses.
type stopLogPath struct {
	platformartifact.Reserver
	path string
}

func (p stopLogPath) Reserve(string, time.Time, string, string) (string, error) { return p.path, nil }

func TestScopeStopDiagnosticsOutliveRuntimeLogSink(t *testing.T) {
	t.Parallel()
	for _, closeBefore := range []bool{false, true} {
		t.Run(fmt.Sprintf("closedBeforeStop=%t", closeBefore), func(t *testing.T) {
			t.Parallel()
			for _, failure := range []error{nil, errors.New("owned stop failed after sink close")} {
				assertStopLogSinkLifetime(t, closeBefore, failure)
			}
		})
	}
}
func assertStopLogSinkLifetime(t *testing.T, closeBefore bool, failure error) {
	t.Helper()
	state := newWorkResolverSessionState()
	core, diagnostics := observer.New(zap.InfoLevel)
	var writeErrors bytes.Buffer
	processLogger := zap.New(core, zap.ErrorOutput(zapcore.AddSync(&writeErrors)))
	path := filepath.Join(t.TempDir(), "runtime.log")
	opener, err := platformlogging.NewRuntimeLogOpener(stopLogPath{path: path})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := opener.Open(platformlogging.RuntimeLogOpeningRequest{BaseLogger: processLogger, RuntimeInstanceID: "owned-runtime", RootDirectory: filepath.Dir(path), StartTimeUTC: time.Unix(1, 0), CollisionID: "owned-log"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	sink.Logger().Info("runtime log opened")
	registerScopeControlRuntime(state, "owned", &scopedControlRuntime{status: "RUNNING"}, sink.Logger())
	if closeBefore {
		if err := sink.Close(); err != nil {
			t.Fatal(err)
		}
	}
	control := NewScopeControl(state, func(factoryruntime.RuntimeRun, factoryruntime.Clock) error {
		if err := sink.Close(); err != nil {
			t.Fatal(err)
		}
		return failure
	}, processLogger)
	err = control.StopLiveSession(t.Context(), "owned")
	if !errors.Is(err, failure) {
		t.Fatalf("stop error = %v, want %v", err, failure)
	}
	if writeErrors.Len() != 0 {
		t.Fatalf("write through closed runtime sink: %s", writeErrors.String())
	}
	terminal := "live Factory Session runtime stopped"
	if failure != nil {
		terminal = "stop live Factory Session runtime failed"
	}
	entries := diagnostics.FilterMessage(terminal).All()
	if len(entries) != 1 {
		t.Fatalf("terminal diagnostics = %#v", entries)
	}
	if failure != nil && entries[0].ContextMap()["error"] != failure.Error() {
		t.Fatalf("lost stop failure diagnostic: %#v", entries[0])
	}
	if entries[0].ContextMap()["session_id"] != "owned" {
		t.Fatalf("missing shutdown correlation: %#v", entries[0])
	}
	if diagnostics.FilterMessage("stopping live Factory Session runtime").Len() != 1 {
		t.Fatal("missing stop intent")
	}
}

func TestGatewayInjectedStreamsPreserveRetainedOutputCompletionAndPeers(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	for _, id := range []string{"selected", "peer"} {
		state.Registry().Upsert(&livesession.LiveSession{ID: id, Handle: struct{}{}}, id == "selected")
	}
	// The supplied stream owner has its own registry. Reads must use that owner,
	// including output already retained before gateway construction.
	registry := newWorkResolverSessionState().ResponseStreams()
	streams := stream.NewManagerWithDependencies(state, sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle), registry)
	fragment := func(payload string) factorysessions.ProgressFragment {
		return factorysessions.ProgressFragment{DispatchID: "dispatch", Kind: factorysessions.ResponseFragmentKind, Type: "TEXT_DELTA", Payload: payload}
	}
	streams.InferenceProgressPublisherFactory(nil)("selected")(fragment("retained"))
	host := SessionServiceHost(state, nil, nil, nil, "", nil, nil, nil, nil, nil)
	gateway := NewWithLiveChangeCoordinator(host, streams, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	gateway.InferenceProgressPublisherFactory(nil)("selected")(fragment("selected-output"))
	gateway.InferenceProgressPublisherFactory(nil)("peer")(fragment("peer-output"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	selected, err := gateway.SubscribeSessionResponseStream("selected", "dispatch", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Detach()
	assertGatewayStreamOutput(t, ctx, selected, 1, []string{"retained", "selected-output"})
	gateway.DispatchCompletionObserverFactory()("selected")("dispatch")
	if _, err := selected.Next(ctx); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("completed selected stream = %v", err)
	}
	peer, err := gateway.SubscribeSessionResponseStream("peer", "dispatch", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Detach()
	assertGatewayStreamOutput(t, ctx, peer, 1, []string{"peer-output"})
	gateway.InferenceProgressPublisherFactory(nil)("peer")(fragment("peer-after-completion"))
	assertGatewayStreamOutput(t, ctx, peer, 2, []string{"peer-after-completion"})
	gateway.DispatchCompletionObserverFactory()("peer")("dispatch")
}

func assertGatewayStreamOutput(t *testing.T, ctx context.Context, subscription *responsestream.Subscription, firstSequence int64, payloads []string) {
	t.Helper()
	read, err := subscription.Next(ctx)
	if err != nil || len(read.Events) != len(payloads) {
		t.Fatalf("retained output = %#v, %v", read, err)
	}
	for i, payload := range payloads {
		if event := read.Events[i]; event.Payload != payload || event.Sequence != firstSequence+int64(i) {
			t.Fatalf("retained event %d = %#v, want sequence %d payload %q", i, event, firstSequence+int64(i), payload)
		}
	}
}

func TestAssemblyInjectedProjectionUsesSelectedFactsAndKeepsPeersReadable(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	processTime := time.Date(2026, 10, 5, 1, 0, 0, 0, time.UTC)
	selectedTime := processTime.Add(time.Hour)
	reader := SessionServiceHost(state, nil, nil, nil, "", projectionIdentityStub{}, projectionClockStub{now: processTime}, nil, nil, nil)
	assembly := &Assembly{state: state, registry: state.Registry(), projectionReader: reader}
	for _, id := range []string{"first", "peer"} {
		registerIndependentProjectionSession(state, id)
	}
	first := state.Resolve("first")
	bound := runtimebinding.SessionStateFrom(first)
	bound.Clock = projectionClockStub{now: selectedTime}
	bound.ProjectionBackendScope = "opening-override"
	projection, err := assembly.GetFactorySession(t.Context(), first.ID)
	if err != nil || projection.Context.FactorySessionID != first.ID || projection.Context.BackendScopeID != "opening-override" || !projection.Context.Now.Equal(selectedTime) {
		t.Fatalf("selected projection = %#v, %v", projection, err)
	}
	failure := errors.New("selected observation failed")
	first.Runtime.Factory.(*observeStubRuntime).err = failure
	if _, err := assembly.GetFactorySession(t.Context(), first.ID); !errors.Is(err, failure) {
		t.Fatalf("observation cause = %v", err)
	}
	peer, err := assembly.GetFactorySession(t.Context(), "peer")
	if err != nil || peer.Context.BackendScopeID != "backend-peer" || peer.Context.LogicalSessionKeyID != "logical-peer" || !peer.Context.Now.Equal(processTime) {
		t.Fatalf("peer projection after failure = %#v, %v", peer, err)
	}
	first.Runtime.Factory.(*observeStubRuntime).err = nil
	bound.ProjectionBackendScope = ""
	projection, err = assembly.GetFactorySession(t.Context(), first.ID)
	if err != nil || projection.Context.BackendScopeID != "backend-first" {
		t.Fatalf("retry projection = %#v, %v", projection, err)
	}
	state.Registry().Remove(first.ID)
	if _, err := assembly.GetFactorySession(t.Context(), first.ID); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("removed projection = %v", err)
	}
	if _, err := reader.BuildSessionProjectionContext(t.Context(), nil); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing projection = %v", err)
	}
}

func registerIndependentProjectionSession(state *sessionruntime.Service, id string) {
	runtime := &observeStubRuntime{result: factoryruntime.ObserveResult{Observation: factoryruntime.Observation{Health: factoryruntime.ObservationHealth{StreamGenerationID: "generation-" + id}}}}
	state.Registry().Upsert(&livesession.LiveSession{
		ID: id, SessionState: livesession.SessionState{FolderPath: id},
		Handle:  &runtimebinding.SessionState{Handle: invocationQueryRun{record: &generationRuntimeRecord{service: runtime}}},
		Runtime: &factorysessions.LiveRuntime{Factory: runtime, BackendScopeID: "backend-" + id, RuntimeConfig: invocationQueryConfig{config: &factorydefinitions.FactoryConfig{}}},
	}, id == "first")
}

type gatewayActivationClock struct {
	factorydefinitions.DefinitionActivationGateway
	now time.Time
}

func (g gatewayActivationClock) SaveNow() time.Time { return g.now }

func TestGatewayInjectedRootCapabilitiesPreserveRequestsResultsAndErrors(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	host := SessionServiceHost(state, nil, nil, nil, "", nil, nil, nil, nil, nil)
	streams := stream.NewManagerWithDependencies(state, sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle), state.ResponseStreams())
	invoker := &canonicalSessionInvokerFake{result: factorydefinitions.FactoryInvocationResult{
		RequestID: "request", TraceID: "trace", SessionID: "selected", WorkID: "work",
		Status: factorydefinitions.InvocationTerminalStatusCompleted,
	}}
	selectedTime := time.Date(2026, 10, 5, 1, 2, 3, 0, time.UTC)
	failure := errors.New("selected activation failure")
	var activated string
	var activationErr error
	ctx := context.Background()
	gateway := NewWithLiveChangeCoordinator(host, streams, nil, nil, nil, nil, nil, nil, invoker,
		func(got context.Context, name string) error {
			if got != ctx {
				t.Fatal("activation lost request context")
			}
			activated = name
			return activationErr
		}, gatewayActivationClock{now: selectedTime})
	assertGatewayInjectedInvocation(t, ctx, gateway, invoker, failure)
	if err := gateway.ActivateNamedFactory(ctx, "first"); err != nil || activated != "first" {
		t.Fatalf("activation = %q, %v", activated, err)
	}
	activationErr = failure
	if err := gateway.ActivateNamedFactory(ctx, "second"); !errors.Is(err, failure) || activated != "second" {
		t.Fatalf("activation failure = %q, %v", activated, err)
	}
	if got := gateway.DefinitionActivationGateway().SaveNow(); !got.Equal(selectedTime) {
		t.Fatalf("selected activation time = %v", got)
	}
}

func TestGatewayOptionalRootCapabilitiesPreserveUnavailableErrors(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	host := SessionServiceHost(state, nil, nil, nil, "", nil, nil, nil, nil, nil)
	streams := stream.NewManagerWithDependencies(state, sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle), state.ResponseStreams())
	gateway := NewWithLiveChangeCoordinator(host, streams, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if _, err := gateway.InvokeFactorySession(context.Background(), "missing", factorysessions.InvocationRequest{}); err == nil || err.Error() != "Factory Session invocation service is required" {
		t.Fatalf("optional invocation = %v", err)
	}
	if err := gateway.ActivateNamedFactory(context.Background(), "missing"); err == nil || err.Error() != "Factory Session activation service is required" {
		t.Fatalf("optional activation = %v", err)
	}
	if gateway.DefinitionActivationGateway() != nil {
		t.Fatal("optional activation gateway was substituted")
	}
}

func assertGatewayInjectedInvocation(t *testing.T, ctx context.Context, gateway *Service, invoker *canonicalSessionInvokerFake, failure error) {
	t.Helper()
	requestID := "request"
	result, err := gateway.InvokeFactorySession(ctx, "selected", factorysessions.InvocationRequest{RequestID: &requestID})
	if err != nil || result.RequestID != "request" || result.TraceID != "trace" || result.SessionID != "selected" || result.WorkID != "work" || result.Status != factorysessions.InvocationTerminalStatusCompleted {
		t.Fatalf("injected invocation = %#v, %v", result, err)
	}
	if invoker.sessionID != "selected" || invoker.requestID != requestID {
		t.Fatal("invocation lost addressed request")
	}
	invoker.err = failure
	if _, err := gateway.InvokeFactorySession(ctx, "selected", factorysessions.InvocationRequest{}); !errors.Is(err, failure) {
		t.Fatalf("invocation failure = %v", err)
	}
	invoker.err = nil
	if _, err := gateway.InvokeFactorySession(ctx, "peer", factorysessions.InvocationRequest{}); err != nil || invoker.sessionID != "peer" {
		t.Fatalf("peer invocation = %v", err)
	}
}

type namedActivationOwner struct {
	runtimebinding.SessionProjectionOwner
	activate func(context.Context, string) error
}

func (o namedActivationOwner) ActivateNamedFactory(ctx context.Context, name string) error {
	return o.activate(ctx, name)
}

func TestNamedFactoryActivatorFollowsSelectedGenerationAndPreservesPeers(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	failure := errors.New("selected activation failed")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	register := func(id string, selected bool, want string, result error) {
		state.Register(sessionruntime.Registration{
			SessionID: id, Select: selected,
			Handle: &runtimebinding.SessionState{Owner: namedActivationOwner{
				activate: func(got context.Context, name string) error {
					if got != ctx || name != want {
						t.Fatalf("activation input = %v/%q, want supplied context/%q", got, name, want)
					}
					return result
				},
			}},
		})
	}
	register("selected", true, "first", failure)
	register("peer", false, "peer", nil)
	activate := NewNamedFactoryActivator(state)
	assembly := NewAssembly(nil, nil, state, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, activate, nil).(*Assembly)
	if err := assembly.ActivateNamedFactory(ctx, "first"); !errors.Is(err, failure) {
		t.Fatalf("selected activation error = %v, want %v", err, failure)
	}
	register("selected", true, "replacement", nil)
	if err := assembly.ActivateNamedFactory(ctx, "replacement"); err != nil {
		t.Fatalf("replacement activation = %v", err)
	}
	state.Registry().Upsert(state.Resolve("peer"), true)
	if err := assembly.ActivateNamedFactory(ctx, "peer"); err != nil {
		t.Fatalf("peer activation after selected failure/replacement = %v", err)
	}
}

func TestNamedFactoryActivatorPreservesMissingCapabilityErrors(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	ctx := context.Background()
	if err := NewNamedFactoryActivator(nil)(ctx, "factory"); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("nil authority error = %v", err)
	}
	activate := NewNamedFactoryActivator(state)
	if err := activate(ctx, "factory"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("empty authority error = %v", err)
	}
	state.Register(sessionruntime.Registration{SessionID: "selected", Handle: struct{}{}, Select: true})
	if err := activate(ctx, "factory"); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("missing record capability error = %v", err)
	}
	state.Register(sessionruntime.Registration{SessionID: "selected", Handle: &runtimebinding.SessionState{}, Select: true})
	if err := activate(ctx, "factory"); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) || err.Error() != factorysessions.ErrRuntimeNotAvailable.Error()+": session activation owner is unavailable" {
		t.Fatalf("missing owner error = %v", err)
	}
}

func TestKeyedDefinitionGatewayFollowsAddressedFactsAndReplacement(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	now := time.Date(2026, 10, 5, 3, 4, 5, 0, time.FixedZone("selected", 3600))
	gateway := NewKeyedDefinitionActivationGateway(state, projectionClockStub{now: now})
	register := func(id, root string, selected bool) {
		owner := &SessionRuntime{sessionState: state, factoryRootDir: root, dir: root + "/configured"}
		owner.runtimeState.SetActive(context.Background(), id, nil)
		record := &generationRuntimeRecord{service: &observeStubRuntime{}}
		state.Registry().Upsert(&livesession.LiveSession{ID: id,
			SessionState: livesession.SessionState{FolderPath: root + "/folder", FactoryDir: root + "/factory"},
			Handle:       &runtimebinding.SessionState{Owner: owner, Handle: invocationQueryRun{record: record}},
		}, selected)
	}
	register("selected", "/first", true)
	register("peer", "/peer", false)
	if got := gateway.RunSessionID(); got != "selected" {
		t.Fatalf("run session = %q", got)
	}
	if got := gateway.SaveNow(); !got.Equal(now) || got.Location() != time.UTC {
		t.Fatalf("save clock = %v", got)
	}
	assertKeyedDefinitionPaths(t, gateway, "selected", "/first")
	selected, err := gateway.RequireSession("selected")
	if err != nil {
		t.Fatal(err)
	}
	selected.FactoryDir = "mutated"
	assertKeyedDefinitionPaths(t, gateway, "selected", "/first")
	register("selected", "/replacement", true)
	assertKeyedDefinitionPaths(t, gateway, "selected", "/replacement")
	state.Registry().Remove("selected")
	assertKeyedDefinitionPaths(t, gateway, "peer", "/peer")
	if _, err := gateway.RequireSession("selected"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("retired session error = %v", err)
	}
	failure := errors.New("activation failed")
	if err := gateway.WithActivationLock(func() error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("locked operation error = %v", err)
	}
	if err := gateway.WithActivationLock(func() error { return nil }); err != nil {
		t.Fatalf("lock retry = %v", err)
	}
}

func assertKeyedDefinitionPaths(t *testing.T, gateway factorydefinitions.DefinitionActivationGateway, id, root string) {
	t.Helper()
	session := gateway.SessionForActivation(id)
	if session == nil || session.FactoryDir != root+"/factory" {
		t.Fatalf("addressed definition = %#v", session)
	}
	persist, folder := gateway.NamedFactoryActivationPaths(session)
	want := root + "/folder"
	if persist != want || folder != root+"/folder" || gateway.SessionFactoryPersistRoot(session) != want {
		t.Fatalf("definition paths = %q/%q, persist=%q, want %q", persist, folder, gateway.SessionFactoryPersistRoot(session), want)
	}
}

func TestKeyedDefinitionGatewayPreservesMissingActivationErrors(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	gateway := NewKeyedDefinitionActivationGateway(state, state.Clock())
	ctx := context.Background()
	for _, unavailable := range []bool{false, true} {
		want := factorysessions.ErrSessionNotFound
		if unavailable {
			state.Registry().Upsert(&livesession.LiveSession{ID: "selected", Handle: &runtimebinding.SessionState{
				Handle: invocationQueryRun{record: &generationRuntimeRecord{service: &observeStubRuntime{}}},
			}}, true)
			want = factorysessions.ErrRuntimeNotAvailable
		}
		for _, err := range []error{
			gateway.RequireIdleRuntimeForSession(ctx, "selected"),
			gateway.RequireIdleBeforeNamedFactoryActivation(ctx, "selected", nil),
			gateway.ActivateSessionEditableFactory(ctx, nil, "selected", "root", "dir", "name", "runtime"),
			gateway.SwapPersistedNamedFactoryRuntime(ctx, "selected", nil, "root", "folder", "dir", "name"),
		} {
			if !errors.Is(err, want) {
				t.Fatalf("activation error = %v, want %v", err, want)
			}
		}
	}
}

func TestKeyedDefinitionGatewayPreservesAddressedBuildFailureAndPeer(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	register := func(id string, failure error) {
		owner := &SessionRuntime{sessionState: state, runtimeBuild: replacementRuntimeBuilderFunc(func(got context.Context, folder, dir, sessionID, base string) (factoryruntime.RuntimeRecord, error) {
			if got != ctx || sessionID != id || folder != "folder" || dir != "factory" {
				t.Fatalf("addressed build = %v/%q/%q/%q", got, sessionID, folder, dir)
			}
			return nil, failure
		})}
		state.Registry().Upsert(&livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{Owner: owner,
			Handle: invocationQueryRun{record: &generationRuntimeRecord{service: &observeStubRuntime{}}},
		}}, true)
	}
	gateway := NewKeyedDefinitionActivationGateway(state, state.Clock())
	first := errors.New("selected build failed")
	peer := errors.New("peer build failed")
	register("selected", first)
	register("peer", peer)
	assertKeyedActivationBuildFailure(t, gateway, ctx, "selected", first)
	next := errors.New("replacement build failed")
	register("selected", next)
	assertKeyedActivationBuildFailure(t, gateway, ctx, "selected", next)
	assertKeyedActivationBuildFailure(t, gateway, ctx, "peer", peer)
}

func assertKeyedActivationBuildFailure(t *testing.T, gateway factorydefinitions.DefinitionActivationGateway, ctx context.Context, id string, failure error) {
	t.Helper()
	session := gateway.SessionForActivation(id)
	for _, err := range []error{
		gateway.ActivateSessionEditableFactory(ctx, session, id, "folder", "factory", "named", "runtime"),
		gateway.SwapPersistedNamedFactoryRuntime(ctx, id, session, "persist", "folder", "factory", "named"),
	} {
		if !errors.Is(err, failure) || !errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
			t.Fatalf("addressed build error = %v, want invalid factory and %v", err, failure)
		}
	}
}

type scopedIdentityStub struct{ sessionidentity.Service }

func (scopedIdentityStub) Normalize(_ context.Context, request sessionidentity.NormalizeRequest) (sessionidentity.ResolvedIdentity, error) {
	return sessionidentity.ResolvedIdentity{LogicalSessionKeyID: request.BackendScopeID + "/" + request.FolderPath}, nil
}

func TestProcessHostIdentityUsesAddressedScopeAndCurrentReplacement(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	for _, id := range []string{"first", "peer"} {
		state.Registry().Upsert(&livesession.LiveSession{
			ID: id, SessionState: livesession.SessionState{FolderPath: id},
			Runtime: &factorysessions.LiveRuntime{BackendScopeID: "runtime-" + id},
			Handle:  &runtimebinding.SessionState{ProjectionBackendScope: "override-" + id},
		}, id == "peer")
	}
	host := SessionServiceHost(state, nil, nil, nil, "", scopedIdentityStub{}, nil, nil, nil, nil)
	if host.BackendScopeID() != "override-peer" || host.LogicalSessionKeyID(state.Resolve("first")) != "override-first/first" {
		t.Fatal("process host used current scope for an addressed peer")
	}
	replacement := &livesession.LiveSession{
		ID: "peer", SessionState: livesession.SessionState{FolderPath: "replacement"},
		Runtime: &factorysessions.LiveRuntime{BackendScopeID: "replacement-scope"},
	}
	state.Registry().Upsert(replacement, true)
	if host.BackendScopeID() != "replacement-scope" || host.LogicalSessionKeyID(replacement) != "replacement-scope/replacement" {
		t.Fatal("process host retained the previous generation's scope")
	}
	explicit := SessionServiceHost(state, nil, nil, nil, " selected ", scopedIdentityStub{}, nil, nil, nil, nil)
	if explicit.BackendScopeID() != "selected" || explicit.LogicalSessionKeyID(state.Resolve("first")) != "selected/first" {
		t.Fatal("explicit scope lost precedence")
	}
}

func TestProcessHostStopUsesCapturedOwnerAndPreservesRetryPeer(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	failure := errors.New("injected keyed stop failure")
	lifecycle := &hostLifecycleStub{err: failure}
	var released []string
	owners := make(map[string]*SessionRuntime)
	for _, id := range []string{"first", "peer"} {
		owner := &SessionRuntime{releaseWorkAdmissionProjection: func(id string) { released = append(released, id) }}
		run := invocationQueryRun{record: &generationRuntimeRecord{service: &observeStubRuntime{}}}
		state.Registry().Upsert(&livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{Handle: run, Owner: owner}}, id == "peer")
		owner.runtimeState.SetActive(context.Background(), id, run)
		owners[id] = owner
	}
	control := NewScopeControl(state, func(run factoryruntime.RuntimeRun, _ factoryruntime.Clock) error {
		return lifecycle.Stop(run)
	}, zap.NewNop())
	host := SessionServiceHost(state, nil, control, nil, "", nil, nil, nil, nil, nil)
	if err := host.StopLiveSession("first"); !errors.Is(err, failure) || len(released) != 0 {
		t.Fatalf("failed keyed stop = %v, releases = %v", err, released)
	}
	assertProcessHostStopRecords(t, host, owners, "first")
	lifecycle.err = nil
	if err := host.StopLiveSession("first"); err != nil || !reflect.DeepEqual(released, []string{"first"}) {
		t.Fatalf("keyed retry = %v, releases = %v", err, released)
	}
	if _, err := host.SessionFactory("first"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("retired first read = %v", err)
	}
	assertProcessHostStopRecords(t, host, owners, "peer")
}

func assertProcessHostStopRecords(t *testing.T, host Host, owners map[string]*SessionRuntime, selected string) {
	t.Helper()
	if _, err := host.SessionFactory("peer"); err != nil {
		t.Fatalf("peer read = %v", err)
	}
	if active := owners["first"].runtimeState.Active(); active == nil || active.SessionID != selected {
		t.Fatalf("selected owner active = %#v", active)
	}
	if active := owners["peer"].runtimeState.Active(); active == nil || active.SessionID != "peer" {
		t.Fatalf("peer owner active = %#v", active)
	}
}
