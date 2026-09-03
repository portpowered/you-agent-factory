package runtimebinding_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
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
	dir              string
	folder           string
	backendScope     string
	service          factory.Service
	config           factory.LoadedConfig
	streamGeneration string
	startTime        time.Time
}

func (instance *hostedInstanceFake) RuntimeService() factory.Service { return instance.service }
func (instance *hostedInstanceFake) Directory() string               { return instance.dir }
func (instance *hostedInstanceFake) FolderDirectory() string         { return instance.folder }
func (instance *hostedInstanceFake) BackendScope() string            { return instance.backendScope }
func (instance *hostedInstanceFake) StartTime() time.Time {
	if instance.startTime.IsZero() {
		return time.Time{}
	}
	return instance.startTime
}
func (instance *hostedInstanceFake) LoadedRuntimeConfig() factory.LoadedConfig {
	return instance.config
}
func (*hostedInstanceFake) CanonicalEvents() []interfaces.FactoryEvent { return nil }
func (*hostedInstanceFake) AddEventTypeRecorder(func(interfaces.FactoryEventType)) {
}
func (instance *hostedInstanceFake) StreamGeneration() string {
	return instance.streamGeneration
}
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
	instance factory.RuntimeRecord
	done     chan struct{}
}

func newHostedHandleFake(instance factory.RuntimeRecord) *hostedHandleFake {
	return &hostedHandleFake{instance: instance, done: make(chan struct{})}
}

func (handle *hostedHandleFake) RuntimeInstance() factory.RuntimeRecord {
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

func (lifecycleFake) Start(_ context.Context, instance factory.RuntimeRecord) (factory.RuntimeRun, error) {
	return newHostedHandleFake(instance), nil
}
func (lifecycleFake) WaitForStart(context.Context, factory.RuntimeRun) error { return nil }
func (lifecycleFake) Stop(handle factory.RuntimeRun) error {
	handle.CancelRun()
	return nil
}
func (lifecycleFake) StopSidecars(factory.RuntimeRun) {}
func (lifecycleFake) PublishReplacement(context.Context, factory.RuntimeRun, factory.RuntimeRecord) error {
	return nil
}

var (
	_ factory.RuntimeRecord    = (*hostedInstanceFake)(nil)
	_ factory.RuntimeRun       = (*hostedHandleFake)(nil)
	_ factory.RuntimeLifecycle = lifecycleFake{}
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

	var stopped factory.RuntimeRun
	err := runtimebinding.StopSession(state, &active, first.ID, func(handle factory.RuntimeRun) error {
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

func TestStopSessionRetiresRegisteredTerminalRuntime(t *testing.T) {
	state := newRuntimeBindingState()
	terminal := registerTestSession(state, "terminal")
	successor := registerTestSession(state, "successor")
	terminalHandle := runtimebinding.HandleFromSession(terminal).(*hostedHandleFake)
	terminalHandle.instance = nil
	var active runtimebinding.State
	active.SetActive(context.Background(), terminal.ID, runtimebinding.HandleFromSession(terminal))

	var stopped factory.RuntimeRun
	err := runtimebinding.StopSession(state, &active, terminal.ID, func(handle factory.RuntimeRun) error {
		stopped = handle
		return nil
	})
	if err != nil {
		t.Fatalf("StopSession terminal runtime: %v", err)
	}
	if stopped != terminalHandle {
		t.Fatal("StopSession did not stop the registered terminal handle")
	}
	if state.Resolve(terminal.ID) != nil {
		t.Fatal("terminal session remains registered")
	}
	if got := active.Active(); got == nil || got.SessionID != successor.ID {
		t.Fatalf("active runtime = %#v, want successor session", got)
	}
}

func TestStopSessionRetiresSessionWhenRuntimeAlreadyStopped(t *testing.T) {
	t.Parallel()

	for _, stopErr := range []error{factory.ErrAlreadyStopped, factory.ErrNotRunning} {
		stopErr := stopErr
		t.Run(stopErr.Error(), func(t *testing.T) {
			state := newRuntimeBindingState()
			session := registerTestSession(state, "already-stopped")
			var active runtimebinding.State
			active.SetActive(context.Background(), session.ID, runtimebinding.HandleFromSession(session))

			if err := runtimebinding.StopSession(state, &active, session.ID, func(factory.RuntimeRun) error {
				return stopErr
			}); err != nil {
				t.Fatalf("StopSession(%v): %v", stopErr, err)
			}
			if state.Resolve(session.ID) != nil {
				t.Fatalf("session remains registered after %v cleanup", stopErr)
			}
		})
	}
}

func TestOpaqueBindingRoutesSessionServiceAndCleanup(t *testing.T) {
	t.Parallel()

	sessions := newRuntimeBindingState()
	fallback := &replacementFactory{}
	bound := &replacementFactory{}
	instance := &hostedInstanceFake{service: fallback}
	handle := newHostedHandleFake(instance)
	deactivationCalls := 0
	binding := factory.RuntimeBinding{}.New(
		"runtime-bound",
		bound,
		func(context.Context) (factory.RuntimeDeactivationResult, error) {
			deactivationCalls++
			return factory.RuntimeDeactivationResult{RuntimeID: "runtime-bound", State: factory.RuntimeLifecycleStateStopped}, nil
		},
	)
	sessions.Register(sessionruntime.Registration{
		SessionID: "bound-session",
		Handle:    &runtimebinding.SessionState{Instance: instance, Handle: handle},
		Runtime: &factorysessions.LiveRuntime{
			Factory: fallback,
			Binding: binding,
		},
	})

	resolved, err := runtimebinding.FactoryForSession(sessions, "bound-session")
	if err != nil {
		t.Fatalf("FactoryForSession: %v", err)
	}
	if resolved != bound {
		t.Fatalf("FactoryForSession = %p, want opaque binding service %p", resolved, bound)
	}

	var runtimeState runtimebinding.State
	runtimeState.SetActive(context.Background(), "bound-session", handle)
	if err := runtimebinding.StopSession(sessions, &runtimeState, "bound-session", func(factory.RuntimeRun) error { return nil }); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if deactivationCalls != 1 {
		t.Fatalf("binding deactivation calls = %d, want one", deactivationCalls)
	}
	if sessions.Resolve("bound-session") != nil {
		t.Fatal("bound session remains registered after cleanup")
	}
}

func TestShutdownOtherLiveSessionsKeepsExceptAndJoinsFailures(t *testing.T) {
	state := newRuntimeBindingState()
	keep := registerTestSession(state, "keep")
	first := registerTestSession(state, "first")
	second := registerTestSession(state, "second")
	stopErr := errors.New("stop failed")
	stopped := map[factory.RuntimeRun]bool{}

	err := runtimebinding.ShutdownOtherLiveSessions(
		state,
		runtimebinding.HandleFromSession(keep),
		func(hosted factory.RuntimeRun) error {
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
	factory.Service
}

func (replacementFactory) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (replacementFactory) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{
		Observation: factory.Observation{
			Status: factory.ObservationStatusActive,
			Health: factory.ObservationHealth{FactoryState: string(interfaces.FactoryStateRunning)},
		},
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
	var stopped factory.RuntimeRun
	var retiredSessionID string
	var retiredRecord factory.RuntimeRecord
	var retiredAfterStop bool

	updated, err := runtimebinding.Replace(
		context.Background(),
		sessions,
		&runtimeState,
		original,
		replacement,
		false,
		lifecycleFake{},
		nil,
		func(handle factory.RuntimeRun) error {
			stopped = handle
			return nil
		},
		nil,
		func(sessionID string, _ *factorysessions.LiveRuntime, record factory.RuntimeRecord) {
			retiredSessionID = sessionID
			retiredRecord = record
			retiredAfterStop = stopped == oldHandle
		},
	)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if stopped != oldHandle {
		t.Fatal("Replace did not stop the previous runtime")
	}
	if retiredSessionID != original.ID || retiredRecord != oldInstance {
		t.Fatalf("retired runtime = (%q, %T), want (%q, %T)", retiredSessionID, retiredRecord, original.ID, oldInstance)
	}
	if !retiredAfterStop {
		t.Fatal("runtime retirement callback ran before the previous runtime stopped")
	}
	newHandle := runtimebinding.HandleFromSession(updated)
	assertReplacementSession(t, original, updated, oldHandle, newHandle, preparedSpec)
	assertActiveReplacement(t, &runtimeState, updated, newHandle)
	newHandle.CancelRun()
	<-newHandle.RunDoneCh()
}

func assertReplacementSession(
	t *testing.T,
	original *livesession.LiveSession,
	updated *livesession.LiveSession,
	oldHandle factory.RuntimeRun,
	newHandle factory.RuntimeRun,
	preparedSpec any,
) {
	t.Helper()
	if updated == nil {
		t.Fatal("replacement session is nil")
		return
	}
	if updated == original || newHandle == nil || newHandle == oldHandle {
		t.Fatalf("updated session/runtime = (%p, %p), want replacement", updated, newHandle)
	}
	if updated.FactoryDir != "/new" || updated.ExecutionBaseDir != "/old-execution" ||
		updated.RuntimeFactorySessionID != "runtime-session-1" {
		t.Fatalf("replacement metadata = %#v", updated)
	}
	if updated.ResponseEvents == nil || updated.ResponseEvents.FactorySessionID() != "runtime-session-1" {
		t.Fatalf("replacement response-event store = %#v, want runtime-session-1", updated.ResponseEvents)
	}
	if runtimebinding.PreparedSpecFromSession(updated) != preparedSpec {
		t.Fatal("replacement did not preserve the prepared runtime specification")
	}
}

func assertActiveReplacement(
	t *testing.T,
	runtimeState *runtimebinding.State,
	updated *livesession.LiveSession,
	newHandle factory.RuntimeRun,
) {
	t.Helper()
	active := runtimeState.Active()
	if active == nil || active.SessionID != updated.ID || active.Handle != newHandle {
		t.Fatalf("active runtime = %#v, want replacement handle", active)
	}
}

func TestStartInitialRegistersAndSelectsCanonicalDefaultSession(t *testing.T) {
	sessions := newRuntimeBindingState()
	var runtimeState runtimebinding.State
	bundle := &hostedInstanceFake{dir: "/factory", service: replacementFactory{}}
	runtimeState.SetStartup(bundle)

	handle, err := runtimebinding.StartInitial(
		context.Background(),
		context.Background(),
		sessions,
		&runtimeState,
		factorysessions.DefaultSessionID,
		"/factory",
		bundle,
		factorysessions.Target{
			Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		},
		interfaces.RuntimeModeBatch,
		lifecycleFake{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("StartInitial: %v", err)
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

func TestStartInitialRegistersExplicitSessionWithoutDefaultAlias(t *testing.T) {
	t.Parallel()

	const sessionID = "session-explicit"
	sessions := newRuntimeBindingState()
	var runtimeState runtimebinding.State
	bundle := &hostedInstanceFake{dir: "/factory", service: replacementFactory{}}
	runtimeState.SetStartup(bundle)
	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "factory-a"},
		FactoryDir: "/factory",
		FolderPath: "/workspace",
		Project:    "project-a",
	}

	handle, err := runtimebinding.StartInitial(
		context.Background(), context.Background(), sessions, &runtimeState,
		sessionID, "/factory", bundle, target, interfaces.RuntimeModeBatch,
		lifecycleFake{}, nil, nil,
	)
	if err != nil {
		t.Fatalf("StartInitial: %v", err)
	}
	t.Cleanup(func() {
		handle.CancelRun()
		<-handle.RunDoneCh()
	})

	session := sessions.Resolve(sessionID)
	if session == nil || runtimebinding.HandleFromSession(session) != handle {
		t.Fatalf("explicit session = %#v, want started handle", session)
	}
	if sessions.Resolve(factorysessions.DefaultSessionID) != nil {
		t.Fatal("explicit startup also registered the compatibility default session")
	}
	if session.Target != target.Ref || session.FactoryDir != target.FactoryDir ||
		session.FolderPath != target.FolderPath || session.Project != target.Project {
		t.Fatalf("explicit session target = %#v, want %#v", session, target)
	}
	if active := runtimeState.Active(); active == nil || active.SessionID != sessionID || active.Handle != handle {
		t.Fatalf("active runtime = %#v, want explicit session %q", active, sessionID)
	}
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

func TestCurrentBundlePrefersInvocationStartupOverLiveProcessDefault(t *testing.T) {
	t.Parallel()

	sessions := newRuntimeBindingState()
	processDefault := &hostedInstanceFake{dir: "/process-default"}
	defaultHandle := newHostedHandleFake(processDefault)
	t.Cleanup(func() {
		defaultHandle.CancelRun()
		<-defaultHandle.RunDoneCh()
	})
	sessions.Register(sessionruntime.Registration{
		SessionID: factorysessions.DefaultSessionID,
		Handle:    &runtimebinding.SessionState{Instance: processDefault, Handle: defaultHandle},
		Default:   true,
		Select:    true,
	})
	startup := &hostedInstanceFake{dir: "/explicit-startup"}
	var runtimeState runtimebinding.State
	runtimeState.SetStartup(startup)

	if got := runtimebinding.CurrentBundle(sessions, &runtimeState); got != startup {
		t.Fatalf("CurrentBundle = %p, want invocation startup %p instead of process default", got, startup)
	}
}

func TestHandleStartFailureTreatsClosedServiceSessionAsExpected(t *testing.T) {
	sessions := newRuntimeBindingState()
	var runtimeState runtimebinding.State
	handle := newHostedHandleFake(&hostedInstanceFake{})
	runtimeState.SetActive(context.Background(), factorysessions.DefaultSessionID, handle)
	var stopped bool
	var removed string

	err := runtimebinding.HandleStartFailure(
		context.Background(), sessions, &runtimeState, factorysessions.DefaultSessionID,
		handle, func(factory.RuntimeRun) error {
			stopped = true
			return nil
		},
		errors.New("startup failed"), interfaces.RuntimeModeService,
		func(sessionID string) { removed = sessionID },
	)
	if err != nil {
		t.Fatalf("HandleStartFailure: %v", err)
	}
	if !stopped || runtimeState.Active() != nil {
		t.Fatalf("cleanup = (stopped %v, active %#v)", stopped, runtimeState.Active())
	}
	if removed != factorysessions.DefaultSessionID {
		t.Fatalf("removed session = %q, want default session", removed)
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
		runtimebinding.HandleFromSession(session), func(factory.RuntimeRun) error { return nil },
		startErr, interfaces.RuntimeModeBatch, nil,
	)
	if !errors.Is(err, startErr) {
		t.Fatalf("HandleStartFailure error = %v, want startup failure", err)
	}
	if sessions.Resolve(factorysessions.DefaultSessionID) != nil || runtimeState.Active() != nil {
		t.Fatal("failed batch session remains active")
	}
}

func TestHandleStartFailureIgnoresAlreadyStoppedCleanupAfterCancellation(t *testing.T) {
	sessions := newRuntimeBindingState()
	session := registerTestSession(sessions, factorysessions.DefaultSessionID)
	var runtimeState runtimebinding.State
	runtimeState.SetActive(context.Background(), session.ID, runtimebinding.HandleFromSession(session))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runtimebinding.HandleStartFailure(
		ctx, sessions, &runtimeState, factorysessions.DefaultSessionID,
		runtimebinding.HandleFromSession(session),
		func(factory.RuntimeRun) error { return factory.ErrAlreadyStopped },
		context.Canceled, interfaces.RuntimeModeBatch, nil,
	)
	if err != nil {
		t.Fatalf("HandleStartFailure: %v, want canceled startup cleanup to be idempotent", err)
	}
	if sessions.Resolve(factorysessions.DefaultSessionID) != nil || runtimeState.Active() != nil {
		t.Fatal("canceled startup session remains active")
	}
}

func registerTestSession(state *sessionruntime.Service, sessionID string) *livesession.LiveSession {
	instance := &hostedInstanceFake{}
	handle := newHostedHandleFake(instance)
	state.Register(sessionruntime.Registration{
		SessionID: sessionID,
		Handle:    &runtimebinding.SessionState{Instance: instance, Handle: handle},
	})
	return state.Resolve(sessionID)
}

type streamGenerationService struct {
	factory.Service
	streamGenerationID string
}

func (service streamGenerationService) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{
		Observation: factory.Observation{
			Health: factory.ObservationHealth{StreamGenerationID: service.streamGenerationID},
		},
	}, nil
}

type legacySnapshotService struct {
	factory.Service
}

func (legacySnapshotService) GetEngineStateSnapshot(context.Context) (*legacysnapshot.Snapshot, error) {
	return nil, nil
}

type legacyEventService struct {
	factory.Service
}

func (legacyEventService) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}

func TestStreamGenerationIDPrefersInstanceTokenThenObserveThenStartTime(t *testing.T) {
	t.Parallel()

	instanceToken := &hostedInstanceFake{streamGeneration: "stream-from-instance"}
	session := &livesession.LiveSession{
		Handle: &runtimebinding.SessionState{
			Instance: instanceToken,
			Handle:   newHostedHandleFake(instanceToken),
		},
	}
	if got := runtimebinding.StreamGenerationID(session); got != "stream-from-instance" {
		t.Fatalf("instance stream generation = %q, want stream-from-instance", got)
	}

	observeToken := &hostedInstanceFake{
		service: streamGenerationService{streamGenerationID: "stream-from-observe"},
	}
	observeSession := &livesession.LiveSession{
		Handle: &runtimebinding.SessionState{
			Instance: observeToken,
			Handle:   newHostedHandleFake(observeToken),
		},
	}
	if got := runtimebinding.StreamGenerationID(observeSession); got != "stream-from-observe" {
		t.Fatalf("observe stream generation = %q, want stream-from-observe", got)
	}

	startedAt := time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)
	startTimeInstance := &hostedInstanceFake{startTime: startedAt}
	startTimeSession := &livesession.LiveSession{
		Handle: &runtimebinding.SessionState{
			Instance: startTimeInstance,
			Handle:   newHostedHandleFake(startTimeInstance),
		},
	}
	if got := runtimebinding.StreamGenerationID(startTimeSession); got != startedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("start-time stream generation = %q, want %q", got, startedAt.UTC().Format(time.RFC3339Nano))
	}
}

func TestLegacyObservationHelpersResolveMigrationCapabilities(t *testing.T) {
	t.Parallel()

	if _, err := runtimebinding.LegacyObservationForService(replacementFactory{}); err == nil {
		t.Fatal("expected legacy observation error for service without snapshot provider")
	}

	legacy := struct {
		factory.Service
		legacySnapshotService
	}{Service: replacementFactory{}}
	provider, err := runtimebinding.LegacyObservationForService(legacy)
	if err != nil || provider == nil {
		t.Fatalf("LegacyObservationForService = (%v, %v), want provider", provider, err)
	}

	if _, err := runtimebinding.LegacyEventSourceForService(replacementFactory{}); err == nil {
		t.Fatal("expected legacy event source error")
	}

	events := struct {
		factory.Service
		legacyEventService
	}{Service: replacementFactory{}}
	source, err := runtimebinding.LegacyEventSourceForService(events)
	if err != nil || source == nil {
		t.Fatalf("LegacyEventSourceForService = (%v, %v), want source", source, err)
	}

	combined := struct {
		factory.Service
		legacySnapshotService
		legacyEventService
	}{Service: replacementFactory{}}
	snapshotProvider, eventSource, err := runtimebinding.LegacyInvocationSourcesForService(combined)
	if err != nil || snapshotProvider == nil || eventSource == nil {
		t.Fatalf("LegacyInvocationSourcesForService = (%v, %v, %v)", snapshotProvider, eventSource, err)
	}
}
