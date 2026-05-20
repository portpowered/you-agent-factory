package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestFactoryService_OpenFactorySession_RunsConcurrentIsolatedSessions(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default alpha runtime")

	betaSessionOne, err := svc.openFactorySession(context.Background(), betaDir)
	if err != nil {
		t.Fatalf("openFactorySession(beta one): %v", err)
	}
	betaSessionTwo, err := svc.openFactorySession(context.Background(), betaDir)
	if err != nil {
		t.Fatalf("openFactorySession(beta two): %v", err)
	}
	if betaSessionOne == betaSessionTwo {
		t.Fatalf("duplicate beta session ids = %q", betaSessionOne)
	}
	if got := svc.sessions.count(); got != 3 {
		t.Fatalf("live session count = %d, want 3", got)
	}

	defaultSession := svc.sessionByID(defaultFactorySessionID)
	firstBeta := svc.sessionByID(betaSessionOne)
	secondBeta := svc.sessionByID(betaSessionTwo)
	if defaultSession == nil || firstBeta == nil || secondBeta == nil {
		t.Fatalf("expected default and beta sessions to be registered, got ids %v", svc.sessions.ids())
	}
	if defaultSession.handle == firstBeta.handle || firstBeta.handle == secondBeta.handle {
		t.Fatal("expected each live session to own a distinct runtime handle")
	}
	if defaultSession.handle.runtime.dir != alphaDir {
		t.Fatalf("default runtime dir = %q, want %q", defaultSession.handle.runtime.dir, alphaDir)
	}
	if firstBeta.handle.runtime.dir != betaDir || secondBeta.handle.runtime.dir != betaDir {
		t.Fatalf("beta runtime dirs = %q and %q, want %q", firstBeta.handle.runtime.dir, secondBeta.handle.runtime.dir, betaDir)
	}

	submitSessionWork(t, defaultSession, "alpha-session-work", "trace-alpha-session")
	submitSessionWork(t, firstBeta, "beta-session-one-work", "trace-beta-session-one")
	submitSessionWork(t, secondBeta, "beta-session-two-work", "trace-beta-session-two")

	waitForSessionEventsToContain(t, defaultSession, "alpha-session-work", time.Second)
	waitForSessionEventsToContain(t, firstBeta, "beta-session-one-work", time.Second)
	waitForSessionEventsToContain(t, secondBeta, "beta-session-two-work", time.Second)

	assertSessionEventsDoNotContain(t, defaultSession, "beta-session-one-work")
	assertSessionEventsDoNotContain(t, defaultSession, "beta-session-two-work")
	assertSessionEventsDoNotContain(t, firstBeta, "alpha-session-work")
	assertSessionEventsDoNotContain(t, firstBeta, "beta-session-two-work")
	assertSessionEventsDoNotContain(t, secondBeta, "alpha-session-work")
	assertSessionEventsDoNotContain(t, secondBeta, "beta-session-one-work")

	if err := svc.stopFactorySession(betaSessionOne); err != nil {
		t.Fatalf("stopFactorySession(beta one): %v", err)
	}
	select {
	case <-firstBeta.handle.runDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stopped beta session to exit")
	}
	if got := svc.sessions.count(); got != 2 {
		t.Fatalf("live session count after stop = %d, want 2", got)
	}
	if svc.sessionByID(betaSessionOne) != nil {
		t.Fatalf("stopped beta session %q is still registered", betaSessionOne)
	}
	assertSessionRemainsLive(t, svc, betaSessionTwo, 200*time.Millisecond, "remaining beta runtime")
	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime after beta stop")

	cancel()
	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func waitForSessionRuntimeStatus(
	t *testing.T,
	svc *FactoryService,
	sessionID string,
	want interfaces.RuntimeStatus,
	wait time.Duration,
	label string,
) {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		session := svc.sessionByID(sessionID)
		if session != nil && session.handle != nil && session.handle.runtime != nil {
			snap, err := session.handle.runtime.factory.GetEngineStateSnapshot(context.Background())
			if err == nil && snap.RuntimeStatus == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to reach runtime status %s", label, want)
}

func submitSessionWork(t *testing.T, session *liveFactorySession, workID, traceID string) {
	t.Helper()

	if session == nil || session.handle == nil || session.handle.runtime == nil {
		t.Fatal("live session runtime is required")
	}
	request := factory.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
		WorkID:     workID,
		Name:       workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"` + workID + `"}`),
	}})
	if _, err := session.handle.runtime.factory.SubmitWorkRequest(context.Background(), request); err != nil {
		t.Fatalf("SubmitWorkRequest(%s): %v", workID, err)
	}
}

func assertSessionRemainsLive(t *testing.T, svc *FactoryService, sessionID string, wait time.Duration, label string) {
	t.Helper()

	session := svc.sessionByID(sessionID)
	if session == nil || session.handle == nil {
		t.Fatalf("%s is not registered", label)
	}
	select {
	case <-session.handle.runDone:
		t.Fatalf("%s stopped unexpectedly", label)
	case <-time.After(wait):
	}
}

func waitForSessionEventsToContain(t *testing.T, session *liveFactorySession, want string, wait time.Duration) {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if sessionEventsContain(t, session, want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session events to contain %q", want)
}

func assertSessionEventsDoNotContain(t *testing.T, session *liveFactorySession, want string) {
	t.Helper()
	if sessionEventsContain(t, session, want) {
		t.Fatalf("session events unexpectedly contained %q", want)
	}
}

func sessionEventsContain(t *testing.T, session *liveFactorySession, want string) bool {
	t.Helper()

	if session == nil || session.handle == nil || session.handle.runtime == nil {
		t.Fatal("live session runtime is required")
	}
	events, err := session.handle.runtime.factory.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("Marshal(events): %v", err)
	}
	return strings.Contains(string(payload), want)
}
