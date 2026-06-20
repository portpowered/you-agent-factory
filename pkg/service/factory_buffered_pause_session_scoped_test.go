package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestFactoryService_PausedSessionBufferedSubmission_DoesNotAffectOtherSessions(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	alphaSession := harness.requireSession(t, defaultFactorySessionID)
	betaSession := harness.requireSession(t, betaSessionID)

	pauseSessionFactory(t, betaSession)
	waitForSessionFactoryState(t, harness.svc, betaSession.ID, interfaces.FactoryStatePaused, time.Second, "beta session paused")
	waitForSessionFactoryState(t, harness.svc, alphaSession.ID, interfaces.FactoryStateRunning, time.Second, "alpha session still running")

	submitSessionWork(t, betaSession, "beta-paused-submit-work", "trace-beta-paused-submit")
	submitSessionWork(t, alphaSession, "alpha-running-submit-work", "trace-alpha-running-submit")

	waitForSessionEventsToContain(t, alphaSession, "alpha-running-submit-work", time.Second)
	assertSessionEventsDoNotContain(t, betaSession, "beta-paused-submit-work")

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		betaSnap := sessionEngineSnapshot(t, betaSession)
		if betaSnap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("beta factory state = %q, want PAUSED", betaSnap.FactoryState)
		}
		if snapshotHasTokenAtPlace(betaSnap, "task:complete") || snapshotHasTokenAtPlace(betaSnap, "task:init") {
			t.Fatalf("paused beta submission applied to marking = %#v", betaSnap.Marking.Tokens)
		}
		if betaSnap.InFlightCount > 0 || len(betaSnap.Dispatches) > 0 {
			t.Fatalf("beta dispatch started while paused inFlight=%d dispatches=%d", betaSnap.InFlightCount, len(betaSnap.Dispatches))
		}
		time.Sleep(20 * time.Millisecond)
	}

	resumeSessionFactory(t, betaSession)
	waitForSessionFactoryState(t, harness.svc, betaSession.ID, interfaces.FactoryStateRunning, time.Second, "beta session resumed")
	waitForSessionEventsToContain(t, betaSession, "beta-paused-submit-work", time.Second)
	assertSessionEventsDoNotContain(t, betaSession, "alpha-running-submit-work")
}

func TestFactoryService_PausedSessionBufferedWorkerResult_DoesNotAffectOtherSessions(t *testing.T) {
	blocking := &prefixBlockingExecutor{
		blockPrefix: "beta-blocked-",
		started:     make(chan struct{}),
		release:     make(chan struct{}),
	}

	rootDir := t.TempDir()
	secondDir := t.TempDir()
	writeFactoryJSON(t, rootDir, minimalFactoryConfig())
	writeFactoryJSON(t, secondDir, minimalFactoryConfig())

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		ExtraOptions: []factory.FactoryOption{
			factory.WithWorkerExecutor("worker-a", blocking),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()
	defer func() {
		cancelRun()
		select {
		case err := <-runErrCh:
			if err != nil {
				t.Fatalf("Run after cancellation: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for service shutdown")
		}
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")
	openResult, err := svc.OpenFactorySessionFromFolder(context.Background(), secondDir, nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}

	alphaSession := requireLiveSession(t, svc, defaultFactorySessionID)
	betaSession := requireLiveSession(t, svc, openResult.SessionID)

	submitSessionWork(t, betaSession, "beta-blocked-result-work", "trace-beta-blocked-result")
	waitForSessionInFlight(t, betaSession, time.Second)

	pauseSessionFactory(t, betaSession)
	waitForSessionFactoryState(t, svc, betaSession.ID, interfaces.FactoryStatePaused, time.Second, "beta session paused")
	close(blocking.release)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		betaSnap := sessionEngineSnapshot(t, betaSession)
		if betaSnap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("beta factory state = %q, want PAUSED", betaSnap.FactoryState)
		}
		if snapshotHasTokenAtPlace(betaSnap, "task:complete") {
			t.Fatalf("beta worker result applied while paused")
		}
		if betaSnap.InFlightCount == 0 {
			t.Fatalf("beta dispatch completed while paused")
		}
		time.Sleep(20 * time.Millisecond)
	}

	submitSessionWork(t, alphaSession, "alpha-running-result-work", "trace-alpha-running-result")
	waitForSessionEventsToContain(t, alphaSession, "alpha-running-result-work", time.Second)
	assertSessionEventsDoNotContain(t, betaSession, "alpha-running-result-work")
	betaSnap := sessionEngineSnapshot(t, betaSession)
	if snapshotHasTokenAtPlace(betaSnap, "task:complete") {
		t.Fatalf("beta worker result applied while alpha session processed normally")
	}

	resumeSessionFactory(t, betaSession)
	waitForSessionFactoryState(t, svc, betaSession.ID, interfaces.FactoryStateRunning, time.Second, "beta session resumed")
	waitForSessionEventsToContain(t, betaSession, "beta-blocked-result-work", time.Second)
	assertSessionEventsDoNotContain(t, betaSession, "alpha-running-result-work")
}

type prefixBlockingExecutor struct {
	blockPrefix string
	started     chan struct{}
	release     chan struct{}
}

func (e *prefixBlockingExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	workID := ""
	if len(dispatch.Execution.WorkIDs) > 0 {
		workID = dispatch.Execution.WorkIDs[0]
	}
	if strings.HasPrefix(workID, e.blockPrefix) {
		select {
		case e.started <- struct{}{}:
		default:
		}
		<-e.release
	}
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "done",
	}, nil
}

func pauseSessionFactory(t *testing.T, session *liveFactorySession) {
	t.Helper()
	if err := liveSessionHandle(session).runtime.factory.Pause(context.Background()); err != nil {
		t.Fatalf("Pause(%s): %v", session.ID, err)
	}
}

func resumeSessionFactory(t *testing.T, session *liveFactorySession) {
	t.Helper()
	if err := liveSessionHandle(session).runtime.factory.Resume(context.Background()); err != nil {
		t.Fatalf("Resume(%s): %v", session.ID, err)
	}
}

func requireLiveSession(t *testing.T, svc *FactoryService, sessionID string) *liveFactorySession {
	t.Helper()
	session := svc.sessionByID(sessionID)
	if session == nil {
		t.Fatalf("session %q is not registered", sessionID)
	}
	return session
}

func sessionEngineSnapshot(t *testing.T, session *liveFactorySession) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).runtime == nil {
		t.Fatal("live session runtime is required")
	}
	snap, err := liveSessionHandle(session).runtime.factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot(%s): %v", session.ID, err)
	}
	return snap
}

func snapshotHasTokenAtPlace(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) bool {
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID == placeID {
			return true
		}
	}
	return false
}

func waitForSessionInFlight(t *testing.T, session *liveFactorySession, wait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		snap := sessionEngineSnapshot(t, session)
		if snap.InFlightCount > 0 && len(snap.Dispatches) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap := sessionEngineSnapshot(t, session)
	t.Fatalf("timed out waiting for in-flight dispatch on session %s inFlight=%d dispatches=%d", session.ID, snap.InFlightCount, len(snap.Dispatches))
}
