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
	"go.uber.org/zap/zaptest/observer"
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
	logger           *zap.Logger
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
func (*hostedInstanceFake) AddEventTypeRecorderWithReady(func(interfaces.FactoryEventType), func()) {
}
func (instance *hostedInstanceFake) StreamGeneration() string {
	return instance.streamGeneration
}
func (instance *hostedInstanceFake) RuntimeLogger() *zap.Logger {
	if instance.logger != nil {
		return instance.logger
	}
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

type canceledReadinessLifecycle struct{ lifecycleFake }

type startupReadinessLifecycle struct {
	lifecycleFake
	readiness func(context.Context, factory.RuntimeRun) error
}

func (l startupReadinessLifecycle) WaitForStart(ctx context.Context, run factory.RuntimeRun) error {
	return l.readiness(ctx, run)
}

func (canceledReadinessLifecycle) WaitForStart(ctx context.Context, _ factory.RuntimeRun) error {
	return ctx.Err()
}

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
	err := runtimebinding.StopSessionGeneration(state, &active, first, func(handle factory.RuntimeRun) error {
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
	err := runtimebinding.StopSessionGeneration(state, &active, terminal, func(handle factory.RuntimeRun) error {
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

func TestStopSessionPreservesReplacementPublishedDuringStop(t *testing.T) {
	t.Parallel()
	state := newRuntimeBindingState()
	old := registerTestSession(state, "a")
	peer := registerTestSession(state, "b")
	var active runtimebinding.State
	active.SetActive(context.Background(), old.ID, runtimebinding.HandleFromSession(old))
	var replacement *livesession.LiveSession
	err := runtimebinding.StopSessionGeneration(state, &active, old, func(handle factory.RuntimeRun) error {
		if handle != runtimebinding.HandleFromSession(old) {
			t.Fatal("stop targeted the replacement")
		}
		replacement = registerTestSession(state, "a")
		return nil
	})
	if err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if state.Resolve("a") != replacement || state.Resolve("b") != peer {
		t.Fatal("old retirement removed the replacement or peer")
	}
	if _, err := state.ResponseStreams().Streams("a").Subscribe("next", 0); err != nil {
		t.Fatalf("replacement response stream was closed: %v", err)
	}
	state.Unregister("a")
	state.Unregister("b")
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

			if err := runtimebinding.StopSessionGeneration(state, &active, session, func(factory.RuntimeRun) error {
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

func TestStopSessionFailedCleanupPreservesSelectionForRetry(t *testing.T) {
	t.Parallel()
	state := newRuntimeBindingState()
	session := registerTestSession(state, "a")
	peer := registerTestSession(state, "b")
	t.Cleanup(func() { state.Unregister("a"); state.Unregister("b") })
	var active runtimebinding.State
	active.SetActive(context.Background(), session.ID, runtimebinding.HandleFromSession(session))
	failure := errors.New("owned stop failed")
	if err := runtimebinding.StopSessionGeneration(state, &active, session, func(factory.RuntimeRun) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("failed stop = %v, want original failure", err)
	}
	if state.Resolve("a") != session || state.Resolve("b") != peer {
		t.Fatal("failed cleanup lost its retryable record or peer")
	}
	if got := active.Current(nil); got != runtimebinding.HandleFromSession(session).RuntimeInstance() {
		t.Fatal("failed cleanup redirected current runtime away from retryable session")
	}
	if err := runtimebinding.StopSessionGeneration(state, &active, session, func(factory.RuntimeRun) error { return nil }); err != nil {
		t.Fatalf("retry stop = %v", err)
	}
	if state.Resolve("a") != nil || active.Current(nil) != runtimebinding.HandleFromSession(peer).RuntimeInstance() {
		t.Fatal("successful retry did not retire A and select its live peer")
	}
}

func TestStopSessionCleanupKeepsNewActiveSelection(t *testing.T) {
	t.Parallel()
	for _, selection := range []string{"replacement", "peer", "unselected replacement"} {
		t.Run(selection, func(t *testing.T) {
			t.Parallel()
			state := newRuntimeBindingState()
			old := registerTestSession(state, "a")
			peer := registerTestSession(state, "b")
			t.Cleanup(func() { state.Unregister("a"); state.Unregister("b") })
			var active runtimebinding.State
			active.SetActive(context.Background(), old.ID, runtimebinding.HandleFromSession(old))
			var expected *livesession.LiveSession
			selectedContext, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			err := runtimebinding.StopSessionGeneration(state, &active, old, func(factory.RuntimeRun) error {
				expected = registerTestSession(state, "a")
				if selection == "peer" {
					expected = peer
				}
				if selection != "unselected replacement" {
					active.SetActive(selectedContext, expected.ID, runtimebinding.HandleFromSession(expected))
				}
				return nil
			})
			if err != nil {
				t.Fatalf("stop = %v", err)
			}
			if active.Current(nil) != runtimebinding.HandleFromSession(expected).RuntimeInstance() {
				t.Fatal("old cleanup replaced the new active runtime selection")
			}
			if selection != "unselected replacement" && active.Active().Context != selectedContext {
				t.Fatal("old cleanup replaced the new selection's context")
			}
			if state.Resolve("a") == old || state.Resolve("a") == nil || state.Resolve("b") != peer {
				t.Fatal("old cleanup retired a replacement or peer")
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
	if err := runtimebinding.StopSessionGeneration(sessions, &runtimeState, sessions.Resolve("bound-session"), func(factory.RuntimeRun) error { return nil }); err != nil {
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

func TestShutdownOtherLiveSessionsKeepsReplacementsPublishedDuringStop(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"same session", "later session", "stop failure"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			state := newRuntimeBindingState()
			keep := registerTestSession(state, "keep")
			first := registerTestSession(state, "a")
			later := registerTestSession(state, "b")
			firstRun := runtimebinding.HandleFromSession(first)
			laterRun := runtimebinding.HandleFromSession(later)
			keepRun := runtimebinding.HandleFromSession(keep)
			stopErr := errors.New("owned stop failed")
			var replacement *livesession.LiveSession
			err := runtimebinding.ShutdownOtherLiveSessions(state, keepRun, func(run factory.RuntimeRun) error {
				if run != firstRun && run != laterRun {
					t.Fatal("shutdown stopped a replacement or the excluded peer")
				}
				if run == firstRun {
					id := first.ID
					if phase == "later session" {
						id = later.ID
					}
					replacement = registerTestSession(state, id)
				}
				run.CancelRun()
				if err := run.Wait(); err != nil {
					return err
				}
				if run == firstRun && phase == "stop failure" {
					return stopErr
				}
				return nil
			})
			assertShutdownCapturedGenerations(t, state, keep, replacement, firstRun, laterRun, keepRun, phase, err, stopErr)

		})
	}
}

type replacementFactory struct {
	factory.Service
}

func TestWorkAndEventIngressRequiresDeclaredSessionCapability(t *testing.T) {
	t.Parallel()
	ingress := struct{ factory.APIFactory }{}
	runtime := &factorysessions.LiveRuntime{
		Factory: struct {
			factory.Service
			factory.APIFactory
		}{},
	}
	if got, ok := runtimebinding.WorkAndEventIngressForLiveRuntime(runtime); ok || got != nil {
		t.Fatalf("undeclared ingress = (%v, %v), want unavailable", got, ok)
	}
	runtime.WorkAndEventIngress = ingress
	if got, ok := runtimebinding.WorkAndEventIngressForLiveRuntime(runtime); !ok || got != ingress {
		t.Fatalf("declared ingress = (%v, %v), want declared capability", got, ok)
	}
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

func TestRegisterRetainsStartupFactsOnlyFromProvisionalOpening(t *testing.T) {
	for _, provisional := range []bool{true, false} {
		t.Run(map[bool]string{true: "initial startup", false: "already running"}[provisional], func(t *testing.T) {
			t.Parallel()
			sessions := newRuntimeBindingState()
			instance := &hostedInstanceFake{}
			recovery := &factorysessions.StartupRecovery{Code: "DURABLE_STATE_QUARANTINED", File: "board", Cause: "CORRUPT_STATE"}
			prior := &runtimebinding.SessionState{Instance: instance, StartupRecovery: recovery, SkippedBoardRecordings: []string{"legacy.jsonl"}}
			if !provisional {
				prior.Handle = newHostedHandleFake(instance)
			}
			sessions.Register(sessionruntime.Registration{SessionID: "selected", Handle: prior})
			runtimebinding.Register(sessions, runtimebinding.Registration{SessionID: "selected", Handle: newHostedHandleFake(instance)})
			bound := runtimebinding.SessionStateFrom(sessions.Resolve("selected"))
			if !provisional {
				if bound.StartupRecovery != nil || len(bound.SkippedBoardRecordings) != 0 {
					t.Fatal("runtime re-registration inherited initial-only observations")
				}
				return
			}
			if bound.StartupRecovery == nil || *bound.StartupRecovery != *recovery || bound.StartupRecovery == recovery ||
				len(bound.SkippedBoardRecordings) != 1 || bound.SkippedBoardRecordings[0] != "legacy.jsonl" {
				t.Fatalf("registered startup facts = %+v", bound)
			}
			recovery.Cause = "changed"
			prior.SkippedBoardRecordings[0] = "changed"
			if bound.StartupRecovery.Cause != "CORRUPT_STATE" || bound.SkippedBoardRecordings[0] != "legacy.jsonl" {
				t.Fatal("registration aliases provisional startup observations")
			}
		})
	}
}

func TestReplaceTransfersLiveSessionAndActiveRuntimeOwnership(t *testing.T) {
	oldCore, oldLogs := observer.New(zap.DebugLevel)
	currentCore, currentLogs := observer.New(zap.DebugLevel)
	sessions := newRuntimeBindingState()
	oldInstance := &hostedInstanceFake{logger: zap.New(oldCore)}
	oldHandle := newHostedHandleFake(oldInstance)
	recovery := &recordings.ResumeRecoveryMetadata{}
	warnings := []recordings.MetadataMismatchWarning{{}}
	clock := platformclock.Real{}
	preparedSpec := &struct{ Name string }{Name: "prepared"}
	sessions.Register(sessionruntime.Registration{
		SessionID: "session-1", FactoryDir: "/old", FolderPath: "/workspace",
		ExecutionBaseDir: "/old-execution", Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "alpha"},
		Handle: &runtimebinding.SessionState{Instance: oldInstance, Handle: oldHandle, Spec: preparedSpec, Clock: clock, Logger: oldInstance.RuntimeLogger(),
			ReplayMetadataWarnings: warnings, ResumeRecoveryMetadata: recovery, OperatorSettingsPath: "scoped.yaml", CurrentBoardRecordPath: "selected.jsonl"},
		Default: false, Project: "project", Select: true,
	})
	original := sessions.Resolve("session-1")
	original.RuntimeFactorySessionID = "runtime-session-1"
	var runtimeState runtimebinding.State
	runtimeState.SetActive(context.Background(), original.ID, oldHandle)
	replacement := &hostedInstanceFake{
		dir: "/new", service: replacementFactory{}, backendScope: "backend-new", logger: zap.New(currentCore),
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
	bound := runtimebinding.SessionStateFrom(updated)
	bound.Logger.Info("replacement session diagnostic")
	if oldLogs.Len() != 0 || currentLogs.FilterMessage("replacement session diagnostic").Len() != 1 {
		t.Fatalf("replacement diagnostic reached wrong generation: retired=%d current=%d", oldLogs.Len(), currentLogs.Len())
	}
	if bound.Clock != clock || bound.OperatorSettingsPath != "scoped.yaml" || bound.CurrentBoardRecordPath != "selected.jsonl" ||
		len(bound.ReplayMetadataWarnings) != 1 || bound.ResumeRecoveryMetadata == nil || bound.ResumeRecoveryMetadata == recovery {
		t.Fatalf("replacement lost scoped facts or failed to detach recovery metadata: %#v", bound)
	}
	bound.ReplayMetadataWarnings[0] = recordings.MetadataMismatchWarning{}
	if &bound.ReplayMetadataWarnings[0] == &warnings[0] {
		t.Fatal("replacement aliases prior warning slice")
	}

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

func registerTestSession(state *sessionruntime.Service, sessionID string) *livesession.LiveSession {
	instance := &hostedInstanceFake{}
	handle := newHostedHandleFake(instance)
	state.Register(sessionruntime.Registration{
		SessionID: sessionID,
		Handle:    &runtimebinding.SessionState{Instance: instance, Handle: handle},
	})
	return state.Resolve(sessionID)
}

type startupActivationClose func(context.Context) error

func (close startupActivationClose) Close(ctx context.Context) error { return close(ctx) }

func TestFailStartupClosesActivationBeforeRetirementAndRetainsIncompleteCleanup(t *testing.T) {
	t.Parallel()
	startupErr := errors.New("listener failed")
	cleanupErr := errors.New("cleanup failed")
	for _, phase := range []string{"success", "stop failure", "joined stop failure", "activation failure"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			sessions := newRuntimeBindingState()
			session := registerTestSession(sessions, "failed-start")
			handle := runtimebinding.HandleFromSession(session)
			var active runtimebinding.State
			active.SetActive(t.Context(), session.ID, handle)
			closed := false
			runtimebinding.SessionStateFrom(session).Activation = startupActivationClose(func(ctx context.Context) error {
				assertStartupCleanupBeforeRetirement(t, ctx, handle, sessions.Resolve(session.ID))
				closed = true
				if phase == "activation failure" {
					return cleanupErr
				}
				return nil
			})
			stop := func(got factory.RuntimeRun) error {
				if got != handle {
					t.Fatal("startup rollback stopped another session")
				}
				if phase == "stop failure" {
					return cleanupErr
				}
				got.CancelRun()
				if phase == "joined stop failure" {
					return errors.Join(got.Wait(), cleanupErr)
				}
				return got.Wait()
			}
			err := runtimebinding.FailStartup(sessions, &active, session.ID, handle, stop, startupErr)
			if !errors.Is(err, startupErr) || active.Active() != nil {
				t.Fatalf("rollback error = %v, active = %#v; want original failure and no active selection", err, active.Active())
			}
			if phase == "success" || phase == "joined stop failure" {
				if !closed || sessions.Resolve(session.ID) != nil || errors.Is(err, cleanupErr) != (phase == "joined stop failure") {
					t.Fatal("successful rollback did not clean and retire its session")
				}
				return
			}
			if !errors.Is(err, cleanupErr) || sessions.Resolve(session.ID) == nil || closed != (phase == "activation failure") {
				t.Fatalf("incomplete rollback error = %v, closed = %t; want retained record and cleanup failure", err, closed)
			}
		})
	}
}

func assertStartupCleanupBeforeRetirement(t *testing.T, ctx context.Context, handle factory.RuntimeRun, session *livesession.LiveSession) {
	t.Helper()
	if ctx.Err() != nil || !handle.Completed() || session == nil {
		t.Fatal("activation cleanup must follow runtime join and precede retirement with a live cleanup context")
	}
}

func TestFailStartupLeavesSelectedPeerUsable(t *testing.T) {
	t.Parallel()
	sessions := newRuntimeBindingState()
	failed := registerTestSession(sessions, "a")
	peer := registerTestSession(sessions, "b")
	var active runtimebinding.State
	active.SetActive(t.Context(), peer.ID, runtimebinding.HandleFromSession(peer))
	startupErr := errors.New("startup failed")
	err := runtimebinding.FailStartup(sessions, &active, failed.ID, runtimebinding.HandleFromSession(failed), func(run factory.RuntimeRun) error {
		run.CancelRun()
		return run.Wait()
	}, startupErr)
	if !errors.Is(err, startupErr) || sessions.Resolve("a") != nil || sessions.Resolve("b") != peer {
		t.Fatalf("failed startup retirement = %v, want only A removed", err)
	}
	if selected := active.Active(); selected == nil || selected.Handle != runtimebinding.HandleFromSession(peer) || selected.Context != t.Context() {
		t.Fatalf("peer lost active selection: %#v", selected)
	}
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

}

func assertShutdownCapturedGenerations(t *testing.T, state *sessionruntime.Service, keep, replacement *livesession.LiveSession, firstRun, laterRun, keepRun factory.RuntimeRun, phase string, err, stopErr error) {
	t.Helper()
	if errors.Is(err, stopErr) != (phase == "stop failure") || (err != nil && phase != "stop failure") {
		t.Fatalf("shutdown error = %v, want only the injected owned stop error", err)
	}
	if !firstRun.Completed() || !laterRun.Completed() {
		t.Fatal("shutdown did not join both captured generations")
	}
	if state.Resolve(replacement.ID) != replacement || state.Resolve(keep.ID) != keep || state.Registry().Count() != 2 {
		t.Fatal("shutdown retired a replacement or peer, or retained an old generation")
	}
	if keepRun.Completed() || runtimebinding.HandleFromSession(replacement).Completed() {
		t.Fatal("surviving replacement or peer was canceled")
	}
	if _, err := state.ResponseStreams().Streams(replacement.ID).Subscribe("next", 0); err != nil {
		t.Fatalf("replacement response stream is unusable: %v", err)
	}
	assertNextShutdownRetiresReplacement(t, state, keep, replacement, keepRun)

}

func assertNextShutdownRetiresReplacement(t *testing.T, state *sessionruntime.Service, keep, replacement *livesession.LiveSession, keepRun factory.RuntimeRun) {
	t.Helper()
	// A subsequent shutdown owns the replacement; the captured-generation
	// fence must protect it only from the earlier shutdown window.
	if err := runtimebinding.ShutdownOtherLiveSessions(state, keepRun, func(run factory.RuntimeRun) error {
		if run != runtimebinding.HandleFromSession(replacement) {
			t.Fatal("next shutdown selected a foreign generation")
		}
		run.CancelRun()
		return run.Wait()
	}); err != nil {
		t.Fatal(err)
	}
	if state.Resolve(replacement.ID) != nil || !runtimebinding.HandleFromSession(replacement).Completed() || state.Resolve(keep.ID) != keep {
		t.Fatal("next shutdown did not retire only its owned replacement")
	}
}

func TestOpeningMetadataRemainsDetachedAndClearsAbsentFacts(t *testing.T) {
	t.Parallel()
	selected, peer := &runtimebinding.SessionState{}, &runtimebinding.SessionState{}
	resume := &recordings.ResumeRecoveryMetadata{SourceRecordingID: "source"}
	recovery := &factorysessions.StartupRecovery{Cause: "quarantined"}
	metadata := runtimebinding.OpeningMetadata{
		CurrentBoardRecordPath: "selected.recording", OperatorSettingsPath: "selected.operator",
		SkippedBoardRecordings: []string{"selected.skipped"},
		ReplayMetadataWarnings: []recordings.MetadataMismatchWarning{{Key: "selected"}},
		ResumeRecoveryMetadata: resume, StartupRecovery: recovery,
	}
	var opening runtimebinding.OpeningState = selected
	opening.SetOpeningMetadata(metadata)
	peer.SetOpeningMetadata(runtimebinding.OpeningMetadata{})
	metadata.SkippedBoardRecordings[0] = "changed"
	metadata.ReplayMetadataWarnings[0].Key = "changed"
	resume.SourceRecordingID, recovery.Cause = "changed", "changed"
	if selected.CurrentBoardRecordPath != "selected.recording" || selected.OperatorSettingsPath != "selected.operator" ||
		selected.SkippedBoardRecordings[0] != "selected.skipped" || selected.ReplayMetadataWarnings[0].Key != "selected" ||
		selected.ResumeRecoveryMetadata.SourceRecordingID != "source" || selected.StartupRecovery.Cause != "quarantined" || peer.ResumeRecoveryMetadata != nil {
		t.Fatal("opening metadata aliases caller or peer state")
	}
	opening.SetOpeningMetadata(runtimebinding.OpeningMetadata{})
	if selected.CurrentBoardRecordPath != "" || selected.OperatorSettingsPath != "" || selected.SkippedBoardRecordings != nil ||
		selected.ReplayMetadataWarnings != nil || selected.ResumeRecoveryMetadata != nil || selected.StartupRecovery != nil {
		t.Fatal("absent opening metadata retained previous facts")
	}
}
