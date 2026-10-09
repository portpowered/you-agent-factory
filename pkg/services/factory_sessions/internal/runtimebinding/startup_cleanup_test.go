package runtimebinding_test

import (
	"context"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

func TestStartUnpublishedFailureJoinsRunAndPreservesPeerForRetry(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"readiness", "sidecars"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			sessions := newRuntimeBindingState()
			peer := registerTestSession(sessions, "peer")
			var active runtimebinding.State
			active.SetActive(t.Context(), peer.ID, runtimebinding.HandleFromSession(peer))
			startupErr := errors.New("injected " + phase + " failure")
			failed := true
			var acquired factory.RuntimeRun
			lifecycle := startupReadinessLifecycle{readiness: func(_ context.Context, run factory.RuntimeRun) error {
				acquired = run
				if failed && phase == "readiness" {
					return startupErr
				}
				return nil
			}}
			startSidecars := func(context.Context, factory.RuntimeRun) error {
				if failed && phase == "sidecars" {
					return startupErr
				}
				return nil
			}
			stop := func(run factory.RuntimeRun) error {
				run.CancelRun()
				return run.Wait()
			}
			bundle := &hostedInstanceFake{dir: "/factory", service: replacementFactory{}}
			start := func() (factory.RuntimeRun, error) {
				return runtimebinding.Start(t.Context(), sessions, &active, "/factory", "retry", bundle,
					factorysessions.Target{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "retry"}},
					true, lifecycle, startSidecars, stop)
			}
			run, err := start()
			if run != nil || !errors.Is(err, startupErr) {
				t.Fatalf("failed start = (%v, %v), want original startup error and no run", run, err)
			}
			if acquired == nil || !acquired.Completed() || sessions.Resolve("retry") != nil {
				t.Fatal("failed unpublished startup must join its acquired run without publishing a ghost")
			}
			if runtimebinding.HandleFromSession(sessions.Resolve(peer.ID)).Completed() || active.Active().SessionID != peer.ID {
				t.Fatal("failed startup canceled or displaced the live peer")
			}
			assertStartupRetryPreservesPeer(t, &failed, start, sessions, &active, peer, stop)

		})
	}
}

func TestStartInitialFailurePreservesReplacementAndPeerSelection(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"replacement during readiness", "peer during readiness", "replacement during stop", "closed service with peer"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			sessions := newRuntimeBindingState()
			peer := registerTestSession(sessions, "peer")
			var active runtimebinding.State
			startupErr := errors.New("initial readiness failed")
			peerCtx, cancelPeer := context.WithCancel(context.Background())
			defer cancelPeer()
			var expected *livesession.LiveSession
			var failed factory.RuntimeRun
			selectSurvivor := func(session *livesession.LiveSession) {
				expected = session
				active.SetActive(peerCtx, session.ID, runtimebinding.HandleFromSession(session))
			}
			lifecycle := startupReadinessLifecycle{readiness: func(_ context.Context, run factory.RuntimeRun) error {
				failed = run
				switch phase {
				case "replacement during readiness":
					selectSurvivor(registerTestSession(sessions, "initial"))
				case "peer during readiness":
					selectSurvivor(peer)
				case "closed service with peer":
					sessions.Unregister("initial")
					selectSurvivor(peer)
				}
				return startupErr
			}}
			stop := func(run factory.RuntimeRun) error {
				if run != failed {
					t.Fatal("startup rollback stopped a surviving run")
				}
				run.CancelRun()
				<-run.RunDoneCh()
				if phase == "replacement during stop" {
					selectSurvivor(registerTestSession(sessions, "initial"))
				}
				return nil
			}
			removed := false
			run, err := runtimebinding.StartInitial(
				context.Background(), context.Background(), sessions, &active,
				"initial", "/factory", &hostedInstanceFake{}, factorysessions.Target{},
				interfaces.RuntimeModeService, lifecycle, stop, func(string) { removed = true },
			)
			if selected := active.Active(); selected == nil || selected.Context != peerCtx {
				t.Fatal("startup rollback changed the surviving selection context")
			}
			assertInitialStartupRollback(t, phase, run, failed, err, startupErr, removed, &active, sessions, peer, expected, lifecycle)

		})
	}
}

func TestFailStartupPreservesReplacementAcrossCleanup(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"before rollback", "during stop", "during activation", "activation failure"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			sessions := newRuntimeBindingState()
			failed := registerTestSession(sessions, "a")
			peer := registerTestSession(sessions, "b")
			handle := runtimebinding.HandleFromSession(failed)
			var active runtimebinding.State
			active.SetActive(t.Context(), failed.ID, handle)
			startupErr := errors.New("startup failed")
			cleanupErr := errors.New("activation cleanup failed")
			closed := false
			var replacement *livesession.LiveSession
			publish := func() {
				replacement = registerTestSession(sessions, failed.ID)
				active.SetActive(t.Context(), replacement.ID, runtimebinding.HandleFromSession(replacement))
				runtimebinding.SessionStateFrom(replacement).Activation = startupActivationClose(func(context.Context) error {
					t.Error("failed startup closed replacement activation")
					return nil
				})
			}
			runtimebinding.SessionStateFrom(failed).Activation = startupActivationClose(func(ctx context.Context) error {
				if ctx.Err() != nil || !handle.Completed() {
					t.Fatal("activation closed before failed run joined or with canceled context")
				}
				closed = true
				if phase == "during activation" || phase == "activation failure" {
					publish()
				}
				if phase == "activation failure" {
					return cleanupErr
				}
				return nil
			})
			if phase == "before rollback" {
				publish()
			}
			err := runtimebinding.FailStartup(sessions, &active, failed.ID, handle, func(got factory.RuntimeRun) error {
				if got != handle {
					t.Fatal("rollback stopped a foreign run")
				}
				if phase == "during stop" {
					publish()
				}
				got.CancelRun()
				return got.Wait()
			}, startupErr)
			assertStartupReplacementSurvives(t, phase, err, startupErr, cleanupErr, closed, handle, sessions, replacement, peer, &active)

		})
	}
}

func assertStartupRetryPreservesPeer(t *testing.T, failed *bool, start func() (factory.RuntimeRun, error), sessions *sessionruntime.Service, active *runtimebinding.State, peer *livesession.LiveSession, stop func(factory.RuntimeRun) error) {
	t.Helper()
	*failed = false
	run, err := start()
	if err != nil || run == nil || sessions.Resolve("retry") == nil {
		t.Fatalf("retry = (%v, %v), want published live run", run, err)
	}
	if err := runtimebinding.StopSessionGeneration(sessions, active, sessions.Resolve("retry"), stop); err != nil {
		t.Fatalf("close retry: %v", err)
	}
	if !run.Completed() || sessions.Resolve("retry") != nil || active.Active().SessionID != peer.ID {
		t.Fatal("retry close did not join and retire only its own run")
	}
	if err := runtimebinding.StopSessionGeneration(sessions, active, peer, stop); err != nil {
		t.Fatalf("close peer: %v", err)
	}
}

func assertInitialStartupRollback(t *testing.T, phase string, run, failed factory.RuntimeRun, err, startupErr error, removed bool, active *runtimebinding.State, sessions *sessionruntime.Service, peer, expected *livesession.LiveSession, lifecycle startupReadinessLifecycle) {
	t.Helper()
	if run != nil || (phase == "closed service with peer" && err != nil) ||
		(phase != "closed service with peer" && !errors.Is(err, startupErr)) {
		t.Fatalf("failed initial start = (%v, %v)", run, err)
	}
	if failed == nil || !failed.Completed() {
		t.Fatal("failed run was not joined")
	}
	if phase == "replacement during readiness" && removed {
		t.Fatal("startup rollback invoked replacement removal effects")
	}
	assertInitialStartupSurvivors(t, active, sessions, peer, expected, lifecycle)

}

func assertInitialStartupSurvivors(t *testing.T, active *runtimebinding.State, sessions *sessionruntime.Service, peer, expected *livesession.LiveSession, lifecycle startupReadinessLifecycle) {
	t.Helper()
	selected := active.Active()
	if selected == nil || selected.Handle != runtimebinding.HandleFromSession(expected) {
		t.Fatalf("survivor selection = %#v", selected)
	}
	for _, session := range []*livesession.LiveSession{peer, expected} {
		if sessions.Resolve(session.ID) != session || runtimebinding.HandleFromSession(session).Completed() {
			t.Fatal("startup rollback retired or canceled a survivor")
		}
		if _, err := sessions.ResponseStreams().Streams(session.ID).Subscribe("next", 0); err != nil {
			t.Fatalf("survivor response stream: %v", err)
		}
	}
	if err := runtimebinding.StopSessionGeneration(sessions, active, expected, lifecycle.Stop); err != nil {
		t.Fatalf("close surviving selection: %v", err)
	}
	if expected != peer {
		if err := runtimebinding.StopSessionGeneration(sessions, active, peer, lifecycle.Stop); err != nil {
			t.Fatalf("close surviving peer: %v", err)
		}
	}
}

func assertStartupReplacementSurvives(t *testing.T, phase string, err, startupErr, cleanupErr error, closed bool, handle factory.RuntimeRun, sessions *sessionruntime.Service, replacement, peer *livesession.LiveSession, active *runtimebinding.State) {
	t.Helper()
	if !errors.Is(err, startupErr) || errors.Is(err, cleanupErr) != (phase == "activation failure") {
		t.Fatalf("rollback error = %v, want original startup and selected cleanup error", err)
	}
	if closed != (phase != "before rollback") || !handle.Completed() {
		t.Fatalf("failed run joined = %t, owned activation closed = %t", handle.Completed(), closed)
	}
	if sessions.Resolve("a") != replacement || sessions.Resolve("b") != peer {
		t.Fatal("rollback retired replacement or peer")
	}
	if selected := active.Active(); selected == nil || selected.Handle != runtimebinding.HandleFromSession(replacement) || selected.Context != t.Context() {
		t.Fatalf("replacement lost active selection: %#v", selected)
	}
	if _, err := sessions.ResponseStreams().Streams("a").Subscribe("next", 0); err != nil {
		t.Fatalf("replacement response stream closed: %v", err)
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

func TestStartInitialPreservesCancellationAfterStartupCleanup(t *testing.T) {
	sessions := newRuntimeBindingState()
	var runtimeState runtimebinding.State
	bundle := &hostedInstanceFake{dir: "/factory", service: replacementFactory{}}
	readinessCtx, cancel := context.WithCancel(context.Background())
	cancel()

	handle, err := runtimebinding.StartInitial(
		readinessCtx, context.Background(), sessions, &runtimeState,
		factorysessions.DefaultSessionID, "/factory", bundle,
		factorysessions.Target{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}},
		interfaces.RuntimeModeBatch, canceledReadinessLifecycle{},
		func(handle factory.RuntimeRun) error { handle.CancelRun(); return nil }, nil,
	)
	if handle != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("StartInitial after canceled readiness = (%#v, %v), want nil handle and cancellation", handle, err)
	}
	if sessions.Resolve(factorysessions.DefaultSessionID) != nil || runtimeState.ActiveHandle() != nil {
		t.Fatal("canceled startup retained an active Factory Session")
	}
}

func TestStartInitialRegistersExplicitSessionWithoutDefaultAlias(t *testing.T) {
	t.Parallel()

	const sessionID = "session-explicit"
	sessions := newRuntimeBindingState()
	var runtimeState runtimebinding.State
	bundle := &hostedInstanceFake{dir: "/factory", service: replacementFactory{}}
	const sourceSessionID = "recorded-source-session"
	sessions.Register(sessionruntime.Registration{
		SessionID: sessionID, RuntimeFactorySessionID: sessionID,
		RuntimeEventSessionID: sourceSessionID,
		Handle:                &runtimebinding.SessionState{Instance: bundle},
	})
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
	if session.RuntimeEventSessionID != sourceSessionID || session.RuntimeFactorySessionID != sessionID {
		t.Fatalf("running successor lost prepared identity: runtime=%q events=%q", session.RuntimeFactorySessionID, session.RuntimeEventSessionID)
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
