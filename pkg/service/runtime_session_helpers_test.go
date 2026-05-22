package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"go.uber.org/zap"
)

type runningSessionServiceOptions struct {
	defaultFactory string
	namedFactories []string
	rootConfig     map[string]any
	runtimeLogDir  string
	recordPath     string
}

type runningSessionService struct {
	rootDir     string
	svc         *FactoryService
	runErrCh    chan error
	cancelRun   context.CancelFunc
	factoryDirs map[string]string
}

type sessionWorkExpectation struct {
	session  *liveFactorySession
	workID   string
	traceID  string
	excluded []string
}

func startRunningSessionService(t *testing.T, options runningSessionServiceOptions) *runningSessionService {
	t.Helper()

	rootDir := t.TempDir()
	if options.rootConfig != nil {
		writeFactoryJSON(t, rootDir, options.rootConfig)
	}

	factoryDirs := map[string]string{}
	for _, name := range options.namedFactories {
		factoryDirs[name] = writeNamedFactoryFixture(t, rootDir, name)
	}

	if options.defaultFactory != "" {
		if err := config.WriteCurrentFactoryPointer(rootDir, options.defaultFactory); err != nil {
			t.Fatalf("WriteCurrentFactoryPointer(%s): %v", options.defaultFactory, err)
		}
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RuntimeLogDir:     options.runtimeLogDir,
		RecordPath:        options.recordPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")

	return &runningSessionService{
		rootDir:     rootDir,
		svc:         svc,
		runErrCh:    runErrCh,
		cancelRun:   cancelRun,
		factoryDirs: factoryDirs,
	}
}

func (h *runningSessionService) stop(t *testing.T) {
	t.Helper()

	h.cancelRun()
	select {
	case err := <-h.runErrCh:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func (h *runningSessionService) openFactorySession(t *testing.T, factoryName string) string {
	t.Helper()

	dir, ok := h.factoryDirs[factoryName]
	if !ok {
		t.Fatalf("factory fixture %q is not registered", factoryName)
	}
	sessionID, err := h.svc.openFactorySession(context.Background(), dir)
	if err != nil {
		t.Fatalf("openFactorySession(%s): %v", factoryName, err)
	}
	return sessionID
}

func (h *runningSessionService) requireSession(t *testing.T, sessionID string) *liveFactorySession {
	t.Helper()

	session := h.svc.sessionByID(sessionID)
	if session == nil {
		t.Fatalf("expected session %q to be registered; got ids %v", sessionID, h.svc.sessions.ids())
	}
	return session
}

func (h *runningSessionService) waitIdle(t *testing.T, sessionID, label string) {
	t.Helper()
	waitForSessionRuntimeStatus(t, h.svc, sessionID, interfaces.RuntimeStatusIdle, time.Second, label)
}

func assertSessionWorkIsolation(t *testing.T, expectations []sessionWorkExpectation) {
	t.Helper()

	for _, expectation := range expectations {
		submitSessionWork(t, expectation.session, expectation.workID, expectation.traceID)
	}
	for _, expectation := range expectations {
		waitForSessionEventsToContain(t, expectation.session, expectation.workID, time.Second)
		for _, excluded := range expectation.excluded {
			assertSessionEventsDoNotContain(t, expectation.session, excluded)
		}
	}
}

func assertSessionArtifactIsolation(t *testing.T, session *liveFactorySession, wantWork string, forbiddenWork map[string]string) {
	t.Helper()

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
	if !strings.Contains(string(payload), wantWork) {
		t.Fatalf("artifact %s did not contain session work %q: %s", runtimeBundle.recordPath, wantWork, string(payload))
	}
	for otherSessionID, otherWork := range forbiddenWork {
		if otherSessionID == session.id {
			continue
		}
		if strings.Contains(string(payload), otherWork) {
			t.Fatalf("artifact %s leaked work %q from session %s: %s", runtimeBundle.recordPath, otherWork, otherSessionID, string(payload))
		}
	}
}

func assertSessionRuntimeLogRecord(t *testing.T, session *liveFactorySession) {
	t.Helper()

	if session == nil || session.handle == nil || session.handle.runtime == nil {
		t.Fatal("expected live session runtime")
	}

	runtimeBundle := session.handle.runtime
	logPath := runtimeBundle.logSink.Path()
	if logPath == "" {
		t.Fatalf("session %s runtime log path is empty", session.id)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	records := parseRuntimeLogRecords(t, string(data))
	for _, record := range records {
		if record["session_id"] != session.id {
			continue
		}
		if record["folder_path"] != runtimeBundle.folderPath {
			t.Fatalf("session %s folder_path = %#v, want %q in %#v", session.id, record["folder_path"], runtimeBundle.folderPath, record)
		}
		if record["runtime_instance_id"] == "" {
			t.Fatalf("session %s runtime_instance_id missing in %#v", session.id, record)
		}
		return
	}
	t.Fatalf("runtime log %s did not contain any records for session %s:\n%s", logPath, session.id, string(data))
}

func assertFactoryWorkType(t *testing.T, workTypes *[]factoryapi.WorkType, want string, label string) {
	t.Helper()

	if workTypes == nil || len(*workTypes) != 1 || (*workTypes)[0].Name != want {
		t.Fatalf("%s = %#v, want %q", label, workTypes, want)
	}
}

func assertSessionTargetMetadata(
	t *testing.T,
	target FactorySessionTarget,
	kind FactorySessionTargetKind,
	name string,
	label string,
	factoryDir string,
	project string,
) {
	t.Helper()

	if target.Ref.Kind != kind || target.Ref.Name != name || target.Label != label || target.FactoryDir != factoryDir || target.Project != project {
		t.Fatalf("session target = %#v, want kind=%q name=%q label=%q dir=%q project=%q", target, kind, name, label, factoryDir, project)
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
