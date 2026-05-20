package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
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

func TestFactoryService_OpenFactorySession_IsolatesSessionLogsAndReplayArtifacts(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	logDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RuntimeLogDir:     logDir,
		RecordPath:        recordPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
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

	defaultSession := svc.sessionByID(defaultFactorySessionID)
	firstBeta := svc.sessionByID(betaSessionOne)
	secondBeta := svc.sessionByID(betaSessionTwo)
	if defaultSession == nil || firstBeta == nil || secondBeta == nil {
		t.Fatalf("expected default and beta sessions to be registered, got ids %v", svc.sessions.ids())
	}

	submitSessionWork(t, defaultSession, "alpha-session-work", "trace-alpha-session")
	submitSessionWork(t, firstBeta, "beta-session-one-work", "trace-beta-session-one")
	submitSessionWork(t, secondBeta, "beta-session-two-work", "trace-beta-session-two")

	waitForSessionEventsToContain(t, defaultSession, "alpha-session-work", time.Second)
	waitForSessionEventsToContain(t, firstBeta, "beta-session-one-work", time.Second)
	waitForSessionEventsToContain(t, secondBeta, "beta-session-two-work", time.Second)

	cancel()
	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service shutdown")
	}

	sessions := []*liveFactorySession{defaultSession, firstBeta, secondBeta}
	wantWorkBySession := map[string]string{
		defaultFactorySessionID: "alpha-session-work",
		betaSessionOne:          "beta-session-one-work",
		betaSessionTwo:          "beta-session-two-work",
	}

	if defaultSession.handle.runtime.recordPath != recordPath {
		t.Fatalf("default record path = %q, want %q", defaultSession.handle.runtime.recordPath, recordPath)
	}
	if firstBeta.handle.runtime.recordPath == "" || secondBeta.handle.runtime.recordPath == "" {
		t.Fatalf("background record paths must be set, got %q and %q", firstBeta.handle.runtime.recordPath, secondBeta.handle.runtime.recordPath)
	}
	if firstBeta.handle.runtime.recordPath == secondBeta.handle.runtime.recordPath {
		t.Fatalf("background sessions shared record path %q", firstBeta.handle.runtime.recordPath)
	}

	for _, session := range sessions {
		if session == nil || session.handle == nil || session.handle.runtime == nil {
			t.Fatal("expected live session runtime")
		}
		runtimeBundle := session.handle.runtime
		artifact, err := replay.Load(runtimeBundle.recordPath)
		if err != nil {
			t.Fatalf("Load(%s): %v", runtimeBundle.recordPath, err)
		}
		payload, err := json.Marshal(artifact.Events)
		if err != nil {
			t.Fatalf("Marshal(%s events): %v", runtimeBundle.recordPath, err)
		}
		wantWork := wantWorkBySession[session.id]
		if !strings.Contains(string(payload), wantWork) {
			t.Fatalf("artifact %s did not contain session work %q: %s", runtimeBundle.recordPath, wantWork, string(payload))
		}
		for otherSessionID, otherWork := range wantWorkBySession {
			if otherSessionID == session.id {
				continue
			}
			if strings.Contains(string(payload), otherWork) {
				t.Fatalf("artifact %s leaked work %q from session %s: %s", runtimeBundle.recordPath, otherWork, otherSessionID, string(payload))
			}
		}

		logPath := runtimeBundle.logSink.Path()
		if logPath == "" {
			t.Fatalf("session %s runtime log path is empty", session.id)
		}
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read runtime log %s: %v", logPath, err)
		}
		records := parseRuntimeLogRecords(t, string(data))
		foundSessionRecord := false
		for _, record := range records {
			if record["session_id"] != session.id {
				continue
			}
			foundSessionRecord = true
			if record["folder_path"] != runtimeBundle.folderPath {
				t.Fatalf("session %s folder_path = %#v, want %q in %#v", session.id, record["folder_path"], runtimeBundle.folderPath, record)
			}
			if record["runtime_instance_id"] == "" {
				t.Fatalf("session %s runtime_instance_id missing in %#v", session.id, record)
			}
		}
		if !foundSessionRecord {
			t.Fatalf("runtime log %s did not contain any records for session %s:\n%s", logPath, session.id, string(data))
		}
	}
}

func TestFactoryService_CloseFactorySession_ClosesDefaultAndPromotesRemainingSession(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
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

	betaSessionID, err := svc.openFactorySession(context.Background(), betaDir)
	if err != nil {
		t.Fatalf("openFactorySession(beta): %v", err)
	}
	waitForSessionRuntimeStatus(t, svc, betaSessionID, interfaces.RuntimeStatusIdle, time.Second, "beta runtime")

	defaultSession := svc.sessionByID(defaultFactorySessionID)
	if defaultSession == nil {
		t.Fatal("expected default session to be registered")
	}

	if err := svc.CloseFactorySession(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("CloseFactorySession(default): %v", err)
	}

	select {
	case <-defaultSession.handle.runDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed default session to exit")
	}
	if svc.sessionByID(defaultFactorySessionID) != nil {
		t.Fatal("default session remained registered after close")
	}
	if current := svc.currentRunState(); current == nil || current.sessionID != betaSessionID {
		t.Fatalf("current run state = %#v, want beta session %q", current, betaSessionID)
	}
	assertSessionRemainsLive(t, svc, betaSessionID, 200*time.Millisecond, "beta runtime after default close")

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

func TestFactoryService_CloseFactorySession_LeavesServiceAliveWithoutLiveSessions(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
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

	defaultSession := svc.sessionByID(defaultFactorySessionID)
	if defaultSession == nil {
		t.Fatal("expected default session to be registered")
	}
	if err := svc.CloseFactorySession(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("CloseFactorySession(default): %v", err)
	}

	select {
	case <-defaultSession.handle.runDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed default session to exit")
	}
	if svc.sessions.count() != 0 {
		t.Fatalf("live session count = %d, want 0", svc.sessions.count())
	}
	if svc.currentFactory() != nil {
		t.Fatal("expected no compatibility runtime after closing the last session")
	}

	select {
	case err := <-runErrCh:
		t.Fatalf("Run exited early after last-session close: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

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

func TestFactoryService_LegacyRuntimeSurfaceTargetsDefaultSessionAlias(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
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

	betaSessionID, err := svc.openFactorySession(context.Background(), betaDir)
	if err != nil {
		t.Fatalf("openFactorySession(beta): %v", err)
	}
	betaSession := svc.sessionByID(betaSessionID)
	if betaSession == nil {
		t.Fatalf("expected beta session %q to be registered", betaSessionID)
	}

	selectCompatibilitySessionForTest(t, svc, betaSessionID)

	submitSessionWork(t, betaSession, "beta-only-work", "trace-beta-only")
	waitForSessionEventsToContain(t, betaSession, "beta-only-work", time.Second)

	submitCompatWork(t, svc, "default-only-work", "trace-default-only")
	waitForSessionEventsToContain(t, svc.sessionByID(defaultFactorySessionID), "default-only-work", time.Second)

	compatEvents, err := svc.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	compatPayload, err := json.Marshal(compatEvents)
	if err != nil {
		t.Fatalf("Marshal(GetFactoryEvents): %v", err)
	}
	if !strings.Contains(string(compatPayload), "default-only-work") {
		t.Fatalf("legacy factory events did not include default session work: %s", string(compatPayload))
	}
	if strings.Contains(string(compatPayload), "beta-only-work") {
		t.Fatalf("legacy factory events unexpectedly included selected non-default session work: %s", string(compatPayload))
	}

	assertSessionEventsDoNotContain(t, svc.sessionByID(defaultFactorySessionID), "beta-only-work")
	assertSessionEventsDoNotContain(t, betaSession, "default-only-work")

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

func TestFactoryService_SessionRuntimeSurfaceTargetsExplicitSessionID(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
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

	betaSessionID, err := svc.openFactorySession(context.Background(), betaDir)
	if err != nil {
		t.Fatalf("openFactorySession(beta): %v", err)
	}

	request := factory.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
		WorkID:     "beta-session-targeted-work",
		Name:       "beta-session-targeted-work",
		WorkTypeID: "task",
		TraceID:    "trace-beta-session-targeted",
		Payload:    []byte(`{"title":"beta-session-targeted-work"}`),
	}})
	if _, err := svc.SubmitWorkRequestForSession(context.Background(), betaSessionID, request); err != nil {
		t.Fatalf("SubmitWorkRequestForSession(beta): %v", err)
	}

	waitForSessionEventsToContain(t, svc.sessionByID(betaSessionID), "beta-session-targeted-work", time.Second)
	assertSessionEventsDoNotContain(t, svc.sessionByID(defaultFactorySessionID), "beta-session-targeted-work")

	betaCurrent, err := svc.GetCurrentNamedFactoryForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetCurrentNamedFactoryForSession(beta): %v", err)
	}
	if betaCurrent.Name != "beta" {
		t.Fatalf("beta current factory name = %q, want beta", betaCurrent.Name)
	}

	defaultCurrent, err := svc.GetCurrentNamedFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentNamedFactoryForSession(default): %v", err)
	}
	if defaultCurrent.Name != "alpha" {
		t.Fatalf("default current factory name = %q, want alpha", defaultCurrent.Name)
	}

	if _, err := svc.GetEngineStateSnapshotForSession(context.Background(), "missing-session"); err == nil || !strings.Contains(err.Error(), "factory session not found") {
		t.Fatalf("GetEngineStateSnapshotForSession(missing) error = %v, want factory session not found", err)
	}

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

func TestFactoryService_Run_RestartsOnlyDefaultSession(t *testing.T) {
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
		t.Fatalf("BuildFactoryService(first): %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default alpha runtime")
	if _, err := svc.openFactorySession(context.Background(), betaDir); err != nil {
		t.Fatalf("openFactorySession(beta): %v", err)
	}
	if got := svc.sessions.count(); got != 2 {
		t.Fatalf("first run live session count = %d, want 2", got)
	}

	cancel()
	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("first Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first service shutdown")
	}

	restarted, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService(restart): %v", err)
	}

	restartCtx, restartCancel := context.WithCancel(context.Background())
	restartErrCh := make(chan error, 1)
	go func() {
		restartErrCh <- restarted.Run(restartCtx)
	}()

	waitForSessionRuntimeStatus(t, restarted, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "restarted default alpha runtime")
	if got := restarted.sessions.ids(); len(got) != 1 || got[0] != defaultFactorySessionID {
		t.Fatalf("restarted session ids = %v, want [%s]", got, defaultFactorySessionID)
	}
	defaultSession := restarted.sessionByID(defaultFactorySessionID)
	if defaultSession == nil || defaultSession.handle == nil || defaultSession.handle.runtime == nil {
		t.Fatal("expected restarted default session runtime to be registered")
	}
	if defaultSession.handle.runtime.dir != alphaDir {
		t.Fatalf("restarted default runtime dir = %q, want %q", defaultSession.handle.runtime.dir, alphaDir)
	}

	restartCancel()
	select {
	case err := <-restartErrCh:
		if err != nil {
			t.Fatalf("restarted Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for restarted service shutdown")
	}
}

func TestFactoryService_SaveEditableFactoryDefinitionForSession_ReplacesOnlyTargetedSession(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
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

	betaSessionID, err := svc.openFactorySession(context.Background(), betaDir)
	if err != nil {
		t.Fatalf("openFactorySession(beta): %v", err)
	}
	waitForSessionRuntimeStatus(t, svc, betaSessionID, interfaces.RuntimeStatusIdle, time.Second, "beta runtime")

	editable, err := svc.GetEditableFactoryDefinitionForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetEditableFactoryDefinitionForSession(beta): %v", err)
	}
	if editable.FactoryDefinition.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("beta editable factory name = %q, want beta", editable.FactoryDefinition.Name)
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	saved, err := svc.SaveEditableFactoryDefinitionForSession(context.Background(), betaSessionID, factoryapi.SaveEditableFactoryDefinitionRequest{
		BaseVersion:       &editable.Version,
		FactoryDefinition: replacement,
	})
	if err != nil {
		t.Fatalf("SaveEditableFactoryDefinitionForSession(beta): %v", err)
	}
	if saved.FactoryDefinition.WorkTypes == nil || len(*saved.FactoryDefinition.WorkTypes) != 1 || (*saved.FactoryDefinition.WorkTypes)[0].Name != "story" {
		t.Fatalf("saved beta work types = %#v, want story", saved.FactoryDefinition.WorkTypes)
	}

	betaCurrent, err := svc.GetCurrentNamedFactoryForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetCurrentNamedFactoryForSession(beta) after save: %v", err)
	}
	if betaCurrent.WorkTypes == nil || len(*betaCurrent.WorkTypes) != 1 || (*betaCurrent.WorkTypes)[0].Name != "story" {
		t.Fatalf("beta current work types after save = %#v, want story", betaCurrent.WorkTypes)
	}

	defaultCurrent, err := svc.GetCurrentNamedFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentNamedFactoryForSession(default) after beta save: %v", err)
	}
	if defaultCurrent.Name != "alpha" {
		t.Fatalf("default current factory name after beta save = %q, want alpha", defaultCurrent.Name)
	}
	if defaultCurrent.WorkTypes == nil || len(*defaultCurrent.WorkTypes) != 1 || (*defaultCurrent.WorkTypes)[0].Name != "task" {
		t.Fatalf("default current work types after beta save = %#v, want unchanged task", defaultCurrent.WorkTypes)
	}

	assertCurrentFactoryPointer(t, rootDir, "alpha", "default session pointer after beta editable save")
	betaConfig, err := config.LoadRuntimeConfig(betaDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(beta) after save: %v", err)
	}
	if betaConfig.FactoryConfig().WorkTypes == nil || len(betaConfig.FactoryConfig().WorkTypes) != 1 || betaConfig.FactoryConfig().WorkTypes[0].Name != "story" {
		t.Fatalf("persisted beta work types after save = %#v, want story", betaConfig.FactoryConfig().WorkTypes)
	}

	legacyCurrent, err := svc.GetCurrentNamedFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory after beta save: %v", err)
	}
	if legacyCurrent.Name != "alpha" {
		t.Fatalf("legacy current factory name after beta save = %q, want alpha", legacyCurrent.Name)
	}

	if _, err := svc.GetEditableFactoryDefinitionForSession(context.Background(), "missing-session"); !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("GetEditableFactoryDefinitionForSession(missing) error = %v, want factory session not found", err)
	}

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

func TestFactoryService_OpenFactorySessionFromFolder_AutoOpensSingleTarget(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
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
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default alpha runtime")

	result, err := svc.OpenFactorySessionFromFolder(context.Background(), rootDir, nil)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(single target): %v", err)
	}
	if result == nil || result.SessionID == "" {
		t.Fatalf("single-target open result = %#v, want session id", result)
	}
	if len(result.Targets) != 0 {
		t.Fatalf("single-target open returned picker targets = %#v, want none", result.Targets)
	}
	session := svc.sessionByID(result.SessionID)
	if session == nil || session.handle == nil || session.handle.runtime == nil {
		t.Fatalf("opened session %q was not registered", result.SessionID)
	}
	if session.handle.runtime.dir != alphaDir {
		t.Fatalf("opened session runtime dir = %q, want %q", session.handle.runtime.dir, alphaDir)
	}
	if got := svc.sessions.count(); got != 2 {
		t.Fatalf("live session count = %d, want 2", got)
	}

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

func TestFactoryService_OpenFactorySessionFromFolder_ReturnsTargetPickerMetadata(t *testing.T) {
	rootDir := t.TempDir()
	writeFactoryJSON(t, rootDir, minimalFactoryConfig())
	writeNamedFactoryFixture(t, rootDir, "alpha")
	writeNamedFactoryFixture(t, rootDir, "beta")

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
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")

	result, err := svc.OpenFactorySessionFromFolder(context.Background(), rootDir, nil)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(multi target): %v", err)
	}
	if result == nil {
		t.Fatal("expected target picker result, got nil")
	}
	if result.SessionID != "" {
		t.Fatalf("multi-target open returned session %q, want target picker", result.SessionID)
	}
	if len(result.Targets) != 3 {
		t.Fatalf("target picker count = %d, want 3", len(result.Targets))
	}

	if got := result.Targets[0]; got.Ref.Kind != FactorySessionTargetKindDefault || got.Ref.Name != "" || got.Label != "default" || got.FactoryDir != rootDir || got.Project != "factory" {
		t.Fatalf("default target = %#v, want default target rooted at %q", got, rootDir)
	}
	if got := result.Targets[1]; got.Ref.Kind != FactorySessionTargetKindNamed || got.Ref.Name != "alpha" || got.Label != "alpha" || got.FactoryDir != filepath.Join(rootDir, "alpha") || got.Project != "alpha" {
		t.Fatalf("alpha target = %#v", got)
	}
	if got := result.Targets[2]; got.Ref.Kind != FactorySessionTargetKindNamed || got.Ref.Name != "beta" || got.Label != "beta" || got.FactoryDir != filepath.Join(rootDir, "beta") || got.Project != "beta" {
		t.Fatalf("beta target = %#v", got)
	}
	if got := svc.sessions.count(); got != 1 {
		t.Fatalf("target-picker flow mutated live sessions to %d, want 1", got)
	}

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

func TestFactoryService_OpenFactorySessionFromFolder_OpensExplicitDefaultAndNamedTargets(t *testing.T) {
	rootDir := t.TempDir()
	writeFactoryJSON(t, rootDir, minimalFactoryConfig())
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")

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
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")

	defaultOpen, err := svc.OpenFactorySessionFromFolder(context.Background(), rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindDefault,
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(default): %v", err)
	}
	betaOpenOne, err := svc.OpenFactorySessionFromFolder(context.Background(), rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "beta",
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(beta one): %v", err)
	}
	betaOpenTwo, err := svc.OpenFactorySessionFromFolder(context.Background(), rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "beta",
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(beta two): %v", err)
	}
	if betaOpenOne.SessionID == betaOpenTwo.SessionID {
		t.Fatalf("duplicate beta session ids = %q", betaOpenOne.SessionID)
	}

	defaultSession := svc.sessionByID(defaultOpen.SessionID)
	betaSessionOne := svc.sessionByID(betaOpenOne.SessionID)
	betaSessionTwo := svc.sessionByID(betaOpenTwo.SessionID)
	if defaultSession == nil || defaultSession.handle == nil || defaultSession.handle.runtime == nil {
		t.Fatalf("default target session %q was not registered", defaultOpen.SessionID)
	}
	if betaSessionOne == nil || betaSessionOne.handle == nil || betaSessionOne.handle.runtime == nil {
		t.Fatalf("beta target session %q was not registered", betaOpenOne.SessionID)
	}
	if betaSessionTwo == nil || betaSessionTwo.handle == nil || betaSessionTwo.handle.runtime == nil {
		t.Fatalf("beta target session %q was not registered", betaOpenTwo.SessionID)
	}
	if defaultSession.handle.runtime.dir != rootDir {
		t.Fatalf("default target runtime dir = %q, want %q", defaultSession.handle.runtime.dir, rootDir)
	}
	if betaSessionOne.handle.runtime.dir != betaDir || betaSessionTwo.handle.runtime.dir != betaDir {
		t.Fatalf("beta target runtime dirs = %q and %q, want %q", betaSessionOne.handle.runtime.dir, betaSessionTwo.handle.runtime.dir, betaDir)
	}
	if got := svc.sessions.count(); got != 4 {
		t.Fatalf("live session count = %d, want 4", got)
	}

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

func TestFactoryService_OpenFactorySessionFromFolder_RejectsInvalidFolderAndTargetWithoutMutation(t *testing.T) {
	rootDir := t.TempDir()
	writeFactoryJSON(t, rootDir, minimalFactoryConfig())

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
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")
	before := svc.sessions.count()

	if _, err := svc.OpenFactorySessionFromFolder(context.Background(), filepath.Join(rootDir, "missing"), nil); err == nil || !strings.Contains(err.Error(), "stat factory session folder") {
		t.Fatalf("OpenFactorySessionFromFolder(missing folder) error = %v, want folder stat failure", err)
	}
	if got := svc.sessions.count(); got != before {
		t.Fatalf("missing-folder open mutated live sessions to %d, want %d", got, before)
	}

	if _, err := svc.OpenFactorySessionFromFolder(context.Background(), rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "missing",
	}); err == nil || !strings.Contains(err.Error(), `factory session target "missing" was not found`) {
		t.Fatalf("OpenFactorySessionFromFolder(missing target) error = %v, want missing-target failure", err)
	}
	if got := svc.sessions.count(); got != before {
		t.Fatalf("missing-target open mutated live sessions to %d, want %d", got, before)
	}

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

func submitCompatWork(t *testing.T, svc *FactoryService, workID, traceID string) {
	t.Helper()

	request := factory.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
		WorkID:     workID,
		Name:       workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"` + workID + `"}`),
	}})
	if _, err := svc.SubmitWorkRequest(context.Background(), request); err != nil {
		t.Fatalf("SubmitWorkRequest(%s): %v", workID, err)
	}
}

func selectCompatibilitySessionForTest(t *testing.T, svc *FactoryService, sessionID string) {
	t.Helper()

	if svc == nil || svc.sessions == nil {
		t.Fatal("service session manager is required")
	}
	svc.sessions.mu.Lock()
	defer svc.sessions.mu.Unlock()
	if _, ok := svc.sessions.sessions[sessionID]; !ok {
		t.Fatalf("session %q is not registered", sessionID)
	}
	svc.sessions.selectedID = sessionID
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
