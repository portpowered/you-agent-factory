package runtimebinding_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

func newRuntimeBindingState() *sessionruntime.Service {
	clock := platformclock.Real{}
	newStream := func() *responsestream.SessionResponseStream {
		return responsestream.NewSessionResponseStream(clock)
	}
	responses := responsestream.NewRegistry(newStream, clock)
	return sessionruntime.New(sessionregistry.New(), responses, nil, clock, func() string { return "response-event-test-id" }, func() string { return "session-test-id" })
}

type hostedInstanceFake struct {
	dir          string
	folder       string
	backendScope string
	service      factory.Service
	config       factory.LoadedConfig
}

func (instance *hostedInstanceFake) RuntimeService() factory.Service { return instance.service }
func (instance *hostedInstanceFake) Directory() string               { return instance.dir }
func (instance *hostedInstanceFake) FolderDirectory() string         { return instance.folder }
func (instance *hostedInstanceFake) BackendScope() string            { return instance.backendScope }
func (*hostedInstanceFake) StartTime() time.Time                     { return time.Time{} }
func (instance *hostedInstanceFake) LoadedRuntimeConfig() factory.LoadedConfig {
	return instance.config
}
func (*hostedInstanceFake) CanonicalEvents() []interfaces.FactoryEvent { return nil }
func (*hostedInstanceFake) AddEventTypeRecorder(func(interfaces.FactoryEventType)) {
}
func (*hostedInstanceFake) StreamGeneration() string { return "" }
func (*hostedInstanceFake) RuntimeLogger() *zap.Logger {
	return zap.NewNop()
}
func (*hostedInstanceFake) RuntimeMetrics() factory.MetricsEmitter { return nil }
func (*hostedInstanceFake) RuntimeDiagnostics() factory.RuntimeLogDiagnostics {
	return factory.RuntimeLogDiagnostics{}
}
func (*hostedInstanceFake) RecordingLedger() recordings.Ledger { return nil }
func (*hostedInstanceFake) CloseArtifacts() error              { return nil }

type hostedHandleFake struct {
	instance factory.HostedInstance
	done     chan struct{}
}

func newHostedHandleFake(instance factory.HostedInstance) *hostedHandleFake {
	return &hostedHandleFake{instance: instance, done: make(chan struct{})}
}

func (handle *hostedHandleFake) RuntimeInstance() factory.HostedInstance {
	return handle.instance
}
func (handle *hostedHandleFake) Completed() bool {
	select {
	case <-handle.done:
		return true
	default:
		return false
	}
}
func (*hostedHandleFake) Result() error { return nil }
func (handle *hostedHandleFake) Wait() error {
	<-handle.done
	return nil
}
func (handle *hostedHandleFake) CancelRun() {
	if !handle.Completed() {
		close(handle.done)
	}
}
func (handle *hostedHandleFake) RunDoneCh() <-chan struct{} { return handle.done }

type lifecycleFake struct{}

func (lifecycleFake) Start(_ context.Context, instance factory.HostedInstance) (factory.HostedHandle, error) {
	return newHostedHandleFake(instance), nil
}
func (lifecycleFake) WaitForStart(context.Context, factory.HostedHandle) error { return nil }
func (lifecycleFake) Stop(handle factory.HostedHandle) error {
	handle.CancelRun()
	return nil
}
func (lifecycleFake) StopSidecars(factory.HostedHandle) {}
func (lifecycleFake) PublishReplacement(context.Context, factory.HostedHandle, factory.HostedInstance) error {
	return nil
}

var (
	_ factory.HostedInstance = (*hostedInstanceFake)(nil)
	_ factory.HostedHandle   = (*hostedHandleFake)(nil)
	_ factory.Lifecycle      = lifecycleFake{}
)

func TestSyncActiveDirectoryUsesBundleAndFallsBackToFactoryRoot(t *testing.T) {
	var mu sync.RWMutex
	configured := "initial"

	runtimebinding.SyncActiveDirectory(
		&mu,
		&configured,
		"factory-root",
		&hostedInstanceFake{dir: "named-factory"},
	)
	if configured != "named-factory" {
		t.Fatalf("configured directory = %q, want named-factory", configured)
	}

	runtimebinding.SyncActiveDirectory(&mu, &configured, "factory-root", nil)
	if configured != "factory-root" {
		t.Fatalf("fallback directory = %q, want factory-root", configured)
	}
}

func TestStopSessionSelectsAnotherLiveRuntime(t *testing.T) {
	state := newRuntimeBindingState()
	first := registerTestSession(state, "first")
	second := registerTestSession(state, "second")
	var active runtimebinding.State
	active.SetActive(context.Background(), first.ID, runtimebinding.HandleFromSession(first))

	var stopped factory.HostedHandle
	err := runtimebinding.StopSession(state, &active, first.ID, func(handle factory.HostedHandle) error {
		stopped = handle
		return nil
	})
	if err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if stopped != runtimebinding.HandleFromSession(first) {
		t.Fatal("StopSession stopped the wrong runtime")
	}
	if state.Resolve(first.ID) != nil {
		t.Fatal("stopped session remains registered")
	}
	if got := active.Active(); got == nil || got.SessionID != second.ID || got.Handle != runtimebinding.HandleFromSession(second) {
		t.Fatalf("active runtime = %#v, want second session", got)
	}
}

func TestShutdownOtherLiveSessionsKeepsExceptAndJoinsFailures(t *testing.T) {
	state := newRuntimeBindingState()
	keep := registerTestSession(state, "keep")
	first := registerTestSession(state, "first")
	second := registerTestSession(state, "second")
	stopErr := errors.New("stop failed")
	stopped := map[factory.HostedHandle]bool{}

	err := runtimebinding.ShutdownOtherLiveSessions(
		state,
		runtimebinding.HandleFromSession(keep),
		func(hosted factory.HostedHandle) error {
			stopped[hosted] = true
			if hosted == runtimebinding.HandleFromSession(first) {
				return stopErr
			}
			return nil
		},
	)
	if !errors.Is(err, stopErr) {
		t.Fatalf("ShutdownOtherLiveSessions error = %v, want %v", err, stopErr)
	}
	if state.Resolve(keep.ID) == nil {
		t.Fatal("except session was removed")
	}
	if state.Resolve(first.ID) != nil || state.Resolve(second.ID) != nil {
		t.Fatal("stopped sessions remain registered")
	}
	if stopped[runtimebinding.HandleFromSession(keep)] ||
		!stopped[runtimebinding.HandleFromSession(first)] ||
		!stopped[runtimebinding.HandleFromSession(second)] {
		t.Fatalf("stopped handles = %#v", stopped)
	}
}

type replacementFactory struct {
	factory.Factory
}

func (replacementFactory) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (replacementFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net], error) {
	return &interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	}, nil
}

func TestReplaceTransfersLiveSessionAndActiveRuntimeOwnership(t *testing.T) {
	sessions := newRuntimeBindingState()
	oldInstance := &hostedInstanceFake{}
	oldHandle := newHostedHandleFake(oldInstance)
	preparedSpec := &struct{ Name string }{Name: "prepared"}
	sessions.Register(sessionruntime.Registration{
		SessionID: "session-1", FactoryDir: "/old", FolderPath: "/workspace",
		ExecutionBaseDir: "/old-execution", Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "alpha"},
		Handle:  &runtimebinding.SessionState{Instance: oldInstance, Handle: oldHandle, Spec: preparedSpec},
		Default: false, Project: "project", Select: true,
	})
	original := sessions.Resolve("session-1")
	original.RuntimeFactorySessionID = "runtime-session-1"
	var runtimeState runtimebinding.State
	runtimeState.SetActive(context.Background(), original.ID, oldHandle)
	replacement := &hostedInstanceFake{
		dir: "/new", service: replacementFactory{}, backendScope: "backend-new",
	}
	var stopped factory.HostedHandle

	updated, err := runtimebinding.Replace(
		context.Background(),
		sessions,
		&runtimeState,
		original,
		replacement,
		false,
		lifecycleFake{},
		nil,
		func(handle factory.HostedHandle) error {
			stopped = handle
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if stopped != oldHandle {
		t.Fatal("Replace did not stop the previous runtime")
	}
	newHandle := runtimebinding.HandleFromSession(updated)
	if updated == nil || updated == original || newHandle == nil || newHandle == oldHandle {
		t.Fatalf("updated session/runtime = (%p, %p), want replacement", updated, newHandle)
	}
	if updated.FactoryDir != "/new" || updated.ExecutionBaseDir != "/old-execution" ||
		updated.RuntimeFactorySessionID != "runtime-session-1" {
		t.Fatalf("replacement metadata = %#v", updated)
	}
	if runtimebinding.PreparedSpecFromSession(updated) != preparedSpec {
		t.Fatal("replacement did not preserve the prepared runtime specification")
	}
	if active := runtimeState.Active(); active == nil || active.SessionID != updated.ID || active.Handle != newHandle {
		t.Fatalf("active runtime = %#v, want replacement handle", active)
	}
	newHandle.CancelRun()
	<-newHandle.RunDoneCh()
}

func TestStartDefaultRegistersAndSelectsCanonicalSession(t *testing.T) {
	sessions := newRuntimeBindingState()
	var runtimeState runtimebinding.State
	bundle := &hostedInstanceFake{dir: "/factory", service: replacementFactory{}}
	runtimeState.SetStartup(bundle)

	handle, err := runtimebinding.StartDefault(
		context.Background(),
		context.Background(),
		sessions,
		&runtimeState,
		"/factory",
		bundle,
		factorysessions.Target{
			Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		},
		false,
		interfaces.RuntimeModeBatch,
		lifecycleFake{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("StartDefault: %v", err)
	}
	session := sessions.Resolve(factorysessions.DefaultSessionID)
	if session == nil || runtimebinding.HandleFromSession(session) != handle {
		t.Fatalf("default session = %#v, want started handle", session)
	}
	if active := runtimeState.Active(); active == nil || active.SessionID != session.ID || active.Handle != handle {
		t.Fatalf("active runtime = %#v, want registered default session", active)
	}
	if runtimeState.Startup() != nil {
		t.Fatal("startup bundle was not released after default start")
	}
	handle.CancelRun()
	<-handle.RunDoneCh()
}

func TestCurrentBundleIgnoresPreparedDefaultWithoutLiveHandle(t *testing.T) {
	sessions := newRuntimeBindingState()
	prepared := &hostedInstanceFake{dir: "/prepared"}
	startup := &hostedInstanceFake{dir: "/replacement"}
	sessions.Register(sessionruntime.Registration{
		SessionID: factorysessions.DefaultSessionID,
		Handle:    &runtimebinding.SessionState{Instance: prepared},
		Default:   true,
		Select:    true,
	})
	var runtimeState runtimebinding.State
	runtimeState.SetStartup(startup)

	if got := runtimebinding.CurrentBundle(sessions, &runtimeState); got != startup {
		t.Fatalf("CurrentBundle = %p, want startup replacement %p", got, startup)
	}
}

func TestHandleStartFailureTreatsClosedServiceSessionAsExpected(t *testing.T) {
	sessions := newRuntimeBindingState()
	var runtimeState runtimebinding.State
	handle := newHostedHandleFake(&hostedInstanceFake{})
	runtimeState.SetActive(context.Background(), factorysessions.DefaultSessionID, handle)
	var stopped bool

	err := runtimebinding.HandleStartFailure(
		context.Background(), sessions, &runtimeState, factorysessions.DefaultSessionID,
		handle, func(factory.HostedHandle) error {
			stopped = true
			return nil
		},
		errors.New("startup failed"), interfaces.RuntimeModeService,
	)
	if err != nil {
		t.Fatalf("HandleStartFailure: %v", err)
	}
	if !stopped || runtimeState.Active() != nil {
		t.Fatalf("cleanup = (stopped %v, active %#v)", stopped, runtimeState.Active())
	}
}

func TestHandleStartFailureUnregistersFailedBatchSession(t *testing.T) {
	sessions := newRuntimeBindingState()
	session := registerTestSession(sessions, factorysessions.DefaultSessionID)
	var runtimeState runtimebinding.State
	runtimeState.SetActive(context.Background(), session.ID, runtimebinding.HandleFromSession(session))
	startErr := errors.New("startup failed")

	err := runtimebinding.HandleStartFailure(
		context.Background(), sessions, &runtimeState, factorysessions.DefaultSessionID,
		runtimebinding.HandleFromSession(session), func(factory.HostedHandle) error { return nil },
		startErr, interfaces.RuntimeModeBatch,
	)
	if !errors.Is(err, startErr) {
		t.Fatalf("HandleStartFailure error = %v, want startup failure", err)
	}
	if sessions.Resolve(factorysessions.DefaultSessionID) != nil || runtimeState.Active() != nil {
		t.Fatal("failed batch session remains active")
	}
}

func registerTestSession(state *sessionruntime.Service, sessionID string) *factorysessions.LiveSession {
	instance := &hostedInstanceFake{}
	handle := newHostedHandleFake(instance)
	state.Register(sessionruntime.Registration{
		SessionID: sessionID,
		Handle:    &runtimebinding.SessionState{Instance: instance, Handle: handle},
	})
	return state.Resolve(sessionID)
}
