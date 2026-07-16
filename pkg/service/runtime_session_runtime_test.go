// backendsizecheck:ignore-file consolidated session-runtime and absolute-path tests remain together until dedicated service test seams split.
// pkgmaintcheck:ignore-file-lines consolidated session-runtime and absolute-path tests remain together until dedicated service test seams split.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/recording"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/recordingreplay"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/factory/sessions/invocation"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/work"
	workinvocation "github.com/portpowered/infinite-you/pkg/work/invocation"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSessionScopedRecordPath_ReplacesGeneratedSessionTokenPerSession(t *testing.T) {
	basePath := filepath.Join(
		t.TempDir(),
		"recordings",
		"2026-05",
		"2026-05-23",
		"factory-session-__factory_session_id__-184512-uuid-1.json",
	)

	defaultPath := sessionScopedRecordPath(basePath, defaultFactorySessionID)
	if !strings.Contains(defaultPath, "factory-session-"+defaultFactorySessionID+"-184512-uuid-1.json") {
		t.Fatalf("default session path = %q", defaultPath)
	}

	sessionPath := sessionScopedRecordPath(basePath, "session-123")
	if !strings.Contains(sessionPath, "factory-session-session-123-184512-uuid-1.json") {
		t.Fatalf("named session path = %q", sessionPath)
	}
	if defaultPath == sessionPath {
		t.Fatalf("session-scoped paths matched: %q", defaultPath)
	}
}

func TestFactoryService_OpenFactorySession_RunsConcurrentIsolatedSessions(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionOne := harness.openFactorySession(t, "beta")
	betaSessionTwo := harness.openFactorySession(t, "beta")
	if betaSessionOne == betaSessionTwo {
		t.Fatalf("duplicate beta session ids = %q", betaSessionOne)
	}
	if got := harness.svc.sessions.Count(); got != 3 {
		t.Fatalf("live session count = %d, want 3", got)
	}

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	firstBeta := harness.requireSession(t, betaSessionOne)
	secondBeta := harness.requireSession(t, betaSessionTwo)
	if liveSessionHandle(defaultSession) == liveSessionHandle(firstBeta) || liveSessionHandle(firstBeta) == liveSessionHandle(secondBeta) {
		t.Fatal("expected each live session to own a distinct runtime handle")
	}
	if liveSessionHandle(defaultSession).Bundle.Dir != harness.factoryDirs["alpha"] {
		t.Fatalf("default runtime dir = %q, want %q", liveSessionHandle(defaultSession).Bundle.Dir, harness.factoryDirs["alpha"])
	}
	if liveSessionHandle(firstBeta).Bundle.Dir != harness.factoryDirs["beta"] || liveSessionHandle(secondBeta).Bundle.Dir != harness.factoryDirs["beta"] {
		t.Fatalf("beta runtime dirs = %q and %q, want %q", liveSessionHandle(firstBeta).Bundle.Dir, liveSessionHandle(secondBeta).Bundle.Dir, harness.factoryDirs["beta"])
	}

	assertSessionWorkIsolation(t, []sessionWorkExpectation{
		{session: defaultSession, workID: "alpha-session-work", traceID: "trace-alpha-session", excluded: []string{"beta-session-one-work", "beta-session-two-work"}},
		{session: firstBeta, workID: "beta-session-one-work", traceID: "trace-beta-session-one", excluded: []string{"alpha-session-work", "beta-session-two-work"}},
		{session: secondBeta, workID: "beta-session-two-work", traceID: "trace-beta-session-two", excluded: []string{"alpha-session-work", "beta-session-one-work"}},
	})

	if err := harness.svc.stopFactorySession(betaSessionOne); err != nil {
		t.Fatalf("stopFactorySession(beta one): %v", err)
	}
	select {
	case <-liveSessionHandle(firstBeta).RunDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stopped beta session to exit")
	}
	if got := harness.svc.sessions.Count(); got != 2 {
		t.Fatalf("live session count after stop = %d, want 2", got)
	}
	if harness.svc.sessionByID(betaSessionOne) != nil {
		t.Fatalf("stopped beta session %q is still registered", betaSessionOne)
	}
	assertSessionRemainsLive(t, harness.svc, betaSessionTwo, 200*time.Millisecond, "remaining beta runtime")
	harness.waitIdle(t, defaultFactorySessionID, "default runtime after beta stop")
}

func TestFactoryService_LiveSessionsOwnIsolatedResponseEventStores(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	betaSession := harness.requireSession(t, betaSessionID)
	assertSessionsOwnIsolatedResponseEventStores(t, defaultSession, betaSession)

	defaultHistory := liveSessionHandle(defaultSession).Bundle.EventHistory
	betaHistory := liveSessionHandle(betaSession).Bundle.EventHistory
	defaultHistoryCount := len(defaultHistory.CanonicalEvents())
	betaHistoryCount := len(betaHistory.CanonicalEvents())
	defaultEvent := publishSessionResponseEvent(t, defaultSession, "run-default")
	betaEvent := publishSessionResponseEvent(t, betaSession, "run-beta")
	assertPublishedResponseEventsAreSessionIsolated(t, defaultSession, betaSession, defaultEvent, betaEvent)
	if len(defaultHistory.CanonicalEvents()) != defaultHistoryCount || len(betaHistory.CanonicalEvents()) != betaHistoryCount {
		t.Fatal("response-event publication mutated canonical FactoryEvent history")
	}

	defaultHistory.RecordSessionCompleted(factoryevents.SessionLifecycleCompleteInput{
		SessionID:        factorysessions.CanonicalFactorySessionID(defaultSession),
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Source:           "runtime",
		FinalStatus:      interfaces.FactorySessionLifecycleStatusSucceeded,
	}, time.Now().UTC())
	assertResponseEventCompletionIsSessionIsolated(t, defaultSession, betaSession)

	if err := harness.svc.stopFactorySession(betaSessionID); err != nil {
		t.Fatalf("stop beta session: %v", err)
	}
	if _, err := betaSession.ResponseEvents.Subscribe(0); !errors.Is(err, responseeventstore.ErrStoreClosed) {
		t.Fatalf("subscribe after session close error = %v, want ErrStoreClosed", err)
	}
}

type serviceResponseEventClock struct {
	now time.Time
}

func (c *serviceResponseEventClock) Now() time.Time {
	return c.now
}

func TestFactoryService_SubscribeFactoryResponseEventsClassifiesExpiredStream(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	session := harness.requireSession(t, defaultFactorySessionID)
	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clock := &serviceResponseEventClock{now: start}
	store := responseeventstore.NewSessionResponseEventStoreWithClock(
		factorysessions.CanonicalFactorySessionID(session),
		clock,
	)
	if _, err := store.Publish(sessionResponseEventFixture("run-completed")); err != nil {
		t.Fatalf("publish response event: %v", err)
	}
	store.Complete()
	clock.now = start.Add(responseeventstore.CompletedStreamRetentionWindow)
	session.ResponseEvents = store

	_, err := harness.svc.SubscribeFactoryResponseEventsForSession(
		context.Background(),
		factorysessions.CanonicalFactorySessionID(session),
		0,
		"",
	)
	if !errors.Is(err, apisurface.ErrFactoryResponseEventStreamExpired) {
		t.Fatalf("SubscribeFactoryResponseEventsForSession error = %v, want ErrFactoryResponseEventStreamExpired", err)
	}
}

func assertSessionsOwnIsolatedResponseEventStores(
	t *testing.T,
	first *factorysessions.LiveSession,
	second *factorysessions.LiveSession,
) {
	t.Helper()

	if first.ResponseEvents == nil || second.ResponseEvents == nil {
		t.Fatal("live sessions must own response-event stores")
	}
	if first.ResponseEvents == second.ResponseEvents {
		t.Fatal("live sessions unexpectedly share one response-event store")
	}
}

func publishSessionResponseEvent(
	t *testing.T,
	session *factorysessions.LiveSession,
	runID string,
) responseevents.FactoryResponseEvent {
	t.Helper()

	event, err := session.ResponseEvents.Publish(sessionResponseEventFixture(runID))
	if err != nil {
		t.Fatalf("publish response event for %s: %v", runID, err)
	}
	return event
}

func assertPublishedResponseEventsAreSessionIsolated(
	t *testing.T,
	first *factorysessions.LiveSession,
	second *factorysessions.LiveSession,
	firstEvent responseevents.FactoryResponseEvent,
	secondEvent responseevents.FactoryResponseEvent,
) {
	t.Helper()

	if firstEvent.Sequence != 1 || secondEvent.Sequence != 1 {
		t.Fatalf("isolated sequences = (%d, %d), want (1, 1)", firstEvent.Sequence, secondEvent.Sequence)
	}
	if firstEvent.FactorySessionID != factorysessions.CanonicalFactorySessionID(first) ||
		secondEvent.FactorySessionID != factorysessions.CanonicalFactorySessionID(second) {
		t.Fatalf("response event session IDs = (%q, %q), want canonical live session IDs", firstEvent.FactorySessionID, secondEvent.FactorySessionID)
	}
	if len(first.ResponseEvents.Events()) != 1 || len(second.ResponseEvents.Events()) != 1 {
		t.Fatal("published response events were not isolated by live session")
	}
}

func assertResponseEventCompletionIsSessionIsolated(
	t *testing.T,
	completed *factorysessions.LiveSession,
	active *factorysessions.LiveSession,
) {
	t.Helper()

	if !completed.ResponseEvents.Completed() {
		t.Fatal("canonical session completion did not complete the response-event store")
	}
	if active.ResponseEvents.Completed() {
		t.Fatal("default session completion affected beta response-event store")
	}
}

func sessionResponseEventFixture(runID string) responseevents.FactoryResponseEvent {
	return responseevents.FactoryResponseEvent{
		RunID: runID,
		Kind:  responseevents.KindMessage,
		Phase: responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:        "integration-test",
			NativeEventType: "message.delta",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationDelta,
			Fidelity:        responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`),
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_KeepsSessionWorkingDirectoriesIsolatedAcrossRoots(t *testing.T) {
	rootOne := t.TempDir()
	rootTwo := t.TempDir()
	writeFactoryJSON(t, rootOne, sessionScriptWorkingDirectoryFactoryConfig("alpha"))
	writeFactoryJSON(t, rootTwo, sessionScriptWorkingDirectoryFactoryConfig("beta"))
	writeScriptWorkerAgentsMD(t, rootOne, "script-worker")
	writeScriptWorkerAgentsMD(t, rootTwo, "script-worker")
	writeSessionRuntimeLookupWorkstationAgentsMD(t, rootOne, "run-script", "workspace-alpha")
	writeSessionRuntimeLookupWorkstationAgentsMD(t, rootTwo, "run-script", "workspace-beta")

	runner := &sessionCapturingCommandRunner{}
	svc, err := BuildFactoryService(context.Background(), serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{
		Dir:              rootOne,
		ExecutionBaseDir: rootOne,
		RuntimeMode:      interfaces.RuntimeModeService,
		Logger:           zap.NewNop(),
	}, workerapplication.Edges{ScriptCommandRunner: runner}))
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")
	harness := &runningSessionService{
		rootDir:   rootOne,
		svc:       svc,
		runErrCh:  runErrCh,
		cancelRun: cancelRun,
	}
	defer harness.stop(t)

	openResult, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), rootTwo, nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(root two): %v", err)
	}
	if openResult == nil || openResult.SessionID == "" {
		t.Fatalf("open result = %#v, want session id", openResult)
	}

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	secondSession := harness.requireSession(t, openResult.SessionID)
	if defaultSession.FolderPath != rootOne {
		t.Fatalf("default session folder path = %q, want %q", defaultSession.FolderPath, rootOne)
	}
	if secondSession.FolderPath != rootTwo {
		t.Fatalf("second session folder path = %q, want %q", secondSession.FolderPath, rootTwo)
	}
	if liveSessionHandle(defaultSession).Bundle.Dir != rootOne {
		t.Fatalf("default runtime dir = %q, want %q", liveSessionHandle(defaultSession).Bundle.Dir, rootOne)
	}
	if liveSessionHandle(secondSession).Bundle.Dir != rootTwo {
		t.Fatalf("second runtime dir = %q, want %q", liveSessionHandle(secondSession).Bundle.Dir, rootTwo)
	}

	submitSessionWork(t, defaultSession, "alpha-work", "trace-alpha-work")
	submitSessionWork(t, secondSession, "beta-work", "trace-beta-work")
	waitForSessionEventsToContain(t, defaultSession, "alpha-work", time.Second)
	waitForSessionEventsToContain(t, secondSession, "beta-work", time.Second)

	requests := runner.waitForRequests(t, 2, time.Second)
	workDirs := map[string]int{}
	for _, request := range requests {
		workDirs[request.WorkDir]++
	}

	defaultWorkingDir := filepath.Join(rootOne, "workspace-alpha")
	secondWorkingDir := filepath.Join(rootTwo, "workspace-beta")
	if workDirs[defaultWorkingDir] != 1 || workDirs[secondWorkingDir] != 1 {
		t.Fatalf("command workdirs = %#v, want one request for %q and one for %q", workDirs, defaultWorkingDir, secondWorkingDir)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_DefaultsEmptyWorkingDirectoryToSessionRoot(t *testing.T) {
	rootOne := t.TempDir()
	rootTwo := t.TempDir()
	writeFactoryJSON(t, rootOne, sessionScriptWorkingDirectoryFactoryConfig("alpha"))
	writeFactoryJSON(t, rootTwo, sessionScriptWorkingDirectoryFactoryConfig("beta"))
	writeScriptWorkerAgentsMD(t, rootOne, "script-worker")
	writeScriptWorkerAgentsMD(t, rootTwo, "script-worker")
	writeSessionRuntimeLookupWorkstationAgentsMDWithoutWorkingDirectory(t, rootOne, "run-script")
	writeSessionRuntimeLookupWorkstationAgentsMDWithoutWorkingDirectory(t, rootTwo, "run-script")

	runner := &sessionCapturingCommandRunner{}
	svc, err := BuildFactoryService(context.Background(), serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{
		Dir:              rootOne,
		ExecutionBaseDir: rootOne,
		RuntimeMode:      interfaces.RuntimeModeService,
		Logger:           zap.NewNop(),
	}, workerapplication.Edges{ScriptCommandRunner: runner}))
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")
	harness := &runningSessionService{
		rootDir:   rootOne,
		svc:       svc,
		runErrCh:  runErrCh,
		cancelRun: cancelRun,
	}
	defer harness.stop(t)

	openResult, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), rootTwo, nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(root two): %v", err)
	}
	if openResult == nil || openResult.SessionID == "" {
		t.Fatalf("open result = %#v, want session id", openResult)
	}

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	secondSession := harness.requireSession(t, openResult.SessionID)
	submitSessionWork(t, defaultSession, "alpha-empty-workdir", "trace-alpha-empty-workdir")
	submitSessionWork(t, secondSession, "beta-empty-workdir", "trace-beta-empty-workdir")
	waitForSessionEventsToContain(t, defaultSession, "alpha-empty-workdir", time.Second)
	waitForSessionEventsToContain(t, secondSession, "beta-empty-workdir", time.Second)

	requests := runner.waitForRequests(t, 2, time.Second)
	workDirs := map[string]int{}
	for _, request := range requests {
		workDirs[request.WorkDir]++
	}
	if workDirs[rootOne] != 1 || workDirs[rootTwo] != 1 {
		t.Fatalf("command workdirs = %#v, want one request for %q and one for %q", workDirs, rootOne, rootTwo)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_DefaultsEmptyModelWorkingDirectoryToSessionRoot(t *testing.T) {
	rootOne := t.TempDir()
	rootTwo := t.TempDir()
	writeFactoryJSON(t, rootOne, sessionModelWorkingDirectoryFactoryConfig("alpha"))
	writeFactoryJSON(t, rootTwo, sessionModelWorkingDirectoryFactoryConfig("beta"))
	writeWorkerAgentsMD(t, rootOne, "model-worker")
	writeWorkerAgentsMD(t, rootTwo, "model-worker")
	writeSessionRuntimeLookupModelWorkstationAgentsMDWithoutWorkingDirectory(t, rootOne, "run-model", interfaces.WorkstationTypeModel)
	writeSessionRuntimeLookupModelWorkstationAgentsMDWithoutWorkingDirectory(t, rootTwo, "run-model", interfaces.WorkstationTypeModel)

	provider := &sessionCapturingProvider{}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:              rootOne,
		ExecutionBaseDir: rootOne,
		RuntimeMode:      interfaces.RuntimeModeService,
		ProviderOverride: provider,
		Logger:           zap.NewNop(),
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
	harness := &runningSessionService{
		rootDir:   rootOne,
		svc:       svc,
		runErrCh:  runErrCh,
		cancelRun: cancelRun,
	}
	defer harness.stop(t)

	openResult, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), rootTwo, nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(root two): %v", err)
	}
	if openResult == nil || openResult.SessionID == "" {
		t.Fatalf("open result = %#v, want session id", openResult)
	}

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	secondSession := harness.requireSession(t, openResult.SessionID)
	submitSessionWork(t, defaultSession, "alpha-empty-model-workdir", "trace-alpha-empty-model-workdir")
	submitSessionWork(t, secondSession, "beta-empty-model-workdir", "trace-beta-empty-model-workdir")
	waitForSessionEventsToContain(t, defaultSession, "alpha-empty-model-workdir", time.Second)
	waitForSessionEventsToContain(t, secondSession, "beta-empty-model-workdir", time.Second)

	requests := provider.waitForRequests(t, 2, time.Second)
	workDirs := map[string]int{}
	for _, request := range requests {
		workDirs[request.WorkingDirectory]++
	}
	if workDirs[rootOne] != 1 || workDirs[rootTwo] != 1 {
		t.Fatalf("provider workdirs = %#v, want one request for %q and one for %q", workDirs, rootOne, rootTwo)
	}
}

func TestFactoryService_OpenFactorySession_IsolatesSessionLogsAndReplayArtifacts(t *testing.T) {
	recordPath := t.TempDir() + "/recording.json"
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
		recordPath:     recordPath,
	})

	betaSessionOne := harness.openFactorySession(t, "beta")
	betaSessionTwo := harness.openFactorySession(t, "beta")

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	firstBeta := harness.requireSession(t, betaSessionOne)
	secondBeta := harness.requireSession(t, betaSessionTwo)

	workBySession := map[string]string{
		defaultFactorySessionID: "alpha-session-work",
		betaSessionOne:          "beta-session-one-work",
		betaSessionTwo:          "beta-session-two-work",
	}
	assertSessionWorkIsolation(t, []sessionWorkExpectation{
		{session: defaultSession, workID: workBySession[defaultFactorySessionID], traceID: "trace-alpha-session"},
		{session: firstBeta, workID: workBySession[betaSessionOne], traceID: "trace-beta-session-one"},
		{session: secondBeta, workID: workBySession[betaSessionTwo], traceID: "trace-beta-session-two"},
	})

	harness.stop(t)

	if liveSessionHandle(defaultSession).Bundle.RecordPath != recordPath {
		t.Fatalf("default record path = %q, want %q", liveSessionHandle(defaultSession).Bundle.RecordPath, recordPath)
	}
	if liveSessionHandle(firstBeta).Bundle.RecordPath == "" || liveSessionHandle(secondBeta).Bundle.RecordPath == "" {
		t.Fatalf("background record paths must be set, got %q and %q", liveSessionHandle(firstBeta).Bundle.RecordPath, liveSessionHandle(secondBeta).Bundle.RecordPath)
	}
	if liveSessionHandle(firstBeta).Bundle.RecordPath == liveSessionHandle(secondBeta).Bundle.RecordPath {
		t.Fatalf("background sessions shared record path %q", liveSessionHandle(firstBeta).Bundle.RecordPath)
	}

	for _, session := range []*liveFactorySession{defaultSession, firstBeta, secondBeta} {
		assertSessionArtifactIsolation(t, session, workBySession[session.ID], workBySession)
	}
	assertSessionRuntimeLogPathsAreDistinct(t, harness.runtimeLogDir, defaultSession, firstBeta, secondBeta)
	assertSessionRuntimeMetricsPathsAreDistinct(t, harness.metricsDir, defaultSession, firstBeta, secondBeta)
	for _, session := range []*liveFactorySession{defaultSession, firstBeta, secondBeta} {
		assertSessionRuntimeLogRecord(t, session)
	}
}

type sessionCapturingCommandRunner struct {
	mu       sync.Mutex
	requests []workers.CommandRequest
}

func (r *sessionCapturingCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.requests = append(r.requests, workers.CommandRequest(workerexecution.CloneSubprocessExecutionRequest(req)))
	r.mu.Unlock()
	return workers.CommandResult{Stdout: []byte("ok")}, nil
}

func (r *sessionCapturingCommandRunner) waitForRequests(t *testing.T, want int, wait time.Duration) []workers.CommandRequest {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		if len(r.requests) >= want {
			requests := append([]workers.CommandRequest(nil), r.requests...)
			r.mu.Unlock()
			return requests
		}
		r.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("timed out waiting for %d command requests; got %d", want, len(r.requests))
	return nil
}

type sessionCapturingProvider struct {
	mu       sync.Mutex
	requests []workerexecution.ProviderInferenceRequest
}

func (p *sessionCapturingProvider) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, workerexecution.CloneProviderInferenceRequest(req))
	p.mu.Unlock()
	return workerexecution.InferenceResponse{Content: "ok"}, nil
}

func (p *sessionCapturingProvider) waitForRequests(t *testing.T, want int, wait time.Duration) []workerexecution.ProviderInferenceRequest {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		if len(p.requests) >= want {
			requests := append([]workerexecution.ProviderInferenceRequest(nil), p.requests...)
			p.mu.Unlock()
			return requests
		}
		p.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	t.Fatalf("timed out waiting for %d provider requests; got %d", want, len(p.requests))
	return nil
}

func sessionScriptWorkingDirectoryFactoryConfig(name string) map[string]any {
	return map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "script-worker",
			},
		},
		"workstations": []map[string]any{
			{
				"name":      "run-script",
				"worker":    "script-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func sessionModelWorkingDirectoryFactoryConfig(name string) map[string]any {
	return map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "model-worker",
			},
		},
		"workstations": []map[string]any{
			{
				"name":      "run-model",
				"worker":    "model-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeSessionRuntimeLookupWorkstationAgentsMD(t *testing.T, factoryDir, workstationName, relativeWorkingDir string) {
	t.Helper()

	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\nworker: script-worker\nworkingDirectory: " + relativeWorkingDir + "\n---\nRun the script.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func writeSessionRuntimeLookupWorkstationAgentsMDWithoutWorkingDirectory(t *testing.T, factoryDir, workstationName string) {
	t.Helper()

	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\nworker: script-worker\n---\nRun the script.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func writeSessionRuntimeLookupModelWorkstationAgentsMDWithoutWorkingDirectory(t *testing.T, factoryDir, workstationName, workstationType string) {
	t.Helper()

	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: " + string(workstationType) + "\nworker: model-worker\n---\nReview {{ (index .Inputs 0).WorkID }}.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func TestFactoryService_OpenFactorySession_ReopenedSessionGetsDistinctReplayArtifact(t *testing.T) {
	recordPath := t.TempDir() + "/recording.json"
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
		recordPath:     recordPath,
	})

	firstBetaSessionID := harness.openFactorySession(t, "beta")
	firstBeta := harness.requireSession(t, firstBetaSessionID)
	submitSessionWork(t, firstBeta, "beta-session-one-work", "trace-beta-session-one")
	waitForSessionEventsToContain(t, firstBeta, "beta-session-one-work", time.Second)

	if err := harness.svc.stopFactorySession(firstBetaSessionID); err != nil {
		t.Fatalf("stopFactorySession(first beta): %v", err)
	}
	select {
	case <-liveSessionHandle(firstBeta).RunDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stopped beta session to exit")
	}

	secondBetaSessionID := harness.openFactorySession(t, "beta")
	secondBeta := harness.requireSession(t, secondBetaSessionID)
	if firstBetaSessionID == secondBetaSessionID {
		t.Fatalf("reopened beta session id = %q, want a new session identity", secondBetaSessionID)
	}
	if liveSessionHandle(firstBeta).Bundle.RecordPath == liveSessionHandle(secondBeta).Bundle.RecordPath {
		t.Fatalf("reopened beta sessions shared record path %q", liveSessionHandle(secondBeta).Bundle.RecordPath)
	}

	submitSessionWork(t, secondBeta, "beta-session-two-work", "trace-beta-session-two")
	waitForSessionEventsToContain(t, secondBeta, "beta-session-two-work", time.Second)

	harness.stop(t)

	workBySession := map[string]string{
		firstBetaSessionID:  "beta-session-one-work",
		secondBetaSessionID: "beta-session-two-work",
	}
	assertSessionArtifactIsolation(t, firstBeta, workBySession[firstBetaSessionID], workBySession)
	assertSessionArtifactIsolation(t, secondBeta, workBySession[secondBetaSessionID], workBySession)
}

func TestFactoryService_CloseFactorySession_ClosesDefaultAndPromotesRemainingSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, betaSessionID, "beta runtime")

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	if err := harness.svc.CloseFactorySession(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("CloseFactorySession(default): %v", err)
	}

	select {
	case <-liveSessionHandle(defaultSession).RunDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed default session to exit")
	}
	if harness.svc.sessionByID(defaultFactorySessionID) != nil {
		t.Fatal("default session remained registered after close")
	}
	if current := harness.svc.currentRunState(); current == nil || current.sessionID != betaSessionID {
		t.Fatalf("current run state = %#v, want beta session %q", current, betaSessionID)
	}
	assertSessionRemainsLive(t, harness.svc, betaSessionID, 200*time.Millisecond, "beta runtime after default close")
}

func TestFactoryService_CloseFactorySession_LeavesServiceAliveWithoutLiveSessions(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	if err := harness.svc.CloseFactorySession(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("CloseFactorySession(default): %v", err)
	}

	select {
	case <-liveSessionHandle(defaultSession).RunDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed default session to exit")
	}
	if harness.svc.sessions.Count() != 0 {
		t.Fatalf("live session count = %d, want 0", harness.svc.sessions.Count())
	}
	if harness.svc.currentFactory() != nil {
		t.Fatal("expected no compatibility runtime after closing the last session")
	}

	select {
	case err := <-harness.runErrCh:
		t.Fatalf("Run exited early after last-session close: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestFactoryService_CompatibilityOnlyLegacyRuntimeSurfaceTargetsDefaultSessionSelector(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	betaSession := harness.requireSession(t, betaSessionID)

	selectCompatibilitySessionForTest(t, harness.svc, betaSessionID)
	submitSessionWork(t, betaSession, "beta-only-work", "trace-beta-only")
	waitForSessionEventsToContain(t, betaSession, "beta-only-work", time.Second)

	submitCompatWork(t, harness.svc, "default-only-work", "trace-default-only")
	waitForSessionEventsToContain(t, harness.requireSession(t, defaultFactorySessionID), "default-only-work", time.Second)

	compatEvents, err := harness.svc.GetFactoryEvents(context.Background())
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

	assertSessionEventsDoNotContain(t, harness.requireSession(t, defaultFactorySessionID), "beta-only-work")
	assertSessionEventsDoNotContain(t, betaSession, "default-only-work")
}

func TestFactoryService_SessionRuntimeSurfaceTargetsExplicitSessionID(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	request := requests.WorkRequestFromSubmitRequests([]work.SubmitRequest{{
		WorkID:     "beta-session-targeted-work",
		Name:       "beta-session-targeted-work",
		WorkTypeID: "task",
		TraceID:    "trace-beta-session-targeted",
		Payload:    []byte(`{"title":"beta-session-targeted-work"}`),
	}})
	if _, err := harness.svc.SubmitWorkRequestForSession(context.Background(), betaSessionID, request); err != nil {
		t.Fatalf("SubmitWorkRequestForSession(beta): %v", err)
	}

	waitForSessionEventsToContain(t, harness.requireSession(t, betaSessionID), "beta-session-targeted-work", time.Second)
	assertSessionEventsDoNotContain(t, harness.requireSession(t, defaultFactorySessionID), "beta-session-targeted-work")

	betaCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(beta): %v", err)
	}
	if betaCurrent.Name != "beta" {
		t.Fatalf("beta current factory name = %q, want beta", betaCurrent.Name)
	}

	defaultCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(default): %v", err)
	}
	if defaultCurrent.Name != "alpha" {
		t.Fatalf("default current factory name = %q, want alpha", defaultCurrent.Name)
	}
	if _, err := harness.svc.GetEngineStateSnapshotForSession(context.Background(), "missing-session"); err == nil || !strings.Contains(err.Error(), "factory session not found") {
		t.Fatalf("GetEngineStateSnapshotForSession(missing) error = %v, want factory session not found", err)
	}
}

func TestFactoryService_Run_RestartsOnlyDefaultSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})

	if _, err := harness.svc.openFactorySession(context.Background(), harness.factoryDirs["beta"]); err != nil {
		t.Fatalf("openFactorySession(beta): %v", err)
	}
	if got := harness.svc.sessions.Count(); got != 2 {
		t.Fatalf("first run live session count = %d, want 2", got)
	}
	harness.stop(t)

	restarted := startRunningSessionServiceOnDir(t, harness.rootDir)
	defer restarted.stop(t)

	if got := restarted.svc.sessions.IDs(); len(got) != 1 || got[0] != defaultFactorySessionID {
		t.Fatalf("restarted session ids = %v, want [%s]", got, defaultFactorySessionID)
	}
	defaultSession := restarted.requireSession(t, defaultFactorySessionID)
	if liveSessionHandle(defaultSession).Bundle.Dir != harness.factoryDirs["alpha"] {
		t.Fatalf("restarted default runtime dir = %q, want %q", liveSessionHandle(defaultSession).Bundle.Dir, harness.factoryDirs["alpha"])
	}
}

func TestFactoryService_ActivateNamedFactory_ReplacesOnlyActiveSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta", "gamma"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, defaultFactorySessionID, "default runtime")
	harness.waitIdle(t, betaSessionID, "beta runtime")

	defaultHandleBefore := liveSessionHandle(harness.requireSession(t, defaultFactorySessionID))
	betaHandleBefore := liveSessionHandle(harness.requireSession(t, betaSessionID))

	if err := harness.svc.ActivateNamedFactory(context.Background(), "gamma"); err != nil {
		t.Fatalf("ActivateNamedFactory(gamma): %v", err)
	}

	defaultHandleAfter := liveSessionHandle(harness.requireSession(t, defaultFactorySessionID))
	betaHandleAfter := liveSessionHandle(harness.requireSession(t, betaSessionID))
	if defaultHandleBefore == defaultHandleAfter {
		t.Fatal("expected default session runtime handle to be replaced after named activation")
	}
	if betaHandleBefore != betaHandleAfter {
		t.Fatal("expected beta session runtime handle to remain after default named activation")
	}
	if got := harness.svc.sessions.Count(); got != 2 {
		t.Fatalf("live session count after activation = %d, want 2", got)
	}

	assertCurrentFactoryPointer(t, harness.rootDir, "gamma", "default session pointer after named activation")
	defaultCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(default) after activation: %v", err)
	}
	assertFactoryName(t, defaultCurrent.Name, "gamma", "default current factory after named activation")

	betaCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(beta) after activation: %v", err)
	}
	assertFactoryName(t, betaCurrent.Name, "beta", "beta current factory after default named activation")
}

func TestFactoryService_ActivateNamedFactory_ReplacesOnlyTargetSessionStreamGenerationID(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta", "gamma"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, defaultFactorySessionID, "default runtime")
	harness.waitIdle(t, betaSessionID, "beta runtime")

	defaultSnapshotBefore, err := harness.svc.GetEngineStateSnapshotForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession(default before): %v", err)
	}
	defaultStreamBefore, err := harness.svc.SubscribeFactoryEventsForSession(context.Background(), defaultFactorySessionID, nil)
	if err != nil {
		t.Fatalf("SubscribeFactoryEventsForSession(default before): %v", err)
	}
	betaSnapshotBefore, err := harness.svc.GetEngineStateSnapshotForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession(beta before): %v", err)
	}

	if defaultSnapshotBefore.StreamGenerationID == "" {
		t.Fatal("default snapshot stream generation id is empty")
	}
	if defaultStreamBefore.StreamGenerationID != defaultSnapshotBefore.StreamGenerationID {
		t.Fatalf("default stream generation id = %q, want snapshot id %q", defaultStreamBefore.StreamGenerationID, defaultSnapshotBefore.StreamGenerationID)
	}
	if betaSnapshotBefore.StreamGenerationID == "" {
		t.Fatal("beta snapshot stream generation id is empty")
	}

	if err := harness.svc.ActivateNamedFactory(context.Background(), "gamma"); err != nil {
		t.Fatalf("ActivateNamedFactory(gamma): %v", err)
	}

	defaultSnapshotAfter, err := harness.svc.GetEngineStateSnapshotForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession(default after): %v", err)
	}
	defaultStreamAfter, err := harness.svc.SubscribeFactoryEventsForSession(context.Background(), defaultFactorySessionID, nil)
	if err != nil {
		t.Fatalf("SubscribeFactoryEventsForSession(default after): %v", err)
	}
	betaSnapshotAfter, err := harness.svc.GetEngineStateSnapshotForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession(beta after): %v", err)
	}

	if defaultSnapshotAfter.StreamGenerationID == defaultSnapshotBefore.StreamGenerationID {
		t.Fatalf("default stream generation id after activation = %q, want distinct from %q", defaultSnapshotAfter.StreamGenerationID, defaultSnapshotBefore.StreamGenerationID)
	}
	if defaultStreamAfter.StreamGenerationID != defaultSnapshotAfter.StreamGenerationID {
		t.Fatalf("default stream generation id after activation = %q, want snapshot id %q", defaultStreamAfter.StreamGenerationID, defaultSnapshotAfter.StreamGenerationID)
	}
	if betaSnapshotAfter.StreamGenerationID != betaSnapshotBefore.StreamGenerationID {
		t.Fatalf("beta stream generation id after default replacement = %q, want unchanged %q", betaSnapshotAfter.StreamGenerationID, betaSnapshotBefore.StreamGenerationID)
	}
}

func TestFactoryService_ActivateNamedFactory_RefreshesLiveSessionIdentityAcrossReadAndHandshake(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta", "gamma"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, defaultFactorySessionID, "default runtime")
	harness.waitIdle(t, betaSessionID, "beta runtime")

	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	defaultRuntimeUUIDBefore := requireDefaultRuntimeUUIDBeforeActivation(t, server, harness.svc)
	defaultIDBefore := assertSessionStreamIdentityConsistent(t, server, harness.svc, defaultFactorySessionID, "before activation")
	betaIDBefore := assertSessionStreamIdentityConsistent(t, server, harness.svc, betaSessionID, "before activation")

	if err := harness.svc.ActivateNamedFactory(context.Background(), "gamma"); err != nil {
		t.Fatalf("ActivateNamedFactory(gamma): %v", err)
	}

	assertDefaultRuntimeUUIDPreservedAfterActivation(t, server, defaultRuntimeUUIDBefore)
	assertDefaultStreamGenerationChangedAfterActivation(t, server, harness.svc, defaultIDBefore)
	assertSessionStreamGenerationUnchanged(t, server, betaSessionID, betaIDBefore, "after default replacement")
}

func requireDefaultRuntimeUUIDBeforeActivation(t *testing.T, server *httptest.Server, svc *FactoryService) string {
	t.Helper()
	defaultSessionBefore := getLiveFactorySession(t, server.URL, defaultFactorySessionID)
	defaultRuntimeUUIDBefore := strings.TrimSpace(defaultSessionBefore.Runtime.StreamIdentity.FactorySessionID)
	if !factorysessions.IsUUIDFactorySessionID(defaultRuntimeUUIDBefore) {
		t.Fatalf("default runtime factorySessionID before activation = %q, want UUID", defaultRuntimeUUIDBefore)
	}
	return defaultRuntimeUUIDBefore
}

func assertSessionStreamIdentityConsistent(
	t *testing.T,
	server *httptest.Server,
	svc *FactoryService,
	sessionID string,
	label string,
) string {
	t.Helper()
	session := getLiveFactorySession(t, server.URL, sessionID)
	streamGenerationID := requireLiveSessionStreamGenerationID(t, session, sessionID, label)
	handshakeGenerationID := getLiveSessionEventStreamGenerationID(t, server.URL, sessionID)
	if handshakeGenerationID != streamGenerationID {
		t.Fatalf("%s handshake stream generation id = %q, want session read id %q", label, handshakeGenerationID, streamGenerationID)
	}
	preflight, err := svc.GetFactorySessionSyncPreflight(context.Background(), sessionID, interfaces.FactorySessionSyncPreflightOptions{})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(%s %s): %v", sessionID, label, err)
	}
	if preflight.StreamGenerationId == nil || *preflight.StreamGenerationId != streamGenerationID {
		t.Fatalf("%s preflight stream generation id = %#v, want session read %q", label, preflight.StreamGenerationId, streamGenerationID)
	}
	return streamGenerationID
}

func assertDefaultRuntimeUUIDPreservedAfterActivation(t *testing.T, server *httptest.Server, wantRuntimeUUID string) {
	t.Helper()
	defaultSessionAfter := getLiveFactorySession(t, server.URL, defaultFactorySessionID)
	defaultRuntimeUUIDAfter := strings.TrimSpace(defaultSessionAfter.Runtime.StreamIdentity.FactorySessionID)
	if defaultRuntimeUUIDAfter != wantRuntimeUUID {
		t.Fatalf("default runtime factorySessionID after activation = %q, want preserved %q", defaultRuntimeUUIDAfter, wantRuntimeUUID)
	}
}

func assertDefaultStreamGenerationChangedAfterActivation(
	t *testing.T,
	server *httptest.Server,
	svc *FactoryService,
	previousStreamGenerationID string,
) {
	t.Helper()
	defaultIDAfter := assertSessionStreamIdentityConsistent(t, server, svc, defaultFactorySessionID, "after activation")
	if defaultIDAfter == previousStreamGenerationID {
		t.Fatalf("default session stream generation id after activation = %q, want distinct from %q", defaultIDAfter, previousStreamGenerationID)
	}
}

func assertSessionStreamGenerationUnchanged(
	t *testing.T,
	server *httptest.Server,
	sessionID string,
	wantStreamGenerationID string,
	label string,
) {
	t.Helper()
	sessionAfter := getLiveFactorySession(t, server.URL, sessionID)
	streamGenerationIDAfter := requireLiveSessionStreamGenerationID(t, sessionAfter, sessionID, label)
	if streamGenerationIDAfter != wantStreamGenerationID {
		t.Fatalf("%s session stream generation id = %q, want unchanged %q", label, streamGenerationIDAfter, wantStreamGenerationID)
	}
	handshakeGenerationIDAfter := getLiveSessionEventStreamGenerationID(t, server.URL, sessionID)
	if handshakeGenerationIDAfter != streamGenerationIDAfter {
		t.Fatalf("%s handshake stream generation id = %q, want session read id %q", label, handshakeGenerationIDAfter, streamGenerationIDAfter)
	}
}

func TestFactoryService_SaveCurrentFactoryForSession_ReplacesOnlyTargetedSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, betaSessionID, "beta runtime")

	current, err := harness.svc.GetCurrentFactoryForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(beta): %v", err)
	}
	assertFactoryName(t, current.Name, "beta", "beta current factory name")
	if current.Version == nil {
		t.Fatal("expected beta current factory version metadata")
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  current.Version.Logical + 1,
		Physical: current.Version.Physical.Add(time.Second),
	}
	saved, err := harness.svc.SaveCurrentFactoryForSession(context.Background(), betaSessionID, replacement)
	if err != nil {
		t.Fatalf("SaveCurrentFactoryForSession(beta): %v", err)
	}
	assertFactoryWorkType(t, saved.WorkTypes, "story", "saved beta work types")

	betaCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(beta) after save: %v", err)
	}
	assertFactoryWorkType(t, betaCurrent.WorkTypes, "story", "beta current work types after save")

	defaultCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(default) after beta save: %v", err)
	}
	assertFactoryName(t, defaultCurrent.Name, "alpha", "default current factory name after beta save")
	assertFactoryWorkType(t, defaultCurrent.WorkTypes, "task", "default current work types after beta save")

	assertCurrentFactoryPointer(t, harness.rootDir, "alpha", "default session pointer after beta current-factory save")
	betaConfig, err := config.LoadRuntimeConfig(harness.factoryDirs["beta"], nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(beta) after save: %v", err)
	}
	assertPersistedFactoryWorkType(t, betaConfig.FactoryConfig().WorkTypes, "story", "persisted beta work types after save")
	if betaConfig.FactoryConfig().Version == nil || betaCurrent.Version == nil {
		t.Fatal("expected persisted and returned beta version metadata")
	}
	assertPersistedFactoryVersionMatchesAPI(t, betaConfig.FactoryConfig().Version, betaCurrent.Version, "persisted beta version after save")

	harness.waitIdle(t, betaSessionID, "beta runtime after replace-current save")
	betaSession := harness.requireSession(t, betaSessionID)
	submitSessionWorkWithType(t, betaSession, "story", "beta-after-replace-work", "trace-beta-after-replace")
	waitForSessionEventsToContain(t, betaSession, "beta-after-replace-work", time.Second)
	stream, err := liveSessionHandle(betaSession).Bundle.Factory.SubscribeFactoryEvents(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents after replace save: %v", err)
	}
	if stream == nil || stream.Events == nil {
		t.Fatal("SubscribeFactoryEvents after replace save returned nil stream")
	}

	legacyCurrent, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after beta save: %v", err)
	}
	assertFactoryName(t, legacyCurrent.Name, "alpha", "legacy current factory name after beta save")
	if _, err := harness.svc.GetCurrentFactoryForSession(context.Background(), "missing-session"); !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("GetCurrentFactoryForSession(missing) error = %v, want factory session not found", err)
	}
}

func TestFactoryService_OpenFactorySession_SubmitsWorkAndServesModelCatalogReads(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	betaSession := harness.requireSession(t, betaSessionID)
	harness.waitIdle(t, betaSessionID, "opened beta runtime")

	submitSessionWork(t, betaSession, "beta-open-session-work", "trace-beta-open-session")
	waitForSessionEventsToContain(t, betaSession, "beta-open-session-work", time.Second)

	if liveSessionHandle(betaSession).Bundle.LocalModels == nil {
		t.Fatal("opened session runtime localModels = nil, want model catalog seam")
	}
	if _, err := localmodels.ListModels(liveSessionHandle(betaSession).Bundle.RuntimeCfg); err != nil {
		t.Fatalf("ListModels on opened session runtime config: %v", err)
	}
}

func startRunningSessionServiceOnDir(t *testing.T, rootDir string) *runningSessionService {
	t.Helper()

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService(restart): %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "restarted default runtime")
	harness := &runningSessionService{
		rootDir:   rootDir,
		svc:       svc,
		runErrCh:  runErrCh,
		cancelRun: cancelRun,
	}
	t.Cleanup(func() {
		harness.stop(t)
	})
	return harness
}

func TestFactoryService_OpenFactorySessionFromFolder_AutoOpensSingleTarget(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(single target): %v", err)
	}
	if result == nil || result.SessionID == "" {
		t.Fatalf("single-target open result = %#v, want session id", result)
	}
	if len(result.Targets) != 0 {
		t.Fatalf("single-target open returned picker targets = %#v, want none", result.Targets)
	}
	session := harness.requireSession(t, result.SessionID)
	if liveSessionHandle(session).Bundle.Dir != harness.factoryDirs["alpha"] {
		t.Fatalf("opened session runtime dir = %q, want %q", liveSessionHandle(session).Bundle.Dir, harness.factoryDirs["alpha"])
	}
	if got := harness.svc.sessions.Count(); got != 2 {
		t.Fatalf("live session count = %d, want 2", got)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_ReturnsTargetPickerMetadata(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig:     minimalFactoryConfig(),
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, false, false)
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

	assertSessionTargetMetadata(t, result.Targets[0], FactorySessionTargetKindDefault, "", "default", harness.rootDir, "factory")
	assertSessionTargetMetadata(t, result.Targets[1], FactorySessionTargetKindNamed, "alpha", "alpha", filepath.Join(harness.rootDir, "alpha"), "alpha")
	assertSessionTargetMetadata(t, result.Targets[2], FactorySessionTargetKindNamed, "beta", "beta", filepath.Join(harness.rootDir, "beta"), "beta")
	if got := harness.svc.sessions.Count(); got != 1 {
		t.Fatalf("target-picker flow mutated live sessions to %d, want 1", got)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_OpensExplicitDefaultAndNamedTargets(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig:     minimalFactoryConfig(),
		namedFactories: []string{"beta"},
	})
	defer harness.stop(t)

	defaultOpen, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindDefault,
	}, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(default): %v", err)
	}
	betaOpenOne, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "beta",
	}, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(beta one): %v", err)
	}
	betaOpenTwo, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "beta",
	}, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(beta two): %v", err)
	}
	if betaOpenOne.SessionID == betaOpenTwo.SessionID {
		t.Fatalf("duplicate beta session ids = %q", betaOpenOne.SessionID)
	}

	defaultSession := harness.requireSession(t, defaultOpen.SessionID)
	betaSessionOne := harness.requireSession(t, betaOpenOne.SessionID)
	betaSessionTwo := harness.requireSession(t, betaOpenTwo.SessionID)
	if liveSessionHandle(defaultSession).Bundle.Dir != harness.rootDir {
		t.Fatalf("default target runtime dir = %q, want %q", liveSessionHandle(defaultSession).Bundle.Dir, harness.rootDir)
	}
	if liveSessionHandle(betaSessionOne).Bundle.Dir != harness.factoryDirs["beta"] || liveSessionHandle(betaSessionTwo).Bundle.Dir != harness.factoryDirs["beta"] {
		t.Fatalf("beta target runtime dirs = %q and %q, want %q", liveSessionHandle(betaSessionOne).Bundle.Dir, liveSessionHandle(betaSessionTwo).Bundle.Dir, harness.factoryDirs["beta"])
	}
	if got := harness.svc.sessions.Count(); got != 4 {
		t.Fatalf("live session count = %d, want 4", got)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_RejectsInvalidFolderAndTargetWithoutMutation(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), filepath.Join(harness.rootDir, "missing"), nil, false, false); err == nil || !strings.Contains(err.Error(), "stat factory session folder") {
		t.Fatalf("OpenFactorySessionFromFolder(missing folder) error = %v, want folder stat failure", err)
	} else {
		assertFactorySessionValidationTarget(t, err, "missing", "folderPath")
	}
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("missing-folder open mutated live sessions to %d, want %d", got, before)
	}

	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "missing",
	}, false, false); err == nil || !strings.Contains(err.Error(), `factory session target "missing" was not found`) {
		t.Fatalf("OpenFactorySessionFromFolder(missing target) error = %v, want missing-target failure", err)
	} else {
		assertFactorySessionValidationTarget(t, err, "target_not_found", "target.name")
	}
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("missing-target open mutated live sessions to %d, want %d", got, before)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_RejectsReadableFolderWithoutRunnableTargetsWithoutMutation(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	emptyDir := filepath.Join(harness.rootDir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(empty): %v", err)
	}

	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), emptyDir, nil, false, false); err == nil || !strings.Contains(err.Error(), `does not expose any runnable factory targets`) {
		t.Fatalf("OpenFactorySessionFromFolder(empty runnable folder) error = %v, want no-runnable-targets failure", err)
	} else {
		assertFactorySessionValidationTarget(t, err, "not_runnable", "folderPath")
	}
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("empty-folder open mutated live sessions to %d, want %d", got, before)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_CanceledRequestDoesNotRegisterSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "beta",
		namedFactories: []string{"beta"},
	})
	defer harness.stop(t)

	beforeIDs := harness.svc.sessions.IDs()
	if len(beforeIDs) != 1 || beforeIDs[0] != defaultFactorySessionID {
		t.Fatalf("session ids before canceled open = %v, want [%s]", beforeIDs, defaultFactorySessionID)
	}

	openCtx, cancelOpen := context.WithCancel(context.Background())
	cancelOpen()

	if _, err := harness.svc.OpenFactorySessionFromFolder(openCtx, harness.factoryDirs["beta"], nil, false, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenFactorySessionFromFolder(canceled) error = %v, want context canceled", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := harness.svc.sessions.IDs(); len(got) == 1 && got[0] == defaultFactorySessionID {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		t.Fatalf("canceled open mutated live sessions to %v, want only [%s]", harness.svc.sessions.IDs(), defaultFactorySessionID)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_ValidateOnlyReturnsTargetsWithoutOpening(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate only): %v", err)
	}
	if result == nil {
		t.Fatal("validate-only result = nil, want target metadata")
	}
	if result.SessionID != "" {
		t.Fatalf("validate-only session id = %q, want none", result.SessionID)
	}
	if len(result.Targets) != 1 {
		t.Fatalf("validate-only targets = %#v, want one target", result.Targets)
	}
	assertSessionTargetMetadata(t, result.Targets[0], FactorySessionTargetKindNamed, "alpha", "alpha", harness.factoryDirs["alpha"], "alpha")
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("validate-only mutated live sessions to %d, want %d", got, before)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_ExpandsLeadingTildeForValidationAndLaunch(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	relativeToHome, err := filepath.Rel(homeDir, harness.rootDir)
	if err != nil {
		t.Fatalf("filepath.Rel(home, root): %v", err)
	}
	if relativeToHome == "." || strings.HasPrefix(relativeToHome, "..") {
		t.Skipf("root dir %q is not under the user home %q", harness.rootDir, homeDir)
	}

	tildePath := "~"
	if relativeToHome != "." {
		tildePath = filepath.Join("~", relativeToHome)
	}

	validateResult, err := harness.svc.OpenFactorySessionFromFolder(
		context.Background(),
		tildePath,
		nil,
		true,
		false,
	)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate tilde): %v", err)
	}
	if validateResult == nil || len(validateResult.Targets) != 1 {
		t.Fatalf("validate-only tilde targets = %#v, want one target", validateResult)
	}
	assertSessionTargetMetadata(
		t,
		validateResult.Targets[0],
		FactorySessionTargetKindNamed,
		"alpha",
		"alpha",
		harness.factoryDirs["alpha"],
		"alpha",
	)
	if validateResult.Targets[0].FolderPath != harness.rootDir {
		t.Fatalf("validated tilde folder path = %q, want %q", validateResult.Targets[0].FolderPath, harness.rootDir)
	}

	openResult, err := harness.svc.OpenFactorySessionFromFolder(
		context.Background(),
		tildePath,
		&FactorySessionTargetRef{
			Kind: FactorySessionTargetKindNamed,
			Name: "alpha",
		},
		false,
		false,
	)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(open tilde): %v", err)
	}
	session := harness.requireSession(t, openResult.SessionID)
	if session.FolderPath != harness.rootDir {
		t.Fatalf("opened session folder path = %q, want %q", session.FolderPath, harness.rootDir)
	}
	if liveSessionHandle(session).Bundle.Dir != harness.factoryDirs["alpha"] {
		t.Fatalf("opened session runtime dir = %q, want %q", liveSessionHandle(session).Bundle.Dir, harness.factoryDirs["alpha"])
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_InvalidExpandedTildePathReturnsResolvedError(t *testing.T) {
	missingPath := filepath.Join("~", ".infinite-you-missing-factory-folder")

	_, err := factorysessions.ResolveSessionFolder(missingPath)
	if err == nil {
		t.Fatal("factorysessions.ResolveSessionFolder(~missing) error = nil, want failure")
	}

	homeDir, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("UserHomeDir: %v", homeErr)
	}
	wantResolvedPath := filepath.Join(homeDir, ".infinite-you-missing-factory-folder")
	if !strings.Contains(err.Error(), wantResolvedPath) {
		t.Fatalf("factorysessions.ResolveSessionFolder(~missing) error = %q, want resolved path %q", err, wantResolvedPath)
	}
	assertFactorySessionValidationTarget(t, err, "missing", "folderPath")
}

func assertFactorySessionValidationTarget(t *testing.T, err error, wantReason string, wantField string) {
	t.Helper()

	var targetedErr interface {
		ErrorTargets() []factoryvalidation.Target
	}
	if !errors.As(err, &targetedErr) {
		t.Fatalf("validation error %v did not expose structured targets", err)
	}

	targets := targetedErr.ErrorTargets()
	if len(targets) != 1 {
		t.Fatalf("validation error targets = %#v, want one target", targets)
	}
	target := targets[0]
	wantCode := "factory.session.field." + wantReason
	if target.Code != wantCode {
		t.Fatalf("validation target code = %q, want %q", target.Code, wantCode)
	}
	if target.Subject.ID != wantField {
		t.Fatalf("validation target subject id = %q, want %q", target.Subject.ID, wantField)
	}
}

func assertFactorySessionConfigLoadFailure(t *testing.T, err error, wantTargetID string) {
	t.Helper()

	reason, field, ok := factorysessions.ValidationReasonFromError(err)
	if !ok || reason != factorysessions.ValidationReasonConfigLoadFailed || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want config_load_failed folderPath", reason, field, ok)
	}

	var targetedErr interface {
		ErrorTargets() []factoryvalidation.Target
	}
	if !errors.As(err, &targetedErr) {
		t.Fatalf("config load error %v did not expose structured targets", err)
	}
	targets := targetedErr.ErrorTargets()
	if len(targets) != 1 {
		t.Fatalf("config load error targets = %#v, want one target", targets)
	}
	target := targets[0]
	if target.Code != "factory.session.target.config_load_failed" {
		t.Fatalf("config load target code = %q, want factory.session.target.config_load_failed", target.Code)
	}
	if target.Subject.ID != wantTargetID {
		t.Fatalf("config load target subject id = %q, want %q", target.Subject.ID, wantTargetID)
	}
}

func TestRuntimeWorkflowContext_SetsSessionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{name: "default session", sessionID: factorysessions.DefaultSessionID, want: factorysessions.DefaultSessionID},
		{name: "named session", sessionID: "session-beta", want: "session-beta"},
		{name: "blank session falls back to default", sessionID: "   ", want: factorysessions.DefaultSessionID},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wfCtx := runtimeWorkflowContext(&interfaces.FactoryConfig{}, tc.sessionID)
			if wfCtx == nil {
				t.Fatal("workflow context = nil")
			}
			if wfCtx.SessionID != tc.want {
				t.Fatalf("SessionID = %q, want %q", wfCtx.SessionID, tc.want)
			}
		})
	}
}

func TestBuildReplacementFactoryRuntime_WiresWorkflowContextSessionID(t *testing.T) {
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

	defaultBundle, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime(default): %v", err)
	}
	defaultCtx := runtime.WorkflowContext(defaultBundle.Factory)
	if defaultCtx == nil || defaultCtx.SessionID != factorysessions.DefaultSessionID {
		t.Fatalf("default workflow context = %#v, want SessionID %q", defaultCtx, factorysessions.DefaultSessionID)
	}

	namedBundle, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, "session-beta")
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime(named): %v", err)
	}
	namedCtx := runtime.WorkflowContext(namedBundle.Factory)
	if namedCtx == nil || namedCtx.SessionID != "session-beta" {
		t.Fatalf("named workflow context = %#v, want SessionID %q", namedCtx, "session-beta")
	}
}

func TestRuntimeWorkflowContext_RendersSessionIDInPromptTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{name: "default session", sessionID: factorysessions.DefaultSessionID, want: factory_context.DefaultSessionID},
		{name: "named session", sessionID: "session-beta", want: "session-beta"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wfCtx := runtimeWorkflowContext(&interfaces.FactoryConfig{}, tc.sessionID)
			data := workerprompting.BuildPromptData(nil, wfCtx)
			if data.Context.SessionID != tc.want {
				t.Fatalf("Context.SessionID = %q, want %q", data.Context.SessionID, tc.want)
			}
		})
	}
}
func TestFactoryService_GetFactorySession_ProjectsLegacyPetriRuntime(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		namedFactories: []string{"alpha"},
		defaultFactory: "alpha",
	})
	defer harness.stop(t)

	session, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	wantSessionID := factorysessions.CanonicalFactorySessionID(harness.requireSession(t, defaultFactorySessionID))
	if wantSessionID == defaultFactorySessionID || !factorysessions.IsUUIDFactorySessionID(wantSessionID) {
		t.Fatalf("default runtime session id = %q, want UUID distinct from %q", wantSessionID, defaultFactorySessionID)
	}
	if session.Id != wantSessionID {
		t.Fatalf("session id = %q, want %q", session.Id, wantSessionID)
	}
	if session.Runtime.OrchestratorKind != factoryapi.PETRI {
		t.Fatalf("orchestrator kind = %q, want PETRI", session.Runtime.OrchestratorKind)
	}
	if session.Runtime.StreamIdentity == nil || strings.TrimSpace(session.Runtime.StreamIdentity.StreamGenerationID) == "" {
		t.Fatalf("streamIdentity = %#v, want non-empty streamGenerationID", session.Runtime.StreamIdentity)
	}
	if session.Runtime.Petri == nil {
		t.Fatal("petri projection is nil")
	}
	if session.Runtime.Javascript != nil {
		t.Fatalf("javascript projection = %#v, want nil for Petri session", session.Runtime.Javascript)
	}
	if session.Runtime.Lifecycle.StartedAt.IsZero() || session.Runtime.Lifecycle.UpdatedAt.IsZero() {
		t.Fatalf("lifecycle = %#v, want startedAt and updatedAt", session.Runtime.Lifecycle)
	}
	if session.Runtime.Lifecycle.UpdatedAt.Before(session.Runtime.Lifecycle.StartedAt.Add(-time.Minute)) {
		t.Fatalf("lifecycle ordering = %#v", session.Runtime.Lifecycle)
	}
}

func TestFactoryService_GetFactorySession_JavaScriptStreamIdentityRemainsStableAcrossReads(t *testing.T) {
	startedAt := time.Date(2026, 6, 26, 11, 5, 0, 0, time.UTC)
	factoryCfg := &interfaces.FactoryConfig{
		Name: "dynamic-workflow",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				Dialect:   "workflow-v1",
				SourceRef: "factory/workflows/review.js",
			},
		},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, nil, nil)
	svc := &FactoryService{cfg: &FactoryServiceConfig{Dir: t.TempDir()}}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		RuntimeInstanceID: "backend-scope-js",
		BackendScopeID:    "backend-scope-js",
		StartedAtUTC:      startedAt,
		RuntimeCfg:        runtimeCfg,
		Factory: &aggregateSnapshotFactory{
			engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
				LifecycleControlStatus: string(factoryapi.FactorySessionDurableLifecycleStatusRunning),
			},
		},
	})

	first, err := svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession first read: %v", err)
	}
	second, err := svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession second read: %v", err)
	}
	if first.Runtime.StreamIdentity == nil || second.Runtime.StreamIdentity == nil {
		t.Fatalf("stream identity missing across reads: first=%#v second=%#v", first.Runtime.StreamIdentity, second.Runtime.StreamIdentity)
	}
	if first.Runtime.StreamIdentity.BackendScopeID != second.Runtime.StreamIdentity.BackendScopeID ||
		first.Runtime.StreamIdentity.FactorySessionID != second.Runtime.StreamIdentity.FactorySessionID ||
		first.Runtime.StreamIdentity.LogicalSessionKeyID != second.Runtime.StreamIdentity.LogicalSessionKeyID ||
		first.Runtime.StreamIdentity.StreamGenerationID != second.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf("stream identity changed across reads: first=%#v second=%#v", first.Runtime.StreamIdentity, second.Runtime.StreamIdentity)
	}
	if first.Runtime.StreamIdentity.StreamGenerationID != startedAt.Format(time.RFC3339Nano) {
		t.Fatalf("stream generation id = %q, want %q", first.Runtime.StreamIdentity.StreamGenerationID, startedAt.Format(time.RFC3339Nano))
	}
}

func TestFactoryService_WriteJavaScriptFactorySessionRecording_UsesProductionRecordPath(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "javascript-session.json")
	factoryCfg := &interfaces.FactoryConfig{
		Name: "recorded-workflow",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				Dialect: "workflow-v1", SourceRef: "factory/workflows/review.js",
				SourceHash:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				DefaultPolicy: json.RawMessage(`{}`),
			},
		},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, nil, nil)
	svc := &FactoryService{cfg: &FactoryServiceConfig{Dir: t.TempDir(), RecordPath: recordPath}}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		RuntimeCfg: runtimeCfg,
		Factory: &aggregateSnapshotFactory{engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			LifecycleControlStatus: string(factoryapi.FactorySessionDurableLifecycleStatusSucceeded),
		}},
	})
	if err := svc.writeJavaScriptFactorySessionRecording(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("write production JavaScript recording: %v", err)
	}
	encoded, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	var value recording.Recording
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatalf("decode recording: %v", err)
	}
	if err := recording.Validate(value); err != nil {
		t.Fatalf("validate recording: %v", err)
	}
	if value.RecordingKind != recording.KindJavaScriptFactorySession || value.Session.OrchestratorKind != interfaces.OrchestratorKindJavaScript {
		t.Fatalf("recording identity = %#v", value)
	}
	for _, forbidden := range []string{"stdout", "stderr", "diagnostics", "dispatches", "checkpointState", "providerTranscript"} {
		if strings.Contains(string(encoded), `"`+forbidden+`"`) {
			t.Fatalf("portable recording contains prohibited field %q: %s", forbidden, encoded)
		}
	}
}

func TestPortableCanonicalFactsRetainJavaScriptProjectionInputsCheckpointAndResult(t *testing.T) {
	t.Parallel()
	checkpointTime := time.Date(2026, 7, 12, 19, 30, 0, 0, time.UTC)
	argsDigest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	javascript := &interfaces.FactorySessionJavaScriptRuntimeState{
		ArgsDigest: argsDigest,
		Checkpoints: []interfaces.FactorySessionJavaScriptCheckpointRef{{
			ID: "checkpoint-1", Label: "approval", Summary: "waiting for approval", Timestamp: checkpointTime,
			ArtifactRef: &interfaces.JavaScriptCheckpointArtifactRef{ID: "artifact-checkpoint"},
		}},
		ResultStatus: "FAILED_WITH_PARTIAL",
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "safe partial result"},
			{Type: work.WorkContentPartTypeBinary, ArtifactID: "artifact-result"},
		},
	}
	succeeded := factoryapi.FactorySessionDurableLifecycleStatusFailed
	sourceRef := "workflow/review.js"
	sourceHash := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	policyHash := "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	artifactHash := "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	artifactSize := int64(12)
	capture := &factoryapi.FactoryArtifactCaptureMetadata{CapturedAt: &checkpointTime}
	artifacts := []factoryapi.FactoryArtifact{{
		Id: "artifact-checkpoint", Kind: factoryapi.FactoryArtifactKindCHECKPOINT,
		Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT, ContentHash: &artifactHash, SizeBytes: &artifactSize, CaptureMetadata: capture,
	}, {
		Id: "artifact-result", Kind: factoryapi.FactoryArtifactKindFINALRESULT,
		Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC, ContentHash: &artifactHash, SizeBytes: &artifactSize, CaptureMetadata: capture,
	}}
	facts := portableCanonicalFacts(factoryapi.FactorySession{
		Id: "default", Runtime: factoryapi.FactorySessionRuntime{
			OrchestratorKind: factoryapi.JAVASCRIPT, LifecycleControlStatus: &succeeded,
			SourceRef: &sourceRef, SourceHash: &sourceHash, PolicyHash: &policyHash, Artifacts: &artifacts,
		},
	}, javascript, nil)

	if facts.ArgumentsDigest != argsDigest {
		t.Fatalf("argumentsDigest = %q, want %q", facts.ArgumentsDigest, argsDigest)
	}
	if facts.Checkpoint == nil || facts.Checkpoint.ID != "checkpoint-1" || facts.Checkpoint.ArtifactID != "artifact-checkpoint" || facts.Checkpoint.Timestamp != checkpointTime {
		t.Fatalf("checkpoint = %#v, want canonical public checkpoint", facts.Checkpoint)
	}
	if facts.Result == nil || facts.Result.Status != "FAILED_WITH_PARTIAL" || facts.Result.Mode != "partial" || len(facts.Result.ArtifactIDs) != 1 || facts.Result.ArtifactIDs[0] != "artifact-result" {
		t.Fatalf("result = %#v, want canonical partial result", facts.Result)
	}
	if !strings.Contains(string(facts.Result.PrimaryResult), "safe partial result") {
		t.Fatalf("primaryResult = %s, want recorded public content", facts.Result.PrimaryResult)
	}
}

func TestFactoryService_PortableReplayRestoresTerminalPublicReadsWithoutLiveExecution(t *testing.T) {
	for _, tc := range []struct {
		name, status, resultStatus string
		finalStatus                factoryapi.FactorySessionDurableLifecycleStatus
		failure                    *factoryapi.FailureDetail
	}{
		{name: "completed", status: "SUCCEEDED", resultStatus: "FINAL", finalStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded},
		{name: "failed", status: "FAILED", resultStatus: "FAILED_WITH_PARTIAL", finalStatus: factoryapi.FactorySessionDurableLifecycleStatusFailed, failure: &factoryapi.FailureDetail{Reason: factoryapi.WorkFailureTypeUnknown, Message: "safe failure"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertTerminalProductionRecordThenReplay(t, tc.status, tc.resultStatus, tc.finalStatus, tc.failure)
		})
	}
}

func assertTerminalProductionRecordThenReplay(t *testing.T, status, resultStatus string, finalStatus factoryapi.FactorySessionDurableLifecycleStatus, failure *factoryapi.FailureDetail) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.json")
	argsDigest := "sha256:" + strings.Repeat("a", 64)
	history := terminalJavaScriptRecordingHistory(t, finalStatus, resultStatus, argsDigest, failure)
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", terminalJavaScriptRecordingFactoryConfig(), nil, nil)
	recorder := &FactoryService{cfg: &FactoryServiceConfig{Dir: t.TempDir(), RecordPath: path}}
	bindServiceStartupRuntime(recorder, &factoryRuntimeBundle{
		RuntimeCfg: runtimeCfg, EventHistory: history,
		Factory: &aggregateSnapshotFactory{engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{LifecycleControlStatus: string(finalStatus), TickCount: 3}},
	})
	if err := recorder.writeJavaScriptFactorySessionRecording(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("write production recording: %v", err)
	}
	value := decodeTerminalProductionRecording(t, path, argsDigest)
	provider := &countingReplayProvider{}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{ReplayPath: path, Dir: t.TempDir(), SystemConfigPath: filepath.Join(t.TempDir(), "config.json"), ProviderOverride: provider})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertTerminalProductionReplayReads(t, svc, provider, value.Session.ID, status, resultStatus, failure)
}

func decodeTerminalProductionRecording(t *testing.T, path, argsDigest string) recording.Recording {
	t.Helper()
	encoded, err := os.Open(path)
	if err != nil {
		t.Fatalf("open recording: %v", err)
	}
	defer encoded.Close()
	value, err := recording.DecodeAndValidate(encoded)
	if err != nil || value.ArgumentsDigest != argsDigest {
		t.Fatalf("recording arguments digest = %q, %v", value.ArgumentsDigest, err)
	}
	return value
}

func assertTerminalProductionReplayReads(t *testing.T, svc *FactoryService, provider *countingReplayProvider, sessionID, status, resultStatus string, failure *factoryapi.FailureDetail) {
	t.Helper()
	session, err := svc.GetDurableFactorySession(context.Background(), sessionID)
	if err != nil || string(session.Status) != status {
		t.Fatalf("session = %#v, %v", session, err)
	}
	result, err := svc.GetDurableFactorySessionResult(context.Background(), sessionID, factoryapi.GetFactorySessionResultsParams{})
	if err != nil || string(result.ResultStatus) != resultStatus || result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("result = %#v, %v", result, err)
	}
	assertTerminalReplayFailure(t, result, failure)
	events, err := svc.ReadDurableFactorySessionEvents(context.Background(), sessionID, factoryapi.GetEventsBySessionIdParams{})
	if err != nil || len(events.History) != 4 {
		t.Fatalf("events = %#v, %v", events, err)
	}
	artifacts, err := svc.ListDurableFactorySessionArtifacts(context.Background(), sessionID)
	if err != nil || len(artifacts.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, %v", artifacts, err)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("live provider calls = %d", provider.calls.Load())
	}
}

func assertTerminalReplayFailure(t *testing.T, result factoryapi.FactorySessionResult, failure *factoryapi.FailureDetail) {
	t.Helper()
	if failure == nil {
		return
	}
	if result.FailureDetail == nil || result.FailureDetail.Message != failure.Message || result.PartialResultAvailable == nil || !*result.PartialResultAvailable {
		t.Fatalf("failed result detail = %#v, want safe partial failure", result)
	}
}

func terminalJavaScriptRecordingFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{Name: "recorded-workflow", Orchestrator: &interfaces.FactoryOrchestratorConfig{
		Kind: interfaces.OrchestratorKindJavaScript,
		JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
			Dialect: "workflow-v1", SourceRef: "workflow.js",
			SourceHash: "sha256:" + strings.Repeat("b", 64), DefaultPolicy: json.RawMessage(`{}`),
		},
	}}
}

func terminalJavaScriptRecordingHistory(
	t *testing.T,
	finalStatus factoryapi.FactorySessionDurableLifecycleStatus,
	resultStatus string,
	argsDigest string,
	failure *factoryapi.FailureDetail,
) *factoryevents.FactoryEventHistory {
	t.Helper()
	eventTime := time.Date(2026, 7, 12, 18, 0, 0, 0, time.UTC)
	history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return eventTime })
	history.RecordSessionStarted(factoryevents.SessionLifecycleStartInput{
		SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, OrchestratorDialect: "workflow-v1",
		Source: "runtime", FactoryID: "recorded-workflow", SourceRef: "workflow.js",
		SourceHash: "sha256:" + strings.Repeat("b", 64), PolicyHash: "sha256:" + strings.Repeat("c", 64), ArgsDigest: argsDigest,
	}, eventTime)
	artifactHash, artifactSize := "sha256:"+strings.Repeat("d", 64), int64(16)
	capturedAt := eventTime.Add(time.Second)
	history.RecordArtifactCreated(factoryevents.ArtifactCreatedInput{
		SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, Source: "runtime", Tick: 1,
		Artifact: interfaces.FactoryArtifact{ID: "artifact-result", Kind: "FINAL_RESULT",
			Visibility: "PUBLIC", ContentHash: &artifactHash, SizeBytes: &artifactSize,
			CaptureMetadata: &interfaces.FactoryArtifactCaptureMetadata{CapturedAt: &capturedAt}},
		CapturedAt: &capturedAt,
	}, capturedAt)
	status := interfaces.FactorySessionResultStatus(resultStatus)
	history.RecordSessionResultUpdated(factoryevents.SessionLifecycleResultInput{
		SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, Source: "runtime", Tick: 2,
		ResultStatus: status, ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "recorded result"}}, ArtifactIDs: []string{"artifact-result"},
	}, eventTime.Add(2*time.Second))
	history.RecordSessionCompleted(factoryevents.SessionLifecycleCompleteInput{
		SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, Source: "runtime", Tick: 3,
		FinalStatus: interfaces.FactorySessionLifecycleStatus(finalStatus), ResultStatus: &status, ArtifactIDs: []string{"artifact-result"}, FailureDetail: sessionEventFailureDetail(failure),
	}, eventTime.Add(3*time.Second))
	return history
}

func sessionEventFailureDetail(failure *factoryapi.FailureDetail) *workerexecution.FailureDetail {
	if failure == nil {
		return nil
	}
	return &workerexecution.FailureDetail{
		Reason:  workerexecution.WorkFailureType(failure.Reason),
		Message: failure.Message,
	}
}

func TestFactoryService_PortableReplayRestoresPausedAndResumedPublicReadsWithoutLiveExecution(t *testing.T) {
	for _, tc := range []struct {
		name, status, resultStatus string
		finalStatus                factoryapi.FactorySessionDurableLifecycleStatus
		resumed                    bool
	}{
		{name: "paused", status: "PAUSED", resultStatus: "PARTIAL", finalStatus: factoryapi.FactorySessionDurableLifecycleStatusPaused},
		{name: "resumed", status: "SUCCEEDED", resultStatus: "FINAL", finalStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded, resumed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertLifecycleProductionRecordThenReplay(t, tc.status, tc.resultStatus, tc.finalStatus, tc.resumed)
		})
	}
}

func assertLifecycleProductionRecordThenReplay(t *testing.T, status, resultStatus string, finalStatus factoryapi.FactorySessionDurableLifecycleStatus, resumed bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.json")
	argsDigest := "sha256:" + strings.Repeat("7", 64)
	history, checkpoint := lifecycleJavaScriptRecordingHistory(t, finalStatus, argsDigest, resumed)
	recorder := &FactoryService{cfg: &FactoryServiceConfig{Dir: t.TempDir(), RecordPath: path}}
	bindServiceStartupRuntime(recorder, &factoryRuntimeBundle{
		RuntimeCfg:   newLoadedFactoryConfigForServiceTest(t, "", terminalJavaScriptRecordingFactoryConfig(), nil, nil),
		EventHistory: history,
		Factory: &aggregateSnapshotFactory{engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			LifecycleControlStatus: string(finalStatus), TickCount: 7,
		}},
	})
	recorder.javascriptCheckpointStoreDirect(recorder.currentSession()).Put(checkpoint)
	if err := recorder.writeJavaScriptFactorySessionRecording(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("write production lifecycle recording: %v", err)
	}
	value := decodeTerminalProductionRecording(t, path, argsDigest)
	if value.Checkpoint == nil || value.Checkpoint.Summary != checkpoint.Summary {
		t.Fatalf("recorded checkpoint = %#v, want public summary", value.Checkpoint)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lifecycle recording: %v", err)
	}
	for _, prohibited := range []string{"must-not-replay", checkpoint.StoragePath, "checkpointState", "dispatches"} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("production lifecycle recording leaked %q: %s", prohibited, encoded)
		}
	}
	provider := &countingReplayProvider{}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		ReplayPath: path, Dir: t.TempDir(), SystemConfigPath: filepath.Join(t.TempDir(), "config.json"), ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPortableLifecycleReplayReads(t, svc, provider, value.Session.ID, status, resultStatus, resumed, len(value.Events))
}

func lifecycleJavaScriptRecordingHistory(t *testing.T, finalStatus factoryapi.FactorySessionDurableLifecycleStatus, argsDigest string, resumed bool) (*factoryevents.FactoryEventHistory, interfaces.JavaScriptCheckpointRecord) {
	t.Helper()
	eventTime := time.Date(2026, 7, 12, 19, 0, 0, 0, time.UTC)
	history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return eventTime })
	history.RecordSessionStarted(factoryevents.SessionLifecycleStartInput{
		SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, OrchestratorDialect: "workflow-v1",
		Source: "runtime", FactoryID: "recorded-workflow", SourceRef: "workflow.js",
		SourceHash: "sha256:" + strings.Repeat("b", 64), PolicyHash: "sha256:" + strings.Repeat("c", 64), ArgsDigest: argsDigest,
	}, eventTime)
	checkpointAt := eventTime.Add(time.Second)
	artifactHash, artifactSize := "sha256:"+strings.Repeat("f", 64), int64(12)
	history.RecordArtifactCreated(factoryevents.ArtifactCreatedInput{
		SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, Source: "runtime", Tick: 1,
		Artifact: interfaces.FactoryArtifact{ID: "artifact-checkpoint", Kind: "CHECKPOINT",
			Visibility: "INTERNAL_CHECKPOINT", ContentHash: &artifactHash, SizeBytes: &artifactSize,
			CaptureMetadata: &interfaces.FactoryArtifactCaptureMetadata{CapturedAt: &checkpointAt}}, CapturedAt: &checkpointAt,
	}, checkpointAt)
	history.RecordOrchestratorCheckpointWritten(factoryevents.OrchestratorCheckpointWrittenInput{
		SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, OrchestratorDialect: "workflow-v1",
		CheckpointID: "checkpoint-public-1", Source: "runtime", Tick: 2, Label: "Approval", Timestamp: &checkpointAt,
		SourceHash: "sha256:" + strings.Repeat("b", 64), RuntimeSnapshotDigest: "sha256:" + strings.Repeat("8", 64),
		ArtifactRef: &interfaces.FactoryArtifactRef{ID: "artifact-checkpoint", Kind: interfaces.JavaScriptCheckpointArtifactKind,
			Visibility: interfaces.JavaScriptCheckpointArtifactVisibility, ContentHash: &artifactHash, SizeBytes: &artifactSize},
		ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable,
	}, checkpointAt)
	partialStatus := interfaces.FactorySessionResultStatusPartial
	history.RecordSessionResultUpdated(factoryevents.SessionLifecycleResultInput{
		SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, Source: "runtime", Tick: 3,
		ResultStatus: partialStatus, ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "recorded partial result"}}, ArtifactIDs: []string{"artifact-checkpoint"},
	}, eventTime.Add(2*time.Second))
	control := factoryevents.SessionLifecycleControlInput{SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, OrchestratorDialect: "workflow-v1", Source: "runtime"}
	history.RecordSessionPaused(control, eventTime.Add(3*time.Second))
	if resumed {
		history.RecordSessionResumed(control, eventTime.Add(4*time.Second))
		finalResultStatus := interfaces.FactorySessionResultStatusFinal
		history.RecordSessionResultUpdated(factoryevents.SessionLifecycleResultInput{
			SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, Source: "runtime", Tick: 6,
			ResultStatus: finalResultStatus, ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "recorded final result"}}, ArtifactIDs: []string{"artifact-checkpoint"},
		}, eventTime.Add(5*time.Second))
		history.RecordSessionCompleted(factoryevents.SessionLifecycleCompleteInput{
			SessionID: defaultFactorySessionID, OrchestratorKind: interfaces.OrchestratorKindJavaScript, Source: "runtime", Tick: 7,
			FinalStatus: interfaces.FactorySessionLifecycleStatus(finalStatus), ResultStatus: &finalResultStatus, ArtifactIDs: []string{"artifact-checkpoint"},
		}, eventTime.Add(6*time.Second))
	}
	return history, interfaces.JavaScriptCheckpointRecord{
		ID: "checkpoint-public-1", Label: "Approval", Summary: "Waiting for operator input", Timestamp: checkpointAt,
		ArtifactID: "artifact-checkpoint", ContentHash: artifactHash, SizeBytes: artifactSize,
		RawBody: json.RawMessage(`{"secret":"must-not-replay"}`), StoragePath: "/private/checkpoint.json",
	}
}

func assertPortableLifecycleReplayReads(t *testing.T, svc *FactoryService, provider *countingReplayProvider, sessionID, status, resultStatus string, resumed bool, eventCount int) {
	t.Helper()
	session, err := svc.GetDurableFactorySession(context.Background(), sessionID)
	if err != nil || string(session.Status) != status || session.Lifecycle == nil || session.Lifecycle.PausedAt == nil {
		t.Fatalf("session = %#v, %v", session, err)
	}
	if resumed != (session.Lifecycle.ResumedAt != nil) {
		t.Fatalf("resumed lifecycle = %#v, want resumed %t", session.Lifecycle, resumed)
	}
	resultRead, err := svc.GetDurableFactorySessionResult(context.Background(), sessionID, factoryapi.GetFactorySessionResultsParams{})
	if err != nil || string(resultRead.ResultStatus) != resultStatus {
		t.Fatalf("result = %#v, %v", resultRead, err)
	}
	assertPortableLifecycleReplayInspection(t, svc, provider, sessionID, eventCount)
}

func assertPortableLifecycleReplayInspection(t *testing.T, svc *FactoryService, provider *countingReplayProvider, sessionID string, eventCount int) {
	t.Helper()
	eventRead, err := svc.ReadDurableFactorySessionEvents(context.Background(), sessionID, factoryapi.GetEventsBySessionIdParams{})
	if err != nil || len(eventRead.History) != eventCount {
		t.Fatalf("events = %#v, %v", eventRead, err)
	}
	checkpointEvent := []byte(nil)
	for _, event := range eventRead.History {
		encoded, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatalf("marshal replay event: %v", marshalErr)
		}
		if strings.Contains(string(encoded), "checkpoint-public-1") {
			checkpointEvent = encoded
			break
		}
	}
	if !strings.Contains(string(checkpointEvent), "Waiting for operator input") {
		t.Fatalf("checkpoint event = %s, want public checkpoint summary", checkpointEvent)
	}
	artifactRead, err := svc.GetDurableFactorySessionArtifact(context.Background(), sessionID, "artifact-checkpoint")
	if err != nil || artifactRead.Id != "artifact-checkpoint" {
		t.Fatalf("checkpoint artifact = %#v, %v", artifactRead, err)
	}
	dispatches, err := svc.ListDurableFactorySessionDispatches(
		context.Background(), sessionID, factoryapi.ListFactorySessionDispatchesParams{},
	)
	if err != nil || len(dispatches.Dispatches) != 0 {
		t.Fatalf("dispatches = %#v, %v", dispatches, err)
	}
	_, controlErr := svc.ResumeDurableFactorySession(context.Background(), sessionID, factoryapi.FactorySessionLifecycleControlRequest{})
	if !errors.Is(controlErr, recordingreplay.ErrNonLiveReplay) {
		t.Fatalf("resume replay error = %v, want ErrNonLiveReplay", controlErr)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("live provider calls = %d", provider.calls.Load())
	}
}

type countingReplayProvider struct{ calls atomic.Int32 }

func (p *countingReplayProvider) Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.calls.Add(1)
	return workerexecution.InferenceResponse{}, errors.New("live provider must not be used during recording replay")
}

func TestFactoryService_GetFactorySession_JavaScriptStreamIdentityMatchesEventHandshakeSnapshotToken(t *testing.T) {
	startedAt := time.Date(2026, 6, 27, 8, 0, 0, 0, time.UTC)
	factoryCfg := &interfaces.FactoryConfig{
		Name: "dynamic-workflow",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				Dialect:   "workflow-v1",
				SourceRef: "factory/workflows/review.js",
			},
		},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, nil, nil)
	svc := &FactoryService{cfg: &FactoryServiceConfig{Dir: t.TempDir()}}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		RuntimeInstanceID: "backend-scope-js",
		BackendScopeID:    "backend-scope-js",
		StartedAtUTC:      startedAt,
		RuntimeCfg:        runtimeCfg,
		Factory: &aggregateSnapshotFactory{
			engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
				LifecycleControlStatus: string(factoryapi.FactorySessionDurableLifecycleStatusRunning),
				StreamGenerationID:     "snapshot-stream-token",
			},
		},
	})
	server := api.NewServer(svc, 0, zap.NewNop()).Handler()
	sessionRecorder := httptest.NewRecorder()
	sessionRequest := httptest.NewRequest(http.MethodGet, "/factory-sessions/"+defaultFactorySessionID, nil)
	server.ServeHTTP(sessionRecorder, sessionRequest)
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/%s status = %d, want 200", defaultFactorySessionID, sessionRecorder.Code)
	}
	var session factoryapi.FactorySession
	if err := json.NewDecoder(sessionRecorder.Body).Decode(&session); err != nil {
		t.Fatalf("decode factory session: %v", err)
	}
	streamGenerationID := requireLiveSessionStreamGenerationID(t, session, defaultFactorySessionID, "javascript session read")
	if streamGenerationID != "snapshot-stream-token" {
		t.Fatalf("session read stream generation id = %q, want snapshot token", streamGenerationID)
	}
	eventsCtx, cancelEvents := context.WithCancel(context.Background())
	cancelEvents()
	eventsRecorder := httptest.NewRecorder()
	eventsRequest := httptest.NewRequest(http.MethodGet, "/factory-sessions/"+defaultFactorySessionID+"/events", nil).WithContext(eventsCtx)
	server.ServeHTTP(eventsRecorder, eventsRequest)
	if eventsRecorder.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/%s/events status = %d, want 200", defaultFactorySessionID, eventsRecorder.Code)
	}
	handshakeGenerationID := eventsRecorder.Header().Get("X-Factory-Session-Stream-Generation-Id")
	if handshakeGenerationID != streamGenerationID {
		t.Fatalf("event handshake stream generation id = %q, want session read id %q", handshakeGenerationID, streamGenerationID)
	}
}

func TestFactoryService_ListFactorySessions_IncludesRuntimeProjection(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		namedFactories: []string{"alpha"},
		defaultFactory: "alpha",
	})
	defer harness.stop(t)

	listed, err := harness.svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(listed.Sessions) == 0 {
		t.Fatal("expected at least one live session")
	}
	wantSessionID := factorysessions.CanonicalFactorySessionID(harness.requireSession(t, defaultFactorySessionID))
	if wantSessionID == defaultFactorySessionID || !factorysessions.IsUUIDFactorySessionID(wantSessionID) {
		t.Fatalf("default runtime session id = %q, want UUID distinct from %q", wantSessionID, defaultFactorySessionID)
	}
	found := false
	for _, summary := range listed.Sessions {
		if summary.Id != wantSessionID {
			continue
		}
		found = true
		if summary.Runtime == nil {
			t.Fatal("default session summary missing runtime projection")
		}
		if summary.Runtime.OrchestratorKind != factoryapi.PETRI {
			t.Fatalf("orchestrator kind = %q, want PETRI", summary.Runtime.OrchestratorKind)
		}
	}
	if !found {
		t.Fatalf("sessions = %#v, want default session", listed.Sessions)
	}
}
func TestFactoryService_CancelDurableFactorySession_RuntimeBackedSession(t *testing.T) {
	t.Parallel()

	fs := newFactoryServiceForDurableLifecycleTest(t, "busy-loop.workflow.js", "busy-loop")
	ctx := context.Background()

	started, err := fs.StartDurableFactorySessionAsync(ctx, factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-lifecycle-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionAsync: %v", err)
	}

	response, err := fs.CancelDurableFactorySession(ctx, started.SessionId, factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("CancelDurableFactorySession: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("operation = %q, want CANCEL", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling {
		t.Fatalf("status = %q, want CANCELING", response.Status)
	}
	waitForDurableLifecycleStatus(t, fs, started.SessionId, factorysessionexecution.LifecycleStatusCanceled)
}

func TestFactoryService_CancelDurableFactorySession_HTTPUsesProductionRuntime(t *testing.T) {
	t.Parallel()

	fs := newFactoryServiceForDurableLifecycleTest(t, "busy-loop.workflow.js", "busy-loop")
	server := httptest.NewServer(api.NewServer(fs, 0, zap.NewNop()).Handler())
	defer server.Close()

	started, err := fs.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-lifecycle-http-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionAsync: %v", err)
	}

	url := server.URL + "/factory-sessions/" + started.SessionId + "/cancel"
	resp, err := http.Post(url, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("operation = %q, want CANCEL", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	waitForDurableLifecycleStatus(t, fs, started.SessionId, factorysessionexecution.LifecycleStatusCanceled)
}

func TestFactoryService_PauseDurableFactorySession_HTTPUsesProductionRuntime(t *testing.T) {
	t.Parallel()

	fs := newFactoryServiceForDurableLifecycleTest(t, "busy-loop.workflow.js", "busy-loop")
	server := httptest.NewServer(api.NewServer(fs, 0, zap.NewNop()).Handler())
	defer server.Close()

	started, err := fs.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-lifecycle-http-pause-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionAsync: %v", err)
	}

	url := server.URL + "/factory-sessions/" + started.SessionId + "/pause"
	resp, err := http.Post(url, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}
}

func TestFactoryService_TerminateDurableFactorySession_HTTPUsesProductionRuntime(t *testing.T) {
	t.Parallel()

	fs := newFactoryServiceForDurableLifecycleTest(t, "busy-loop.workflow.js", "busy-loop")
	server := httptest.NewServer(api.NewServer(fs, 0, zap.NewNop()).Handler())
	defer server.Close()

	started, err := fs.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-lifecycle-http-terminate-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionAsync: %v", err)
	}

	url := server.URL + "/factory-sessions/" + started.SessionId + "/terminate"
	resp, err := http.Post(url, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindTerminate {
		t.Fatalf("operation = %q, want TERMINATE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusTerminated {
		t.Fatalf("status = %q, want TERMINATED", response.Status)
	}
}

func TestFactoryService_ResumeDurableFactorySession_HTTPUsesProductionRuntime(t *testing.T) {
	t.Parallel()

	fs := newFactoryServiceForDurableLifecycleTest(t, "busy-loop.workflow.js", "busy-loop")
	server := httptest.NewServer(api.NewServer(fs, 0, zap.NewNop()).Handler())
	defer server.Close()

	started, err := fs.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-lifecycle-http-resume-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionAsync: %v", err)
	}

	sessionPath := "/factory-sessions/" + started.SessionId
	pauseURL := server.URL + sessionPath + "/pause"
	pauseResp, err := http.Post(pauseURL, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST %s: %v", pauseURL, err)
	}
	pauseResp.Body.Close()
	if pauseResp.StatusCode != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", pauseResp.StatusCode)
	}

	resumeURL := server.URL + sessionPath + "/resume"
	resp, err := http.Post(resumeURL, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST %s: %v", resumeURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
	if response.Links == nil || response.Links.Session == nil || *response.Links.Session != sessionPath {
		t.Fatalf("links = %#v, want session %q", response.Links, sessionPath)
	}
}

func TestFactoryService_LifecycleControls_PreserveReadSurfacesAfterPauseResume_HTTPUsesProductionRuntime(t *testing.T) {
	t.Parallel()

	fs := newFactoryServiceForDurableLifecycleTest(t, "busy-loop.workflow.js", "busy-loop")
	server := httptest.NewServer(api.NewServer(fs, 0, zap.NewNop()).Handler())
	defer server.Close()

	started, err := fs.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-lifecycle-read-parity-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("busy-loop"),
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionAsync: %v", err)
	}

	ctx := context.Background()
	sessionID := started.SessionId
	beforeEvents, err := fs.ReadDurableFactorySessionEvents(ctx, sessionID, factoryapi.GetEventsBySessionIdParams{})
	if err != nil {
		t.Fatalf("ReadDurableFactorySessionEvents before controls: %v", err)
	}
	beforeEventCount := len(beforeEvents.History)

	sessionPath := "/factory-sessions/" + sessionID
	pauseURL := server.URL + sessionPath + "/pause"
	pauseResp, err := http.Post(pauseURL, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST %s: %v", pauseURL, err)
	}
	pauseResp.Body.Close()
	if pauseResp.StatusCode != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", pauseResp.StatusCode)
	}

	assertProductionDurableReadSurfacesReachable(t, server.URL, sessionID)

	resumeURL := server.URL + sessionPath + "/resume"
	resp, err := http.Post(resumeURL, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST %s: %v", resumeURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", resp.StatusCode)
	}

	assertProductionDurableReadSurfacesReachable(t, server.URL, sessionID)

	afterEvents, err := fs.ReadDurableFactorySessionEvents(ctx, sessionID, factoryapi.GetEventsBySessionIdParams{})
	if err != nil {
		t.Fatalf("ReadDurableFactorySessionEvents after controls: %v", err)
	}
	if len(afterEvents.History) < beforeEventCount {
		t.Fatalf("event count after lifecycle controls = %d, want at least %d", len(afterEvents.History), beforeEventCount)
	}

	read, err := fs.GetDurableFactorySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetDurableFactorySession: %v", err)
	}
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session status after resume = %q, want RUNNING", read.Status)
	}
}

func assertProductionDurableReadSurfacesReachable(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	base := serverURL + "/factory-sessions/" + sessionID
	for _, path := range []string{
		base,
		base + "/results",
		base + "/dispatches",
		base + "/artifacts",
	} {
		resp, err := http.Get(path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestFactoryService_RetryDurableFactorySessionDispatch_RuntimeBackedFailedSession(t *testing.T) {
	t.Parallel()

	fs, sessionID, dispatchID := newFactoryServiceForDurableRetryDispatchTest(t)
	ctx := context.Background()

	response, err := fs.RetryDurableFactorySessionDispatch(ctx, sessionID, factoryapi.FactorySessionRetryDispatchRequest{
		DispatchId: dispatchID,
	})
	if err != nil {
		t.Fatalf("RetryDurableFactorySessionDispatch: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindRetryDispatch {
		t.Fatalf("operation = %q, want RETRY_DISPATCH", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
	if response.RetryDispatchId == nil || *response.RetryDispatchId != dispatchID {
		t.Fatalf("retryDispatchId = %#v, want %q", response.RetryDispatchId, dispatchID)
	}
}

func TestFactoryService_RetryDurableFactorySessionDispatch_HTTPUsesProductionRuntime(t *testing.T) {
	t.Parallel()

	fs, sessionID, dispatchID := newFactoryServiceForDurableRetryDispatchTest(t)
	server := httptest.NewServer(api.NewServer(fs, 0, zap.NewNop()).Handler())
	defer server.Close()

	url := server.URL + "/factory-sessions/" + sessionID + "/retry-dispatch"
	body, err := json.Marshal(factoryapi.FactorySessionRetryDispatchRequest{DispatchId: dispatchID})
	if err != nil {
		t.Fatalf("marshal retry-dispatch request: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindRetryDispatch {
		t.Fatalf("operation = %q, want RETRY_DISPATCH", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
	if response.RetryDispatchId == nil || *response.RetryDispatchId != dispatchID {
		t.Fatalf("retryDispatchId = %#v, want %q", response.RetryDispatchId, dispatchID)
	}
}

type durableRetryDispatchFailingChildProvider struct{}

func (durableRetryDispatchFailingChildProvider) Infer(
	_ context.Context,
	_ workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, workerprovider.NewProviderError(
		workerexecution.WorkFailureTypePermanentBadRequest,
		"simulated live child error",
		nil,
	)
}

func newFactoryServiceForDurableRetryDispatchTest(t *testing.T) (*FactoryService, string, string) {
	t.Helper()
	const dispatchID = "dispatch-1"

	projectRoot := setupDurableLifecycleWorkflowFixture(t, "agent-run-live-child-failure.workflow.js", "agent-run-live-child-failure")
	fs := &FactoryService{
		cfg: &FactoryServiceConfig{
			Dir: projectRoot,
		},
		factoryRootDir: projectRoot,
	}
	fs.durableExecution = factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeLive,
		Provider:          durableRetryDispatchFailingChildProvider{},
	})

	completed, err := fs.StartDurableFactorySessionSync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-factory-service-retry-dispatch-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("agent-run-live-child-failure"),
		},
		Args: &map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartDurableFactorySessionSync: %v", err)
	}
	if completed.SessionId == "" {
		t.Fatal("session id unexpectedly empty")
	}

	read, err := fs.durableExecution.GetSession(context.Background(), completed.SessionId)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", read.Status)
	}

	return fs, completed.SessionId, dispatchID
}

func TestFactoryService_DurableOperationsRequireInjectedExecution(t *testing.T) {
	t.Parallel()

	fs := &FactoryService{}
	_, startErr := fs.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-missing-durable-execution",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr("missing-execution"),
		},
	})
	if !errors.Is(startErr, factorysessionexecution.ErrServiceNotConfigured) {
		t.Fatalf("StartDurableFactorySessionAsync error = %v, want missing execution error", startErr)
	}

	_, listErr := fs.ListDurableExecutionSessions(context.Background(), factorysessionexecution.ListSessionsRequest{})
	if !errors.Is(listErr, factorysessionexecution.ErrServiceNotConfigured) {
		t.Fatalf("ListDurableExecutionSessions error = %v, want missing execution error", listErr)
	}
	if fs.durableExecution != nil {
		t.Fatal("durable operation lazily created hidden execution state")
	}
}

func newFactoryServiceForDurableLifecycleTest(t *testing.T, fixtureName, workflowName string) *FactoryService {
	t.Helper()
	projectRoot := setupDurableLifecycleWorkflowFixture(t, fixtureName, workflowName)
	execution, err := factorysessionexecution.NewExecutionService(
		factorysessionexecution.ExecutionProviderJavaScriptRuntime,
		factorysessionexecution.ServiceConfig{ProjectRoot: projectRoot, Persistence: factorysessionexecution.DisabledPersistence(), Clock: factory.EnsureClock(nil)},
	)
	if err != nil {
		t.Fatalf("compose durable execution: %v", err)
	}
	return &FactoryService{
		cfg: &FactoryServiceConfig{
			Dir: projectRoot,
		},
		factoryRootDir:   projectRoot,
		durableExecution: execution,
	}
}

func waitForDurableLifecycleStatus(
	t *testing.T,
	fs *FactoryService,
	sessionID string,
	want factorysessionexecution.LifecycleStatus,
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		read, err := fs.durableExecution.GetSession(context.Background(), sessionID)
		if err == nil && read.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	read, err := fs.durableExecution.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession(%q): %v", sessionID, err)
	}
	t.Fatalf("session %q status = %q, want %q", sessionID, read.Status, want)
}

func setupDurableLifecycleWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "orchestrators", "javascript", "runtime", "testdata", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func strPtr(value string) *string {
	return &value
}

func TestFactoryService_LiveSessionPauseResume_HTTPReturnsTypedLifecycleControl(t *testing.T) {
	t.Parallel()

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	sessionID := factorysessions.DefaultSessionID
	sessionPath := "/factory-sessions/" + sessionID

	pauseResp, pauseStatus := postLiveSessionLifecycleControl(t, server.URL, sessionID, "pause")
	if pauseStatus != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", pauseStatus)
	}
	if pauseResp.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", pauseResp.Operation)
	}
	if pauseResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", pauseResp.Outcome)
	}
	if pauseResp.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", pauseResp.Status)
	}

	pausedSession := getLiveFactorySession(t, server.URL, sessionID)
	if pausedSession.Runtime.Progress.FactoryState != string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state after pause = %q, want PAUSED", pausedSession.Runtime.Progress.FactoryState)
	}

	resumeResp, resumeStatus := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume")
	if resumeStatus != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", resumeStatus)
	}
	if resumeResp.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", resumeResp.Operation)
	}
	if resumeResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", resumeResp.Outcome)
	}
	if resumeResp.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", resumeResp.Status)
	}

	runningSession := getLiveFactorySession(t, server.URL, sessionID)
	if runningSession.Runtime.Progress.FactoryState == string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state after resume = %q, want not PAUSED", runningSession.Runtime.Progress.FactoryState)
	}
	if pauseResp.Links == nil || pauseResp.Links.Session == nil || *pauseResp.Links.Session != sessionPath {
		t.Fatalf("pause links = %#v, want session %q", pauseResp.Links, sessionPath)
	}
}

func TestFactoryService_LiveSessionResume_HTTPNoOpWhenAlreadyRunning(t *testing.T) {
	t.Parallel()

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	sessionID := factorysessions.DefaultSessionID

	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "pause"); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}
	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume"); status != http.StatusOK {
		t.Fatalf("first resume status = %d, want 200", status)
	}

	resumeResp, resumeStatus := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume")
	if resumeStatus != http.StatusOK {
		t.Fatalf("second resume status = %d, want 200", resumeStatus)
	}
	if resumeResp.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", resumeResp.Operation)
	}
	if resumeResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", resumeResp.Outcome)
	}
	if resumeResp.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", resumeResp.Status)
	}
}

func TestFactoryService_LiveSessionPauseResume_HTTPEmitsSessionLifecycleControlEvents(t *testing.T) {
	t.Parallel()

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	sessionID := factorysessions.DefaultSessionID

	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "pause"); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}
	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume"); status != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", status)
	}

	events, err := harness.svc.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	lifecycleControls := filterSessionLifecycleControlEvents(events)
	if len(lifecycleControls) != 2 {
		t.Fatalf("SESSION_LIFECYCLE_CONTROL events = %d, want pause and resume", len(lifecycleControls))
	}
	assertAcceptedSessionLifecycleControlPayload(
		t,
		lifecycleControls[0],
		factoryapi.FactorySessionLifecycleControlKindPause,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)
	assertAcceptedSessionLifecycleControlPayload(
		t,
		lifecycleControls[1],
		factoryapi.FactorySessionLifecycleControlKindResume,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)
}

func filterSessionLifecycleControlEvents(events []factoryapi.FactoryEvent) []factoryapi.FactoryEvent {
	var lifecycleControls []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeSessionLifecycleControl {
			lifecycleControls = append(lifecycleControls, event)
		}
	}
	return lifecycleControls
}

func assertAcceptedSessionLifecycleControlPayload(
	t *testing.T,
	event factoryapi.FactoryEvent,
	operation factoryapi.FactorySessionLifecycleControlKind,
	previousStatus factoryapi.FactorySessionDurableLifecycleStatus,
	newStatus factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	payload, err := event.Payload.AsSessionLifecycleControlEventPayload()
	if err != nil {
		t.Fatalf("lifecycle payload: %v", err)
	}
	if payload.Operation != operation ||
		payload.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		payload.PreviousStatus != previousStatus ||
		payload.NewStatus != newStatus {
		t.Fatalf("lifecycle payload = %#v, want %s %s->%s ACCEPTED", payload, operation, previousStatus, newStatus)
	}
}

func TestFactoryService_LiveSessionPauseResume_HTTPDrainsBufferedSubmissionWithoutExternalSignal(t *testing.T) {
	t.Parallel()

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	sessionID := factorysessions.DefaultSessionID

	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "pause"); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}
	waitForSessionFactoryState(t, harness.svc, sessionID, interfaces.FactoryStatePaused, time.Second, "live session paused")

	submitStatus := postLiveSessionWork(t, server.URL, sessionID, `{"name":"api-paused-submit","workTypeName":"task","traceId":"trace-api-paused-submit"}`)
	if submitStatus != http.StatusOK && submitStatus != http.StatusCreated {
		t.Fatalf("submit status = %d, want 200 or 201", submitStatus)
	}

	assertSessionWorkNotAtPlace(t, harness.svc, sessionID, "task:complete", 300*time.Millisecond)

	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume"); status != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", status)
	}
	waitForSessionFactoryState(t, harness.svc, sessionID, interfaces.FactoryStateRunning, time.Second, "live session resumed")
	waitForSessionWorkAtPlace(t, harness.svc, sessionID, "task:complete", 2*time.Second)

	resumedSession := getLiveFactorySession(t, server.URL, sessionID)
	if resumedSession.Runtime.Progress.FactoryState == string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state after drain = %q, want not PAUSED", resumedSession.Runtime.Progress.FactoryState)
	}
}

func postLiveSessionLifecycleControl(
	t *testing.T,
	serverURL, sessionID, operation string,
) (factoryapi.FactorySessionLifecycleControlResponse, int) {
	t.Helper()
	resp, err := http.Post(serverURL+"/factory-sessions/"+sessionID+"/"+operation, "application/json", nil)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/%s: %v", sessionID, operation, err)
	}
	defer resp.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	return response, resp.StatusCode
}

func postLiveSessionWork(t *testing.T, serverURL, sessionID, body string) int {
	t.Helper()
	resp, err := http.Post(
		serverURL+"/factory-sessions/"+sessionID+"/work",
		"application/json",
		bytes.NewReader([]byte(body)),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/work: %v", sessionID, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func getLiveFactorySession(t *testing.T, serverURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s: %v", sessionID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /factory-sessions/%s status = %d, want 200", sessionID, resp.StatusCode)
	}
	var session factoryapi.FactorySession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode factory session: %v", err)
	}
	return session
}

func getLiveSessionEventStreamGenerationID(t *testing.T, serverURL, sessionID string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/factory-sessions/"+sessionID+"/events", nil)
	if err != nil {
		t.Fatalf("new GET /factory-sessions/%s/events request: %v", sessionID, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s/events: %v", sessionID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /factory-sessions/%s/events status = %d, want 200", sessionID, resp.StatusCode)
	}
	return resp.Header.Get("X-Factory-Session-Stream-Generation-Id")
}

func requireLiveSessionStreamGenerationID(t *testing.T, session factoryapi.FactorySession, sessionID, label string) string {
	t.Helper()
	if session.Runtime.StreamIdentity == nil || strings.TrimSpace(session.Runtime.StreamIdentity.StreamGenerationID) == "" {
		t.Fatalf("%s session read streamIdentity for %s = %#v, want non-empty streamGenerationID", label, sessionID, session.Runtime.StreamIdentity)
	}
	return strings.TrimSpace(session.Runtime.StreamIdentity.StreamGenerationID)
}

func assertSessionWorkNotAtPlace(t *testing.T, svc *FactoryService, sessionID, placeID string, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if sessionHasWorkAtPlace(t, svc, sessionID, placeID) {
			t.Fatalf("work reached %s while session %s remained paused", placeID, sessionID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForSessionWorkAtPlace(t *testing.T, svc *FactoryService, sessionID, placeID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sessionHasWorkAtPlace(t, svc, sessionID, placeID) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work at %s in session %s", placeID, sessionID)
}

func sessionHasWorkAtPlace(t *testing.T, svc *FactoryService, sessionID, placeID string) bool {
	t.Helper()
	snapshot, err := svc.GetEngineStateSnapshotForSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession(%s): %v", sessionID, err)
	}
	for _, token := range snapshot.Marking.Tokens {
		if token.PlaceID == placeID {
			return true
		}
	}
	return false
}

func TestFactoryService_PauseLiveFactorySession_AcceptsRunningSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	response, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}

	waitForSessionFactoryState(
		t,
		harness.svc,
		defaultFactorySessionID,
		interfaces.FactoryStatePaused,
		time.Second,
		"live session paused",
	)
}

func TestFactoryService_ResumeLiveFactorySession_AcceptsPausedSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	response, err := harness.svc.ResumeLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession: %v", err)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestFactoryService_PauseLiveFactorySession_RepeatPauseReturnsNoOp(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	response, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err != nil {
		t.Fatalf("repeat PauseLiveFactorySession: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}
}

func TestFactoryService_PauseLiveFactorySession_MissingSessionReturnsNotFound(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	_, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		"live-session-missing-001",
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err == nil {
		t.Fatal("PauseLiveFactorySession = nil, want not found")
	}
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("error = %v, want ErrFactorySessionNotFound", err)
	}
}

func TestFactoryService_ResumeLiveFactorySession_RunningSessionReturnsNoOp(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	response, err := harness.svc.ResumeLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestObserveLiveLifecycleControl_LogsAcceptedPauseWithoutSensitiveFields(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
		logger:     zap.New(core),
	})
	defer harness.stop(t)
	observed.TakeAll()

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{
			RequestId: strPtr("pause-req-001"),
			Reason:    strPtr("operator pause with secret /Users/me/prompt.txt"),
		},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control")
	assertLogField(t, entry, "session_id", defaultFactorySessionID)
	assertLogField(t, entry, "operation", "PAUSE")
	assertLogField(t, entry, "outcome", "ACCEPTED")
	assertLogField(t, entry, "lifecycle_control_status", "PAUSED")
	assertLogField(t, entry, "request_id", "pause-req-001")
	assertLogDoesNotContain(t, entry, "prompt")
	assertLogDoesNotContain(t, entry, "/Users/")
}

func TestObserveLiveLifecycleControl_LogsNoOpRepeatPause(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
		logger:     zap.New(core),
	})
	defer harness.stop(t)
	observed.TakeAll()

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("initial PauseLiveFactorySession: %v", err)
	}
	observed.TakeAll()

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("repeat PauseLiveFactorySession: %v", err)
	}

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control")
	assertLogField(t, entry, "outcome", "NO_OP")
}

func TestObserveLiveLifecycleControl_LogsNoOpResume(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
		logger:     zap.New(core),
	})
	defer harness.stop(t)
	observed.TakeAll()

	response, err := harness.svc.ResumeLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", response.Outcome)
	}

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control")
	assertLogField(t, entry, "operation", "RESUME")
	assertLogField(t, entry, "outcome", "NO_OP")
}

func TestObserveLiveLifecycleControl_LogsNotFound(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
		logger:     zap.New(core),
	})
	defer harness.stop(t)
	observed.TakeAll()

	_, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		"live-session-missing-001",
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err == nil {
		t.Fatal("PauseLiveFactorySession = nil, want not found")
	}
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("error = %v, want ErrFactorySessionNotFound", err)
	}

	entry := findLifecycleControlLogForSession(t, observed, "factory session lifecycle control rejected", "live-session-missing-001")
	assertLogField(t, entry, "session_id", "live-session-missing-001")
	assertLogField(t, entry, "outcome", "NOT_FOUND")
}

func TestObserveLiveLifecycleControl_EmitsAcceptedPauseMetric(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	session := harness.svc.sessionByID(defaultFactorySessionID)
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil || liveSessionHandle(session).Bundle.MetricsSink == nil {
		t.Fatal("live session runtime metrics sink is required")
	}
	metricsPath := liveSessionHandle(session).Bundle.MetricsSink.Path()

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleControl, 1) &&
			metricRecordString(record, "outcome") == "ACCEPTED" &&
			metricRecordString(record, "reason") == "PAUSE"
	}, "accepted pause lifecycle control")
}

func findLifecycleControlLog(t *testing.T, observed *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()
	for _, entry := range observed.All() {
		if entry.Message == message {
			return entry
		}
	}
	t.Fatalf("lifecycle control log %q not found in %#v", message, observed.All())
	return observer.LoggedEntry{}
}

func findLifecycleControlLogForSession(t *testing.T, observed *observer.ObservedLogs, message, sessionID string) observer.LoggedEntry {
	t.Helper()
	for _, entry := range observed.All() {
		if entry.Message == message && entry.ContextMap()["session_id"] == sessionID {
			return entry
		}
	}
	t.Fatalf("lifecycle control log %q for session %q not found in %#v", message, sessionID, observed.All())
	return observer.LoggedEntry{}
}

func findObservedLog(t *testing.T, observed *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()
	for _, entry := range observed.All() {
		if entry.Message == message {
			return entry
		}
	}
	t.Fatalf("log %q not found in %#v", message, observed.All())
	return observer.LoggedEntry{}
}

func assertLogField(t *testing.T, entry observer.LoggedEntry, key, want string) {
	t.Helper()
	var values []string
	for _, field := range entry.Context {
		if field.Key != key {
			continue
		}
		values = append(values, field.String)
		if field.String == want {
			return
		}
	}
	if len(values) > 0 {
		t.Fatalf("log field %q values = %q, want one equal to %q", key, values, want)
	}
	t.Fatalf("log field %q missing from %#v", key, entry.Context)
}

func assertLogDoesNotContain(t *testing.T, entry observer.LoggedEntry, fragment string) {
	t.Helper()
	for _, field := range entry.Context {
		if strings.Contains(field.String, fragment) {
			t.Fatalf("log field %q leaked %q: %q", field.Key, fragment, field.String)
		}
	}
}

func TestFactoryService_GetFactorySession_ReflectsPausedLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	waitForSessionFactoryState(
		t,
		harness.svc,
		defaultFactorySessionID,
		interfaces.FactoryStatePaused,
		time.Second,
		"live session paused",
	)

	session, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatal("lifecycleControlStatus = nil, want PAUSED")
	}
	if *session.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", *session.Runtime.LifecycleControlStatus)
	}
	if session.Runtime.Progress.FactoryState != string(interfaces.FactoryStatePaused) {
		t.Fatalf("progress.factoryState = %q, want raw engine snapshot PAUSED", session.Runtime.Progress.FactoryState)
	}
}

func TestFactoryService_GetFactorySession_UntouchedSessionPreservesRawFactoryState(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	snapshot, err := harness.svc.GetEngineStateSnapshotForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession: %v", err)
	}

	session, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Runtime.LifecycleControlStatus != nil {
		t.Fatalf("lifecycleControlStatus = %#v, want unset before canonical pause/resume events", session.Runtime.LifecycleControlStatus)
	}
	if session.Runtime.Progress.FactoryState != snapshot.FactoryState {
		t.Fatalf("progress.factoryState = %q, want raw engine snapshot %q", session.Runtime.Progress.FactoryState, snapshot.FactoryState)
	}
}

func TestFactoryService_GetFactorySession_ReflectsResumedLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if _, err := harness.svc.ResumeLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("ResumeLiveFactorySession: %v", err)
	}

	waitForSessionFactoryState(
		t,
		harness.svc,
		defaultFactorySessionID,
		interfaces.FactoryStateRunning,
		time.Second,
		"live session resumed",
	)

	session, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatal("lifecycleControlStatus = nil, want RUNNING")
	}
	if *session.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("lifecycleControlStatus = %q, want RUNNING", *session.Runtime.LifecycleControlStatus)
	}
}

func TestFactoryService_GetFactorySession_RepeatPauseDoesNotChangeLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	before, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession before repeat pause: %v", err)
	}

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("repeat PauseLiveFactorySession: %v", err)
	}
	after, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession after repeat pause: %v", err)
	}
	if before.Runtime.LifecycleControlStatus == nil || after.Runtime.LifecycleControlStatus == nil {
		t.Fatalf("lifecycleControlStatus missing: before=%#v after=%#v", before.Runtime.LifecycleControlStatus, after.Runtime.LifecycleControlStatus)
	}
	if *before.Runtime.LifecycleControlStatus != *after.Runtime.LifecycleControlStatus {
		t.Fatalf("lifecycleControlStatus changed after no-op pause: before=%q after=%q",
			*before.Runtime.LifecycleControlStatus, *after.Runtime.LifecycleControlStatus)
	}
}

func TestFactoryService_GetEngineStateSnapshotForSession_ReflectsLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	snapshot, err := harness.svc.GetEngineStateSnapshotForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession: %v", err)
	}
	if snapshot.LifecycleControlStatus != string(factoryapi.FactorySessionDurableLifecycleStatusPaused) {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", snapshot.LifecycleControlStatus)
	}
}

func TestGetStatusBySessionId_ReflectsPausedLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	server := newLiveSessionStatusTestServer(t, harness.svc)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/factory-sessions/"+defaultFactorySessionID+"/status", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want 200", resp.StatusCode)
	}

	var payload factoryapi.StatusResponse
	if err := decodeJSONResponse(resp, &payload); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if payload.LifecycleControlStatus == nil {
		t.Fatal("lifecycleControlStatus = nil, want PAUSED")
	}
	if *payload.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", *payload.LifecycleControlStatus)
	}
}

func TestGetFactorySession_MissingLiveSessionReturnsNotFound(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	_, err := harness.svc.GetFactorySession(context.Background(), "live-session-missing-001")
	if err == nil {
		t.Fatal("GetFactorySession = nil, want not found")
	}
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("error = %v, want ErrFactorySessionNotFound", err)
	}
}

func TestFactoryService_GetFactorySessionSyncPreflight_ValidatesReconnectCursor(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	session := harness.requireSession(t, defaultFactorySessionID)
	eventHistory := liveSessionHandle(session).Bundle.EventHistory
	recorded := generatedFactoryEventsForTest(t, eventHistory.CanonicalEvents())
	if len(recorded) == 0 {
		t.Fatal("event history = empty, want initial structure event")
	}

	valid, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, interfaces.FactorySessionSyncPreflightOptions{
		Reconnect: &interfaces.FactoryEventReconnectCursor{
			AfterEventID: recorded[0].Id,
		},
	})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(valid): %v", err)
	}
	assertSyncPreflightReasonCode(t, valid, factoryapi.Ok, "valid")
	assertSyncPreflightCheckpointReusable(t, valid, true, "valid")
	assertSyncPreflightCursorState(t, valid, true, true, "valid")
	assertSyncPreflightDefaultSessionIdentity(t, harness.svc, session, valid)

	stale, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, interfaces.FactorySessionSyncPreflightOptions{
		Reconnect: &interfaces.FactoryEventReconnectCursor{
			AfterEventID: "factory-event/missing-preflight-cursor",
		},
	})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(stale): %v", err)
	}
	assertSyncPreflightReasonCode(t, stale, factoryapi.CursorStale, "stale")
	assertSyncPreflightCheckpointReusable(t, stale, false, "stale")
	assertSyncPreflightCursorState(t, stale, true, false, "stale")
}

func TestFactoryService_GetFactorySessionSyncPreflight_MissingSessionReturnsTypedOutcome(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	response, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), "live-session-missing-001", interfaces.FactorySessionSyncPreflightOptions{})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(missing): %v", err)
	}
	if response.ReasonCode != factoryapi.SessionNotFound {
		t.Fatalf("reasonCode = %q, want %q", response.ReasonCode, factoryapi.SessionNotFound)
	}
	if response.CheckpointReusable {
		t.Fatal("checkpointReusable = true, want false")
	}
	if response.BackendScopeId != nil || response.FactorySessionId != nil || response.StreamGenerationId != nil {
		t.Fatalf("missing-session identity fields = %#v, want nil", response)
	}
}

func TestFactoryService_GetFactorySessionSyncPreflight_DefaultAliasRemapReturnsTypedOutcome(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, betaSessionID, "beta runtime")

	if err := harness.svc.CloseFactorySession(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("CloseFactorySession(default): %v", err)
	}

	response, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, interfaces.FactorySessionSyncPreflightOptions{})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(remap): %v", err)
	}
	betaSession := harness.requireSession(t, betaSessionID)
	assertSyncPreflightDefaultAliasRemapResponse(t, harness.svc, betaSessionID, betaSession, response)
}

func assertSyncPreflightDefaultAliasRemapResponse(
	t *testing.T,
	fs *FactoryService,
	betaSessionID string,
	betaSession *factorysessions.LiveSession,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()
	if response.ReasonCode != factoryapi.LogicalSessionRemap {
		t.Fatalf("reasonCode = %q, want %q", response.ReasonCode, factoryapi.LogicalSessionRemap)
	}
	if response.CheckpointReusable {
		t.Fatal("checkpointReusable = true, want false after remap")
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != betaSessionID {
		t.Fatalf("factorySessionId = %#v, want promoted beta session %q", response.FactorySessionId, betaSessionID)
	}
	wantLogicalSessionKeyID := factorySessionLogicalSessionKeyID(fs, betaSession)
	if response.LogicalSessionKeyId == nil || *response.LogicalSessionKeyId != wantLogicalSessionKeyID {
		t.Fatalf("logicalSessionKeyId = %v, want %q", response.LogicalSessionKeyId, wantLogicalSessionKeyID)
	}
	betaSessionRead, err := fs.GetFactorySession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetFactorySession(beta): %v", err)
	}
	assertSyncPreflightStreamGenerationMatchesSessionRead(t, response, betaSessionRead)
	if response.ReconnectCursor.Provided || response.ReconnectCursor.ValidForStreamGeneration {
		t.Fatalf("reconnect cursor = %#v, want absent and invalid", response.ReconnectCursor)
	}
}

func assertSyncPreflightStreamGenerationMatchesSessionRead(
	t *testing.T,
	response factoryapi.FactorySessionSyncPreflightResponse,
	sessionRead factoryapi.FactorySession,
) {
	t.Helper()
	if sessionRead.Runtime.StreamIdentity == nil || strings.TrimSpace(sessionRead.Runtime.StreamIdentity.StreamGenerationID) == "" {
		t.Fatalf("session streamIdentity = %#v, want non-empty stream generation", sessionRead.Runtime.StreamIdentity)
	}
	if response.StreamGenerationId == nil || *response.StreamGenerationId != sessionRead.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf("streamGenerationId = %#v, want session read %q", response.StreamGenerationId, sessionRead.Runtime.StreamIdentity.StreamGenerationID)
	}
}

func TestFactoryService_GetFactorySessionSyncPreflight_DefaultAliasResolvesToRuntimeUUID(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	wantRuntimeSessionID := factorysessions.CanonicalFactorySessionID(defaultSession)
	if !factorysessions.IsUUIDFactorySessionID(wantRuntimeSessionID) {
		t.Fatalf("default runtime session id = %q, want UUID", wantRuntimeSessionID)
	}

	response, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, interfaces.FactorySessionSyncPreflightOptions{})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(default alias): %v", err)
	}
	assertSyncPreflightReasonCode(t, response, factoryapi.Ok, "default alias")
	if !response.CheckpointReusable {
		t.Fatal("checkpointReusable = false, want true for default alias resolution")
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != wantRuntimeSessionID {
		t.Fatalf("factorySessionId = %#v, want runtime UUID %q", response.FactorySessionId, wantRuntimeSessionID)
	}
	assertSyncPreflightDefaultSessionIdentity(t, harness.svc, defaultSession, response)

	runtimeResponse, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), wantRuntimeSessionID, interfaces.FactorySessionSyncPreflightOptions{})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(runtime uuid): %v", err)
	}
	assertSyncPreflightReasonCode(t, runtimeResponse, factoryapi.Ok, "runtime uuid")
	assertSyncPreflightDefaultSessionIdentity(t, harness.svc, defaultSession, runtimeResponse)
}

func TestFactoryService_GetFactorySessionSyncPreflight_ResolvesCurrentSessionByLogicalSessionKeyID(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	wantLogicalSessionKeyID := factorySessionLogicalSessionKeyID(harness.svc, defaultSession)
	backendScopeID := factorySessionBackendScopeID(harness.svc, nil)

	response, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, interfaces.FactorySessionSyncPreflightOptions{
		BackendScopeID:      &backendScopeID,
		LogicalSessionKeyID: &wantLogicalSessionKeyID,
	})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(resolved): %v", err)
	}
	assertSyncPreflightReasonCode(t, response, factoryapi.Ok, "resolved")
	assertSyncPreflightDefaultSessionIdentity(t, harness.svc, defaultSession, response)
}

func TestFactoryService_GetFactorySessionSyncPreflight_RemapsStaleFactorySessionIDByLogicalSessionKeyID(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, betaSessionID, "beta runtime")
	betaSession := harness.requireSession(t, betaSessionID)
	wantLogicalSessionKeyID := factorySessionLogicalSessionKeyID(harness.svc, betaSession)
	backendScopeID := factorySessionBackendScopeID(harness.svc, nil)
	staleFactorySessionID := "live-session-stale-beta-001"

	response, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), staleFactorySessionID, interfaces.FactorySessionSyncPreflightOptions{
		BackendScopeID:      &backendScopeID,
		LogicalSessionKeyID: &wantLogicalSessionKeyID,
	})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(remap-by-logical-key): %v", err)
	}
	assertSyncPreflightReasonCode(t, response, factoryapi.LogicalSessionRemap, "remap-by-logical-key")
	if response.CheckpointReusable {
		t.Fatal("checkpointReusable = true, want false after logical remap")
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != betaSessionID {
		t.Fatalf("factorySessionId = %#v, want beta session %q", response.FactorySessionId, betaSessionID)
	}
	if response.LogicalSessionKeyId == nil || *response.LogicalSessionKeyId != wantLogicalSessionKeyID {
		t.Fatalf("logicalSessionKeyId = %#v, want %q", response.LogicalSessionKeyId, wantLogicalSessionKeyID)
	}
}

func TestFactoryService_GetFactorySessionSyncPreflight_UnresolvedLogicalTargetReturnsTypedOutcome(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	wantLogicalSessionKeyID := factorySessionLogicalSessionKeyID(harness.svc, defaultSession)
	backendScopeID := factorySessionBackendScopeID(harness.svc, nil)

	if err := harness.svc.CloseFactorySession(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("CloseFactorySession(default): %v", err)
	}

	response, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), "live-session-missing-001", interfaces.FactorySessionSyncPreflightOptions{
		BackendScopeID:      &backendScopeID,
		LogicalSessionKeyID: &wantLogicalSessionKeyID,
	})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(unresolved): %v", err)
	}
	assertSyncPreflightReasonCode(t, response, factoryapi.SessionNotFound, "unresolved")
	if response.CheckpointReusable {
		t.Fatal("checkpointReusable = true, want false")
	}
	if response.BackendScopeId != nil || response.FactorySessionId != nil || response.StreamGenerationId != nil {
		t.Fatalf("unresolved identity fields = %#v, want nil", response)
	}
}

func TestFactoryService_GetFactorySessionSyncPreflight_InvalidLogicalSessionKeyIDReturnsTypedOutcome(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	invalidLogicalSessionKeyID := "not-a-logical-session-key"
	response, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, interfaces.FactorySessionSyncPreflightOptions{
		LogicalSessionKeyID: &invalidLogicalSessionKeyID,
	})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(invalid-key): %v", err)
	}
	assertSyncPreflightReasonCode(t, response, factoryapi.InvalidTargetReference, "invalid-key")
	if response.CheckpointReusable {
		t.Fatal("checkpointReusable = true, want false")
	}
	if response.BackendScopeId != nil || response.FactorySessionId != nil || response.NormalizedTarget != nil {
		t.Fatalf("invalid-target identity fields = %#v, want nil", response)
	}
}

func TestFactoryService_GetFactorySessionSyncPreflight_WrongBackendScopeReturnsTypedOutcome(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	wantLogicalSessionKeyID := factorySessionLogicalSessionKeyID(harness.svc, defaultSession)
	wrongBackendScopeID := "backend-scope-other-001"

	response, err := harness.svc.GetFactorySessionSyncPreflight(context.Background(), defaultFactorySessionID, interfaces.FactorySessionSyncPreflightOptions{
		BackendScopeID:      &wrongBackendScopeID,
		LogicalSessionKeyID: &wantLogicalSessionKeyID,
	})
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight(wrong-scope): %v", err)
	}
	assertSyncPreflightReasonCode(t, response, factoryapi.SessionNotFound, "wrong-scope")
	if response.BackendScopeId != nil || response.FactorySessionId != nil {
		t.Fatalf("wrong-scope identity fields = %#v, want nil", response)
	}
}

func TestFactoryService_GetFactorySessionSyncPreflight_HTTPRemapsStaleFactorySessionIDByLogicalSessionKeyID(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, betaSessionID, "beta runtime")
	betaSession := harness.requireSession(t, betaSessionID)
	wantLogicalSessionKeyID := factorySessionLogicalSessionKeyID(harness.svc, betaSession)
	backendScopeID := factorySessionBackendScopeID(harness.svc, nil)
	staleFactorySessionID := "live-session-stale-beta-http-001"

	server := newLiveSessionStatusTestServer(t, harness.svc)
	defer server.Close()

	requestURL := server.URL + "/factory-sessions/" + staleFactorySessionID + "/sync-preflight" +
		"?backend_scope_id=" + backendScopeID +
		"&logical_session_key_id=" + wantLogicalSessionKeyID
	resp, err := http.Get(requestURL)
	if err != nil {
		t.Fatalf("GET sync-preflight: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET sync-preflight status = %d, want 200", resp.StatusCode)
	}

	var response factoryapi.FactorySessionSyncPreflightResponse
	if err := decodeJSONResponse(resp, &response); err != nil {
		t.Fatalf("decode sync-preflight response: %v", err)
	}
	assertSyncPreflightReasonCode(t, response, factoryapi.LogicalSessionRemap, "http-remap-by-logical-key")
	if response.FactorySessionId == nil || *response.FactorySessionId != betaSessionID {
		t.Fatalf("factorySessionId = %#v, want beta session %q", response.FactorySessionId, betaSessionID)
	}
}

func assertSyncPreflightReasonCode(t *testing.T, response factoryapi.FactorySessionSyncPreflightResponse, want factoryapi.FactorySessionSyncPreflightReasonCode, label string) {
	t.Helper()
	if response.ReasonCode != want {
		t.Fatalf("%s reasonCode = %q, want %q", label, response.ReasonCode, want)
	}
}

func assertSyncPreflightCheckpointReusable(t *testing.T, response factoryapi.FactorySessionSyncPreflightResponse, want bool, label string) {
	t.Helper()
	if response.CheckpointReusable != want {
		t.Fatalf("%s checkpointReusable = %t, want %t", label, response.CheckpointReusable, want)
	}
}

func assertSyncPreflightCursorState(t *testing.T, response factoryapi.FactorySessionSyncPreflightResponse, wantProvided bool, wantValid bool, label string) {
	t.Helper()
	if response.ReconnectCursor.Provided != wantProvided || response.ReconnectCursor.ValidForStreamGeneration != wantValid {
		t.Fatalf("%s reconnect cursor = %#v, want provided=%t valid=%t", label, response.ReconnectCursor, wantProvided, wantValid)
	}
}

func assertSyncPreflightDefaultSessionIdentity(
	t *testing.T,
	fs *FactoryService,
	session *factorysessions.LiveSession,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()
	assertSyncPreflightDefaultIdentityFields(t, fs, session, response)
	sessionRead, err := fs.GetFactorySession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetFactorySession(%q): %v", session.ID, err)
	}
	assertSyncPreflightDefaultSessionReadMatches(t, response, sessionRead)
}

func assertSyncPreflightDefaultIdentityFields(
	t *testing.T,
	fs *FactoryService,
	session *factorysessions.LiveSession,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()
	if response.BackendScopeId == nil || strings.TrimSpace(*response.BackendScopeId) == "" {
		t.Fatalf("backendScopeId = %#v, want non-empty", response.BackendScopeId)
	}
	wantFactorySessionID := factorysessions.CanonicalFactorySessionID(session)
	if response.FactorySessionId == nil || *response.FactorySessionId != wantFactorySessionID {
		t.Fatalf("factorySessionId = %#v, want %q", response.FactorySessionId, wantFactorySessionID)
	}
	if !factorysessions.IsUUIDFactorySessionID(wantFactorySessionID) {
		t.Fatalf("factorySessionId = %q, want UUID runtime identity", wantFactorySessionID)
	}
	wantLogicalSessionKeyID := factorySessionLogicalSessionKeyID(fs, session)
	if response.LogicalSessionKeyId == nil || *response.LogicalSessionKeyId != wantLogicalSessionKeyID {
		t.Fatalf("logicalSessionKeyId = %#v, want %q", response.LogicalSessionKeyId, wantLogicalSessionKeyID)
	}
	wantStreamGenerationID := factorySessionStreamGenerationID(fs, session)
	if response.StreamGenerationId == nil || *response.StreamGenerationId != wantStreamGenerationID {
		t.Fatalf("streamGenerationId = %#v, want %q", response.StreamGenerationId, wantStreamGenerationID)
	}
}

func assertSyncPreflightDefaultSessionReadMatches(
	t *testing.T,
	response factoryapi.FactorySessionSyncPreflightResponse,
	sessionRead factoryapi.FactorySession,
) {
	t.Helper()
	if sessionRead.Runtime.StreamIdentity == nil || strings.TrimSpace(sessionRead.Runtime.StreamIdentity.StreamGenerationID) == "" {
		t.Fatalf("session streamIdentity = %#v, want non-empty stream generation", sessionRead.Runtime.StreamIdentity)
	}
	if *response.StreamGenerationId != sessionRead.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf("streamGenerationId = %#v, want session read %q", response.StreamGenerationId, sessionRead.Runtime.StreamIdentity.StreamGenerationID)
	}
	if sessionRead.Runtime.StreamIdentity.BackendScopeID != *response.BackendScopeId {
		t.Fatalf("session streamIdentity.backendScopeID = %q, want preflight %q", sessionRead.Runtime.StreamIdentity.BackendScopeID, *response.BackendScopeId)
	}
	if response.NormalizedTarget == nil || response.NormalizedTarget.Kind != factoryapi.FactorySessionLogicalTargetKindDefault {
		t.Fatalf("normalizedTarget = %#v, want default logical target", response.NormalizedTarget)
	}
}

func newLiveSessionStatusTestServer(t *testing.T, svc *FactoryService) *httptest.Server {
	t.Helper()
	return httptest.NewServer(api.NewServer(svc, 0, zap.NewNop()).Handler())
}

func decodeJSONResponse(resp *http.Response, target any) error {
	decoder := json.NewDecoder(resp.Body)
	return decoder.Decode(target)
}

func TestFactoryService_SessionResponseStreamOwnedByLiveSessionRuntime(t *testing.T) {
	session := &factorysessions.LiveSession{
		ID:     "session-response-stream",
		Handle: &liveSessionState{},
	}
	svc := &FactoryService{}

	first := svc.sessionResponseStream(session, "dispatch-a")
	second := svc.sessionResponseStream(session, "dispatch-a")
	third := svc.sessionResponseStream(session, "dispatch-b")
	if first == nil || second == nil || third == nil {
		t.Fatal("session response stream = nil, want live session runtime instance")
	}
	if first != second {
		t.Fatal("same dispatch stream instances differ, want one stream per live dispatch")
	}
	if first == third {
		t.Fatal("different dispatches shared one stream, want dispatch-scoped session streams")
	}

	state := liveSessionRuntimeState(session)
	if state == nil || state.responseStreams == nil {
		t.Fatal("live session state stream set = nil, want session-owned stream set")
	}
	if got := state.responseStreams.Count(); got != 2 {
		t.Fatalf("session stream set count = %d, want 2", got)
	}
	if svc.sessionResponseStreams(nil) != nil {
		t.Fatal("nil session stream = non-nil, want nil")
	}
}

func TestFactoryService_InferenceProgressPublisherPublishesOrderedInternalEvents(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-publish"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	factory := newInferenceProgressPublisherFactory(sessions, nil)
	publisher := factory(sessionID)
	if publisher == nil {
		t.Fatal("publisher = nil, want session publisher")
	}

	publisher(workerprovider.ProgressFragment("dispatch-1", nil, "phase=planning"))
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "partial-response"))
	publisher(workerprovider.CompletedFragment("dispatch-1", &workerexecution.ProviderSessionMetadata{
		Provider: "cursor",
		Kind:     "session_id",
		ID:       "cursor-session-1",
	}))
	publisher(workerprovider.FailedFragment("dispatch-2", nil, "normalized provider failure"))

	session := sessions.Get(sessionID)
	svc := &FactoryService{sessions: sessions}
	stream := svc.sessionResponseStream(session, "dispatch-1")
	if stream == nil {
		t.Fatal("dispatch-1 stream = nil, want live session stream")
	}
	events := stream.Events()
	if len(events) != 3 || events[0].Sequence != 1 || events[2].Sequence != 3 {
		t.Fatalf("stream events = %#v, want ascending internal sequences", events)
	}
	if events[2].Kind != responsestream.EventKindStreamCompleted {
		t.Fatalf("completion kind = %q, want %q", events[2].Kind, responsestream.EventKindStreamCompleted)
	}
	if events[2].ProviderSessionRef == nil || events[2].ProviderSessionRef.ID != "cursor-session-1" {
		t.Fatalf("completion provider session = %#v, want cursor-session-1", events[2].ProviderSessionRef)
	}

	failedStream := svc.sessionResponseStream(session, "dispatch-2")
	if failedStream == nil {
		t.Fatal("dispatch-2 stream = nil, want live session stream")
	}
	failedEvents := failedStream.Events()
	if len(failedEvents) != 1 || failedEvents[0].Kind != responsestream.EventKindStreamFailed || failedEvents[0].Payload != "normalized provider failure" {
		t.Fatalf("failure events = %#v, want one terminal failed marker", failedEvents)
	}

	var factoryEventType factoryapi.FactoryEventType
	_ = factoryEventType
	if string(events[0].Kind) == string(factoryapi.FactoryEventTypeInferenceResponse) {
		t.Fatal("internal stream event must not alias canonical inference response events")
	}
}

func TestFactoryService_InferenceProgressPublisher_DoesNotEmitCanonicalFactoryEvents(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})

	session := harness.requireSession(t, defaultFactorySessionID)
	runtimeFactory := liveSessionHandle(session).Bundle.Factory
	before, err := runtimeFactory.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents(before): %v", err)
	}

	publisher := harness.svc.inferenceProgressPublisher(defaultFactorySessionID, nil)
	if publisher == nil {
		t.Fatal("publisher = nil, want inference progress publisher")
	}
	publisher(workerprovider.ResponseFragment("dispatch-private", nil, "internal-response-fragment"))
	publisher(workerprovider.ProgressFragment("dispatch-private", nil, "internal-progress-fragment"))

	after, err := runtimeFactory.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents(after): %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("factory event count changed from %d to %d after internal stream publication", len(before), len(after))
	}
	for i := range before {
		if after[i].Type != before[i].Type {
			t.Fatalf("factory event types changed after internal stream publication: before=%v after=%v", serviceCanonicalFactoryEventTypes(before), serviceCanonicalFactoryEventTypes(after))
		}
	}

	assertSessionEventsDoNotContain(t, session, "internal-response-fragment")
	assertSessionEventsDoNotContain(t, session, "internal-progress-fragment")
	assertSessionEventsDoNotContain(t, session, string(responsestream.EventKindResponseFragment))
	assertSessionEventsDoNotContain(t, session, string(responsestream.EventKindProgressFragment))
}

func serviceCanonicalFactoryEventTypes(events []interfaces.FactoryEvent) []interfaces.FactoryEventType {
	types := make([]interfaces.FactoryEventType, len(events))
	for index, event := range events {
		types[index] = event.Type
	}
	return types
}

func TestFactoryService_InferenceProgressPublisherConcurrentFirstFragmentsShareOneSessionStream(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-concurrent-first"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	constructorEntered := make(chan struct{}, 1)
	releaseConstructor := make(chan struct{})
	var constructorCalls atomic.Int32
	svc := &FactoryService{
		sessions: sessions,
		newSessionResponseStream: func() *factorysessions.SessionResponseStream {
			constructorCalls.Add(1)
			constructorEntered <- struct{}{}
			<-releaseConstructor
			return factorysessions.NewSessionResponseStream()
		},
	}
	publisher := svc.inferenceProgressPublisher(sessionID, nil)
	if publisher == nil {
		t.Fatal("publisher = nil, want session publisher")
	}

	start := make(chan struct{})
	var publishWG sync.WaitGroup
	publishWG.Add(2)
	go func() {
		defer publishWG.Done()
		<-start
		publisher(workerprovider.ResponseFragment("dispatch-1", nil, "stdout-fragment"))
	}()
	go func() {
		defer publishWG.Done()
		<-start
		publisher(workerprovider.ProgressFragment("dispatch-1", nil, "stderr-fragment"))
	}()

	close(start)
	<-constructorEntered
	close(releaseConstructor)
	publishWG.Wait()

	if got := constructorCalls.Load(); got != 1 {
		t.Fatalf("stream constructor calls = %d, want 1", got)
	}

	session := sessions.Get(sessionID)
	stream := svc.sessionResponseStream(session, "dispatch-1")
	if stream == nil {
		t.Fatal("session stream = nil, want live session stream")
	}
	events := stream.Events()
	if len(events) != 2 {
		t.Fatalf("stream events = %#v, want both concurrent fragments retained", events)
	}
	if events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("event sequences = %#v, want ascending retained order", events)
	}

	payloads := map[string]responsestream.EventKind{}
	for _, event := range events {
		payloads[event.Payload] = event.Kind
	}
	if payloads["stdout-fragment"] != responsestream.EventKindResponseFragment {
		t.Fatalf("stdout fragment kind = %q, want %q", payloads["stdout-fragment"], responsestream.EventKindResponseFragment)
	}
	if payloads["stderr-fragment"] != responsestream.EventKindProgressFragment {
		t.Fatalf("stderr fragment kind = %q, want %q", payloads["stderr-fragment"], responsestream.EventKindProgressFragment)
	}
}

func TestFactoryService_InferenceProgressPublisherSeparatesDispatchScopedStreams(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-separate-dispatches"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	publisher := (&FactoryService{sessions: sessions}).inferenceProgressPublisher(sessionID, nil)
	if publisher == nil {
		t.Fatal("publisher = nil, want session publisher")
	}

	publisher(workerprovider.ResponseFragment("dispatch-a", nil, "alpha-1"))
	publisher(workerprovider.ResponseFragment("dispatch-b", nil, "beta-1"))
	publisher(workerprovider.ProgressFragment("dispatch-a", nil, "alpha-2"))

	session := sessions.Get(sessionID)
	svc := &FactoryService{sessions: sessions}
	alpha := svc.sessionResponseStream(session, "dispatch-a")
	beta := svc.sessionResponseStream(session, "dispatch-b")
	if alpha == nil || beta == nil {
		t.Fatal("dispatch stream = nil, want allocated streams")
	}
	if alpha == beta {
		t.Fatal("different dispatches shared one stream")
	}

	alphaEvents := alpha.Events()
	betaEvents := beta.Events()
	if len(alphaEvents) != 2 || len(betaEvents) != 1 {
		t.Fatalf("dispatch events = (%#v, %#v), want isolated per-dispatch event windows", alphaEvents, betaEvents)
	}
	if alphaEvents[0].Sequence != 1 || alphaEvents[1].Sequence != 2 {
		t.Fatalf("alpha sequences = %#v, want per-dispatch monotonic sequence", alphaEvents)
	}
	if betaEvents[0].Sequence != 1 {
		t.Fatalf("beta sequences = %#v, want independent per-dispatch ordering", betaEvents)
	}
}

func TestFactoryService_SessionResponseStreamDispatchIDs(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-response-stream-dispatch-ids"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	svc := &FactoryService{sessions: sessions}
	publisher := svc.inferenceProgressPublisher(sessionID, nil)
	publisher(workerprovider.ResponseFragment("dispatch-a", nil, "alpha"))
	publisher(workerprovider.ProgressFragment("dispatch-b", nil, "beta"))

	got, err := svc.SessionResponseStreamDispatchIDs(sessionID)
	if err != nil {
		t.Fatalf("SessionResponseStreamDispatchIDs: %v", err)
	}
	if len(got) != 2 || got[0] != "dispatch-a" || got[1] != "dispatch-b" {
		t.Fatalf("dispatch IDs = %#v, want [dispatch-a dispatch-b]", got)
	}
}

func TestFactoryService_SubscribeSessionResponseStream_ReadsRetainedAndLiveEvents(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-subscribe"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	svc := &FactoryService{sessions: sessions}
	publisher := svc.inferenceProgressPublisher(sessionID, nil)
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "retained-1"))
	publisher(workerprovider.ProgressFragment("dispatch-1", nil, "retained-2"))

	subscription, err := svc.SubscribeSessionResponseStream(sessionID, "dispatch-1", 0)
	if err != nil {
		t.Fatalf("SubscribeSessionResponseStream: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(initial): %v", err)
	}
	if len(initial.Events) != 2 {
		t.Fatalf("initial event count = %d, want 2", len(initial.Events))
	}

	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "live-3"))
	live, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(live): %v", err)
	}
	if len(live.Events) != 1 || live.Events[0].Payload != "live-3" {
		t.Fatalf("live events = %#v, want one live event", live.Events)
	}
}

func TestFactoryService_InferenceProgressPublisher_SlowSubscriberCompactionEmitsDiagnostics(t *testing.T) {
	const sessionID = "session-progress-backpressure"
	svc, publisher, logPath, unrelated, metricsPath := newSlowSubscriberCompactionTestHarness(t)
	if publisher == nil {
		t.Fatal("publisher = nil, want inference progress publisher")
	}

	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "retained-1"))
	subscription, err := svc.SubscribeSessionResponseStream(sessionID, "dispatch-1", 0)
	if err != nil {
		t.Fatalf("SubscribeSessionResponseStream: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(initial): %v", err)
	}
	if len(initial.Events) != 1 || initial.Events[0].Payload != "retained-1" {
		t.Fatalf("initial events = %#v, want retained seed event", initial.Events)
	}

	publishSlowSubscriberCompactionBurst(t, publisher)

	catchUp, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(catch-up): %v", err)
	}
	assertSlowSubscriberCompactionCatchUp(t, catchUp)
	assertSlowSubscriberCompactionDiagnostics(t, logPath, unrelated, metricsPath)
}

func TestFactoryService_InferenceProgressPublisherWithoutSubscriberDoesNotBlockExecution(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-no-subscriber"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	svc := &FactoryService{sessions: sessions}
	publisher := svc.inferenceProgressPublisher(sessionID, nil)
	if publisher == nil {
		t.Fatal("publisher = nil, want session publisher")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 256; i++ {
			publisher(workerprovider.ProgressFragment("dispatch-1", nil, "phase"))
		}
		publisher(workerprovider.CompletedFragment("dispatch-1", nil))
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out publishing provider progress without any attached consumer")
	}

	stream := svc.sessionResponseStream(sessions.Get(sessionID), "dispatch-1")
	if stream == nil {
		t.Fatal("session stream = nil, want live session stream")
	}
	events := stream.Events()
	if len(events) == 0 {
		t.Fatal("stream events = empty, want retained progress or terminal marker")
	}
	if events[len(events)-1].Kind != responsestream.EventKindStreamCompleted {
		t.Fatalf("last event kind = %q, want terminal completion marker", events[len(events)-1].Kind)
	}
}

func TestFactoryService_InferenceProgressPublisherSlowConsumerFallsBehindWithoutBlockingExecution(t *testing.T) {
	svc, publisher, stream := newSlowConsumerProgressPublisherHarness(t)
	subscription := subscribeToSessionResponseStream(t, stream)
	defer subscription.Detach()
	assertInitialRetainedSeedEvent(t, subscription)

	done := make(chan struct{})
	go func() {
		defer close(done)
		publisher(workerprovider.ProgressFragment("dispatch-1", nil, "one"))
		publisher(workerprovider.ResponseFragment("dispatch-1", nil, "two"))
		publisher(workerprovider.ProgressFragment("dispatch-1", nil, "three"))
		publisher(workerprovider.CompletedFragment("dispatch-1", nil))
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out publishing provider progress while consumer lagged behind")
	}

	assertSlowConsumerCatchupRetainsCompletedTail(t, subscription)
	_ = svc
}

func newSlowConsumerProgressPublisherHarness(t *testing.T) (*FactoryService, workerprovider.InferenceProgressPublisher, *factorysessions.SessionResponseStream) {
	t.Helper()

	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-slow-consumer"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{},
		false,
		"factory",
	), true)

	svc := &FactoryService{
		sessions: sessions,
		newSessionResponseStream: func() *factorysessions.SessionResponseStream {
			return responsestream.NewSessionResponseStreamWithClock(platformclock.Real{}, responsestream.RetentionLimits{MaxEvents: 2})
		},
	}
	publisher := svc.inferenceProgressPublisher(sessionID, nil)
	if publisher == nil {
		t.Fatal("publisher = nil, want session publisher")
	}
	publisher(workerprovider.ProgressFragment("dispatch-1", nil, "seed"))

	stream := svc.sessionResponseStream(sessions.Get(sessionID), "dispatch-1")
	if stream == nil {
		t.Fatal("session stream = nil, want live session stream")
	}
	return svc, publisher, stream
}

func subscribeToSessionResponseStream(t *testing.T, stream *factorysessions.SessionResponseStream) *factorysessions.SessionResponseStreamSubscription {
	t.Helper()

	subscription, err := stream.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	return subscription
}

func assertInitialRetainedSeedEvent(t *testing.T, subscription *factorysessions.SessionResponseStreamSubscription) {
	t.Helper()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(initial): %v", err)
	}
	if len(initial.Events) != 1 || initial.Events[0].Payload != "seed" {
		t.Fatalf("initial events = %#v, want retained seed event", initial.Events)
	}
}

func assertSlowConsumerCatchupRetainsCompletedTail(
	t *testing.T,
	subscription *factorysessions.SessionResponseStreamSubscription,
) {
	t.Helper()

	read, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(catch-up): %v", err)
	}
	if !read.BehindRetainedWindow {
		t.Fatal("behind retained window = false, want slow consumer compaction signal")
	}
	if read.Compaction == nil || read.Compaction.Reason != responsestream.CompactionReasonTruncated {
		t.Fatalf("compaction = %#v, want truncated retained-window summary", read.Compaction)
	}
	if len(read.Events) == 0 {
		t.Fatal("catch-up events = empty, want retained tail after truncation")
	}
	if read.Events[len(read.Events)-1].Kind != responsestream.EventKindCompactionSignal {
		t.Fatalf("last retained event kind = %q, want retained-window compaction signal", read.Events[len(read.Events)-1].Kind)
	}
	for _, event := range read.Events {
		if event.Kind == responsestream.EventKindStreamCompleted {
			return
		}
	}
	t.Fatalf("retained events = %#v, want terminal completion marker before compaction signal", read.Events)
}

func newSlowSubscriberCompactionTestHarness(t *testing.T) (*FactoryService, workerprovider.InferenceProgressPublisher, string, *observer.ObservedLogs, string) {
	t.Helper()
	const sessionID = "session-progress-backpressure"

	metricsSink, err := platformmetrics.BuildRuntimeMetricsSink(
		"session-progress-backpressure",
		"runtime-progress-backpressure",
		"/factory",
		"/factory",
		t.TempDir(),
		platformmetrics.RuntimeMetricsConfig{},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	t.Cleanup(func() {
		_ = metricsSink.Close()
	})

	unrelatedCore, unrelated := observer.New(zap.WarnLevel)
	undoGlobal := zap.ReplaceGlobals(zap.New(unrelatedCore))
	t.Cleanup(undoGlobal)

	logSink, err := logging.BuildRuntimeLogger(zap.NewNop(), "runtime-progress-backpressure", t.TempDir(), logging.RuntimeLogConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeLogger: %v", err)
	}
	t.Cleanup(func() {
		_ = logSink.Close()
	})
	logger := runtimebuild.NewSessionLogger(logSink.Logger(), sessionID, "/factory", "/factory")
	sessions := factorysessions.NewRegistry()
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
			Logger:      logger,
			MetricsSink: metricsSink,
		}}},
		false,
		"factory",
	), true)

	svc := &FactoryService{
		sessions: sessions,
		newSessionResponseStream: func() *factorysessions.SessionResponseStream {
			return responsestream.NewSessionResponseStreamWithClock(
				platformclock.Real{},
				responsestream.RetentionLimits{MaxEvents: 2},
			)
		},
	}
	publisher := svc.inferenceProgressPublisher(sessionID, logger)
	return svc, publisher, logSink.Path(), unrelated, metricsSink.Path()
}

func publishSlowSubscriberCompactionBurst(t *testing.T, publisher workerprovider.InferenceProgressPublisher) {
	t.Helper()

	publishDone := make(chan struct{})
	go func() {
		for i := 0; i < 64; i++ {
			payload := "chunk-" + strconv.Itoa(i)
			if i%2 == 0 {
				publisher(workerprovider.ProgressFragment("dispatch-1", nil, payload))
				continue
			}
			publisher(workerprovider.ResponseFragment("dispatch-1", nil, payload))
		}
		close(publishDone)
	}()

	select {
	case <-publishDone:
	case <-time.After(time.Second):
		t.Fatal("publishing stalled behind a slow subscriber")
	}
}

func assertSlowSubscriberCompactionCatchUp(t *testing.T, catchUp factorysessions.SessionResponseStreamReadResult) {
	t.Helper()

	if !catchUp.BehindRetainedWindow {
		t.Fatalf("catch-up result = %#v, want retained-window gap signal", catchUp)
	}
	if catchUp.Compaction == nil || catchUp.Compaction.Reason != responsestream.CompactionReasonTruncated {
		t.Fatalf("catch-up compaction = %#v, want truncation summary", catchUp.Compaction)
	}
	for _, event := range catchUp.Events {
		if event.Kind == responsestream.EventKindCompactionSignal {
			return
		}
	}
	t.Fatalf("catch-up events = %#v, want retained compaction signal", catchUp.Events)
}

func assertSlowSubscriberCompactionDiagnostics(t *testing.T, logPath string, unrelated *observer.ObservedLogs, metricsPath string) {
	t.Helper()

	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricSessionResponseStreamCompacted, 1) &&
			metricRecordString(record, "dispatch_id") == "dispatch-1" &&
			metricRecordString(record, "reason") == string(responsestream.CompactionReasonTruncated)
	}, "response stream compaction")

	record := requireRuntimeLogMessage(t, logPath, "session response stream compacted internal provider progress")
	if record["runtime_instance_id"] != "runtime-progress-backpressure" ||
		record["session_id"] != "session-progress-backpressure" ||
		record["dispatch_id"] != "dispatch-1" ||
		record["compaction_reason"] != string(responsestream.CompactionReasonTruncated) ||
		record["dropped_sequence_count"] == float64(0) ||
		record["first_retained_sequence"] == nil ||
		record["last_dropped_sequence"] == nil {
		t.Fatalf("compaction warning fields = %#v, want runtime, session, dispatch, reason, and sequence correlation", record)
	}
	if unrelated.FilterMessage("session response stream compacted internal provider progress").Len() != 0 {
		t.Fatalf("unrelated global sink received compaction warning: %#v", unrelated.All())
	}
}

func requireRuntimeLogMessage(t *testing.T, path, message string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", path, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode runtime log record: %v", err)
		}
		if record["msg"] == message {
			return record
		}
	}
	t.Fatalf("runtime log message %q not found in %s", message, path)
	return nil
}

func TestFactoryService_DispatchCompletionObserverClosesDispatchSubscribers(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-complete"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	svc := &FactoryService{sessions: sessions}
	subscription, err := svc.SubscribeSessionResponseStream(sessionID, "dispatch-1", 0)
	if err != nil {
		t.Fatalf("SubscribeSessionResponseStream: %v", err)
	}

	observerFactory := newSessionDispatchCompletionObserverFactory(sessions)
	if observerFactory == nil {
		t.Fatal("observer factory = nil, want dispatch completion observer")
	}
	observerFactory(sessionID)("dispatch-1")

	if _, err := subscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next after dispatch completion error = %v, want ErrSubscriptionClosed", err)
	}
	session := sessions.Get(sessionID)
	if got := svc.sessionResponseStreams(session).SubscriberCount("dispatch-1"); got != 0 {
		t.Fatalf("subscriber count after dispatch completion = %d, want 0", got)
	}
}

func TestFactoryService_SubscribeSessionResponseStreamAfterDispatchCompletionReadsRetainedEvents(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-late-subscribe"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	svc := &FactoryService{sessions: sessions}
	publisher := svc.inferenceProgressPublisher(sessionID, nil)
	if publisher == nil {
		t.Fatal("publisher = nil, want session publisher")
	}
	publisher(workerprovider.InferenceProgressFragment{
		DispatchID: "dispatch-1",
		Kind:       workerprovider.ProgressFragmentKind,
		Type:       workerprovider.NormalizedEventTypeProgress,
		Payload:    "planning",
	})

	observerFactory := newSessionDispatchCompletionObserverFactory(sessions)
	if observerFactory == nil {
		t.Fatal("observer factory = nil, want dispatch completion observer")
	}
	observerFactory(sessionID)("dispatch-1")

	dispatchIDs, err := svc.SessionResponseStreamDispatchIDs(sessionID)
	if err != nil {
		t.Fatalf("SessionResponseStreamDispatchIDs: %v", err)
	}
	if len(dispatchIDs) != 1 || dispatchIDs[0] != "dispatch-1" {
		t.Fatalf("dispatch ids after completion = %#v, want retained dispatch-1", dispatchIDs)
	}

	subscription, err := svc.SubscribeSessionResponseStream(sessionID, "dispatch-1", 0)
	if err != nil {
		t.Fatalf("SubscribeSessionResponseStream after dispatch completion: %v", err)
	}
	result, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next after late subscribe: %v", err)
	}
	if len(result.Events) != 1 || result.Events[0].Payload != "planning" {
		t.Fatalf("late subscribe events = %#v, want retained planning progress", result.Events)
	}
	if _, err := subscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next after drained retained window error = %v, want ErrSubscriptionClosed", err)
	}
}

func TestFactoryService_StopFactorySession_ClosesSessionResponseStreamSubscribers(t *testing.T) {
	sessionID := "session-progress-stop"
	svc := &FactoryService{sessions: factorysessions.NewRegistry()}
	runDone := make(chan struct{})
	close(runDone)
	handle := &liveRuntimeHandle{RunDone: runDone, Bundle: &factoryRuntimeBundle{}}
	svc.sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: handle, responseStreams: factorysessions.NewSessionResponseStreamSetWithFactory(factorysessions.NewSessionResponseStream)},
		false,
		"factory",
	), true)

	subscription, err := svc.SubscribeSessionResponseStream(sessionID, "dispatch-1", 0)
	if err != nil {
		t.Fatalf("SubscribeSessionResponseStream: %v", err)
	}

	if err := svc.stopFactorySession(sessionID); err != nil {
		t.Fatalf("stopFactorySession: %v", err)
	}
	if _, err := subscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next after session stop error = %v, want ErrSubscriptionClosed", err)
	}
}

func TestFactoryService_InferenceProgressPublisherPreservesNormalizedCodexMetadata(t *testing.T) {
	sessions := factorysessions.NewRegistry()
	sessionID := "session-progress-codex-normalized"
	sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		"/factory",
		"/factory",
		"/factory",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{handle: &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{}}},
		false,
		"factory",
	), true)

	publisher := (&FactoryService{sessions: sessions}).inferenceProgressPublisher(sessionID, nil)
	if publisher == nil {
		t.Fatal("publisher = nil, want session publisher")
	}

	publisher(workerprovider.InferenceProgressFragment{
		DispatchID:        "dispatch-codex-json-1",
		Kind:              workerprovider.ResponseFragmentKind,
		Type:              workerprovider.NormalizedEventTypeFinalText,
		Payload:           "final response",
		ExternalEventType: "response.completed",
		ProviderSessionRef: &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-codex-1",
		},
		Metadata: map[string]string{
			"runner_id":        "codex",
			"workstation_name": "review",
			"work_id":          "work-codex-json-1",
		},
	})

	stream := (&FactoryService{sessions: sessions}).sessionResponseStream(sessions.Get(sessionID), "dispatch-codex-json-1")
	events := stream.Events()
	if len(events) != 1 {
		t.Fatalf("stream events = %#v, want one normalized event", events)
	}
	event := events[0]
	if event.Kind != responsestream.EventKindResponseFragment || event.Type != responsestream.EventType(workerprovider.NormalizedEventTypeFinalText) {
		t.Fatalf("stored event = %#v, want response FINAL_TEXT", event)
	}
	if event.ExternalEventType != "response.completed" {
		t.Fatalf("external event type = %q, want response.completed", event.ExternalEventType)
	}
	if event.ProviderSessionRef == nil || event.ProviderSessionRef.ID != "sess-codex-1" {
		t.Fatalf("provider session = %#v, want sess-codex-1", event.ProviderSessionRef)
	}
	if got := event.Metadata["runner_id"]; got != "codex" {
		t.Fatalf("metadata runner_id = %q, want codex", got)
	}
	if got := event.Metadata["workstation_name"]; got != "review" {
		t.Fatalf("metadata workstation_name = %q, want review", got)
	}
	if got := event.Metadata["work_id"]; got != "work-codex-json-1" {
		t.Fatalf("metadata work_id = %q, want work-codex-json-1", got)
	}
}

func TestFactoryService_InferenceProgressPublisherUnavailableStreamEmitsDegradedDiagnostics(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	session := harness.svc.sessionByID(defaultFactorySessionID)
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil || liveSessionHandle(session).Bundle.MetricsSink == nil {
		t.Fatal("live session runtime metrics sink is required")
	}
	liveSessionHandle(session).Bundle.Logger = zap.New(core)
	harness.svc.newSessionResponseStream = func() *factorysessions.SessionResponseStream {
		return nil
	}

	publisher := harness.svc.inferenceProgressPublisher(defaultFactorySessionID, zap.NewNop())
	if publisher == nil {
		t.Fatal("publisher = nil, want session publisher")
	}
	publisher(workerprovider.ProgressFragment("dispatch-unavailable", nil, "phase"))

	metricsPath := liveSessionHandle(session).Bundle.MetricsSink.Path()
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricSessionResponseStreamDegraded, 1) &&
			metricRecordString(record, "dispatch_id") == "dispatch-unavailable" &&
			metricRecordString(record, "reason") == "STREAM_UNAVAILABLE"
	}, "degraded internal provider progress publication")

	entry := findObservedLog(t, observed, "internal provider progress publication degraded")
	assertLogField(t, entry, "session_id", defaultFactorySessionID)
	assertLogField(t, entry, "dispatch_id", "dispatch-unavailable")
	assertLogField(t, entry, "reason", "STREAM_UNAVAILABLE")
}

type forwardingSessionInvoker struct {
	ctx       context.Context
	sessionID string
	request   sessioninvocation.InvocationRequest
	result    sessioninvocation.FactoryInvocationResult
	err       error
}

func (s *forwardingSessionInvoker) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request sessioninvocation.InvocationRequest,
) (sessioninvocation.FactoryInvocationResult, error) {
	s.ctx = ctx
	s.sessionID = sessionID
	s.request = request
	return s.result, s.err
}

func TestFactoryService_InvokeFactorySessionForwardsToCanonicalOwner(t *testing.T) {
	requestID := "request-1"
	request := factoryapi.InvocationRequest{RequestId: &requestID, Args: &map[string]any{"input": "hello"}}
	wantResult := sessioninvocation.FactoryInvocationResult{
		RequestID: "result-request", TraceID: "trace-1",
		Status: "COMPLETED",
	}
	wantErr := errors.New("owner failure")
	invoker := &forwardingSessionInvoker{result: wantResult, err: wantErr}
	svc := &FactoryService{sessionInvoker: invoker}
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("forwarding"), "preserved")

	got, err := svc.InvokeFactorySession(ctx, "session-1", request)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want owner error %v", err, wantErr)
	}
	if !reflect.DeepEqual(got, wantResult) {
		t.Fatalf("result = %#v, want %#v", got, wantResult)
	}
	if invoker.ctx != ctx || invoker.sessionID != "session-1" {
		t.Fatalf("forwarded ctx/session = %#v/%q", invoker.ctx, invoker.sessionID)
	}
	if invoker.request.RequestID == nil || *invoker.request.RequestID != requestID || invoker.request.Args == nil || (*invoker.request.Args)["input"] != "hello" {
		t.Fatalf("forwarded request = %#v", invoker.request)
	}
}

func TestFactoryService_InvokeFactorySessionRecordsDurablePetriCompletion(t *testing.T) {
	for _, test := range []struct {
		name       string
		result     sessioninvocation.FactoryInvocationResult
		wantStatus factorysessionexecution.LifecycleStatus
	}{
		{
			name: "completed",
			result: sessioninvocation.FactoryInvocationResult{
				Status: interfaces.InvocationTerminalStatusCompleted,
				PrimaryResult: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText, Text: "durable result",
				}},
			},
			wantStatus: factorysessionexecution.LifecycleStatusSucceeded,
		},
		{
			name: "failed",
			result: sessioninvocation.FactoryInvocationResult{
				Status:    interfaces.InvocationTerminalStatusFailed,
				ErrorCode: "INVOCATION_FAILED", Message: "worker failed",
			},
			wantStatus: factorysessionexecution.LifecycleStatusFailed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			execution := factorysessionexecution.NewJavaScriptRuntimeService(
				factorysessionexecution.JavaScriptRuntimeServiceConfig{
					ProjectRoot: t.TempDir(),
				},
			)
			svc := &FactoryService{
				sessionInvoker:   &forwardingSessionInvoker{result: test.result},
				durableExecution: execution,
			}

			if _, err := svc.InvokeFactorySession(context.Background(), "session-1", factoryapi.InvocationRequest{}); err != nil {
				t.Fatalf("InvokeFactorySession: %v", err)
			}
			read, err := execution.GetSession(context.Background(), "session-1")
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if read.Status != test.wantStatus {
				t.Fatalf("durable status = %q, want %q", read.Status, test.wantStatus)
			}
		})
	}
}

func TestInvokeJavaScriptFactorySession_UsesDurableResultAndTypedArguments(t *testing.T) {
	t.Parallel()
	requestID := "deep-research-invocation-001"
	service := &FactoryService{durableExecution: factorysessionexecution.NewFakeService(factorysessionexecution.WithFakeScenarios(factorysessionexecution.FakeScenario{
		ID:        "deep-research",
		RequestID: requestID,
		Session: factorysessionexecution.SessionReadResult{
			SessionID: "dur-sess-deep-research-001",
			Status:    factorysessionexecution.LifecycleStatusSucceeded,
		},
		Result: factorysessionexecution.ResultReadResult{
			SessionID: "dur-sess-deep-research-001", SessionStatus: factorysessionexecution.LifecycleStatusSucceeded,
			ResultStatus:  factorysessionexecution.ResultStatusFinal,
			PrimaryResult: json.RawMessage(`[{"type":"text","text":"Synthesized research result"}]`),
		},
	}))}
	result, err := service.invokeJavaScriptFactorySession(context.Background(), "~default", deepResearchInvocationConfig(), factoryapi.InvocationRequest{
		RequestId: &requestID,
		Args:      &map[string]any{"topic": "event sourcing", "researchDepth": "3", "maxSubagents": "1"},
	})
	if err != nil {
		t.Fatalf("invoke JavaScript factory session: %v", err)
	}
	if result.Status != interfaces.InvocationTerminalStatusCompleted || result.SessionID != "dur-sess-deep-research-001" {
		t.Fatalf("result = %#v, want completed durable-session result", result)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "Synthesized research result" {
		t.Fatalf("primary result = %#v", result.PrimaryResult)
	}
}

func TestJavaScriptInvocationArgs_CoercesSchemaTypedSignatureArguments(t *testing.T) {
	t.Parallel()
	resolved, err := sessionInvocationInputForTest(deepResearchInvocationConfig())
	if err != nil {
		t.Fatal(err)
	}
	args, err := javascriptInvocationArgs(deepResearchInvocationConfig(), resolved)
	if err != nil {
		t.Fatalf("javascriptInvocationArgs: %v", err)
	}
	if depth, ok := args["researchDepth"].(int); !ok || depth != 3 {
		t.Fatalf("researchDepth = %#v, want int(3)", args["researchDepth"])
	}
	if limit, ok := args["maxSubagents"].(int); !ok || limit != 1 {
		t.Fatalf("maxSubagents = %#v, want int(1)", args["maxSubagents"])
	}
}

func TestCoerceJavaScriptArgument_ValidatesDeclaredScalarTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		value      string
		schemaType string
		want       any
		wantErr    string
	}{
		{name: "number", value: "1.5", schemaType: "number", want: 1.5},
		{name: "boolean", value: "true", schemaType: "boolean", want: true},
		{name: "invalid integer", value: "three", schemaType: "integer", wantErr: "argument \"researchDepth\" must be an integer"},
		{name: "invalid number", value: "one", schemaType: "number", wantErr: "argument \"researchDepth\" must be a number"},
		{name: "invalid boolean", value: "yes", schemaType: "boolean", wantErr: "argument \"researchDepth\" must be a boolean"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceJavaScriptArgument("researchDepth", tt.value, tt.schemaType)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("coerceJavaScriptArgument() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("coerceJavaScriptArgument() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("coerceJavaScriptArgument() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestJavaScriptInvocationResult_TimesOutWithoutDurableResult(t *testing.T) {
	t.Parallel()
	result, err := javaScriptInvocationResult("request-1", factorysessionexecution.SyncStartResult{
		AsyncStartResult: factorysessionexecution.AsyncStartResult{SessionID: "session-1"},
		SyncOutcome:      factorysessionexecution.SyncOutcomeTimedOut,
	})
	if err != nil {
		t.Fatalf("javaScriptInvocationResult() error = %v", err)
	}
	if result.Status != interfaces.InvocationTerminalStatusTimedOut || result.ErrorCode != string(factoryapi.INVOCATIONTIMEDOUT) {
		t.Fatalf("result = %#v, want timed-out invocation diagnostic", result)
	}
}

func TestJavaScriptInvocationResult_ReportsInvalidAndFailedDurableResults(t *testing.T) {
	t.Parallel()
	if _, err := javaScriptInvocationResult("request-invalid", factorysessionexecution.SyncStartResult{
		AsyncStartResult: factorysessionexecution.AsyncStartResult{SessionID: "session-invalid"},
		SyncOutcome:      factorysessionexecution.SyncOutcomeCompleted,
		Result:           json.RawMessage(`{`),
	}); err == nil || !strings.Contains(err.Error(), "decode JavaScript workflow result") {
		t.Fatalf("invalid result error = %v, want decode context", err)
	}

	result, err := javaScriptInvocationResult("request-empty", factorysessionexecution.SyncStartResult{
		AsyncStartResult: factorysessionexecution.AsyncStartResult{SessionID: "session-empty"},
		SyncOutcome:      factorysessionexecution.SyncOutcomeCompleted,
		Result:           json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("empty durable result: %v", err)
	}
	if result.Status != interfaces.InvocationTerminalStatusFailed || result.ErrorCode != "INVOCATION_RESULT_UNAVAILABLE" {
		t.Fatalf("empty durable result = %#v, want unavailable failure", result)
	}

	result, err = javaScriptInvocationResult("request-failed", factorysessionexecution.SyncStartResult{
		AsyncStartResult: factorysessionexecution.AsyncStartResult{SessionID: "session-failed"},
		SyncOutcome:      factorysessionexecution.SyncOutcomeCompleted,
		Result:           json.RawMessage(`{"failureDetail":{"reason":"WORKER_FAILED","message":"worker stopped"}}`),
	})
	if err != nil {
		t.Fatalf("failed durable result: %v", err)
	}
	if result.ErrorCode != "WORKER_FAILED" || result.Message != "worker stopped" {
		t.Fatalf("failed durable result = %#v, want failure detail", result)
	}
}

func TestJavaScriptInvocationArgs_RejectsInvalidShapesAndSchema(t *testing.T) {
	t.Parallel()
	if args, err := javascriptInvocationArgs(nil, nil); err != nil || args != nil {
		t.Fatalf("nil arguments = %#v, %v, want nil, nil", args, err)
	}

	multiple := &workinvocation.NormalizedArguments{Arguments: map[string]workinvocation.NormalizedArgument{
		"topic": {Values: []string{"one", "two"}},
	}}
	if _, err := javascriptInvocationArgs(nil, multiple); err == nil || !strings.Contains(err.Error(), "exactly one value") {
		t.Fatalf("multiple-value error = %v, want cardinality diagnostic", err)
	}

	invalidSchema := deepResearchInvocationConfig()
	invalidSchema.Orchestrator.JavaScript.ArgsSchema = json.RawMessage(`{`)
	single := &workinvocation.NormalizedArguments{Arguments: map[string]workinvocation.NormalizedArgument{
		"topic": {Values: []string{"event sourcing"}},
	}}
	if _, err := javascriptInvocationArgs(invalidSchema, single); err == nil || !strings.Contains(err.Error(), "invalid JavaScript args schema") {
		t.Fatalf("invalid-schema error = %v, want schema diagnostic", err)
	}
}

func TestJavaScriptInvocationSource_RejectsMissingConfigurationAndSource(t *testing.T) {
	t.Parallel()
	service := &FactoryService{}
	if _, err := service.javaScriptInvocationSource("session-1", nil); err == nil || !strings.Contains(err.Error(), "configuration is required") {
		t.Fatalf("nil config error = %v, want configuration diagnostic", err)
	}
	missingSource := &interfaces.FactoryConfig{Orchestrator: &interfaces.FactoryOrchestratorConfig{
		Kind:       interfaces.OrchestratorKindJavaScript,
		JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{},
	}}
	if _, err := service.javaScriptInvocationSource("session-1", missingSource); err == nil || !strings.Contains(err.Error(), "no workflow source") {
		t.Fatalf("missing source error = %v, want source diagnostic", err)
	}
}

func sessionInvocationInputForTest(cfg *interfaces.FactoryConfig) (*workinvocation.NormalizedArguments, error) {
	resolved, err := sessioninvocation.ResolveSessionInvocationInput(cfg, factorysessionmapping.InvocationRequestFromAPI(factoryapi.InvocationRequest{
		Args: &map[string]any{"topic": "event sourcing", "researchDepth": "3", "maxSubagents": "1"},
	}))
	return resolved.NormalizedArguments, err
}

func deepResearchInvocationConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name: "@you/deep-research",
		InvocationSignature: &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{
			{Name: "topic", Required: true, Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "POSITIONAL", Position: 1}}},
			{Name: "researchDepth", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "NAMED"}}},
			{Name: "maxSubagents", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "NAMED"}}},
		}},
		Orchestrator: &interfaces.FactoryOrchestratorConfig{Kind: interfaces.OrchestratorKindJavaScript, JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
			InlineSource: &interfaces.FactoryOrchestratorJavaScriptInlineSource{Inline: `meta({ name: "deep-research", version: 1 }); final("done");`},
			ArgsSchema:   json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string"},"researchDepth":{"type":"integer"},"maxSubagents":{"type":"integer"}}}`),
		}},
	}
}
