package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
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
	assembly := &Assembly{SessionGateway: &Service{durable: owner}}
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

func (owner *closeRegistryOwnerFake) PrepareOwnedSessionClose(_ context.Context, sessionID string) error {
	*owner.order = append(*owner.order, sessionID)
	if sessionID == owner.failID {
		return owner.failErr
	}
	return nil
}

func (owner *closeRegistryOwnerFake) RetireOwnedSession(sessionID string) error {
	owner.registry.Remove(sessionID)
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

type stubRuntimeSidecars struct {
	preseedCalls int
	startCalls   int
	stopCalls    int
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
	assembly := NewAssembly(state.Registry(), state, streams, nil, nil, nil, nil, nil, nil, nil, state.Clock(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).(*Assembly)
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
