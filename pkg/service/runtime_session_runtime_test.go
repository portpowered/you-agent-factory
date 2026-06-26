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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/workers"
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
	if liveSessionHandle(defaultSession).runtime.dir != harness.factoryDirs["alpha"] {
		t.Fatalf("default runtime dir = %q, want %q", liveSessionHandle(defaultSession).runtime.dir, harness.factoryDirs["alpha"])
	}
	if liveSessionHandle(firstBeta).runtime.dir != harness.factoryDirs["beta"] || liveSessionHandle(secondBeta).runtime.dir != harness.factoryDirs["beta"] {
		t.Fatalf("beta runtime dirs = %q and %q, want %q", liveSessionHandle(firstBeta).runtime.dir, liveSessionHandle(secondBeta).runtime.dir, harness.factoryDirs["beta"])
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
	case <-liveSessionHandle(firstBeta).runDone:
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
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                   rootOne,
		ExecutionBaseDir:      rootOne,
		RuntimeMode:           interfaces.RuntimeModeService,
		CommandRunnerOverride: runner,
		Logger:                zap.NewNop(),
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
	if defaultSession.FolderPath != rootOne {
		t.Fatalf("default session folder path = %q, want %q", defaultSession.FolderPath, rootOne)
	}
	if secondSession.FolderPath != rootTwo {
		t.Fatalf("second session folder path = %q, want %q", secondSession.FolderPath, rootTwo)
	}
	if liveSessionHandle(defaultSession).runtime.dir != rootOne {
		t.Fatalf("default runtime dir = %q, want %q", liveSessionHandle(defaultSession).runtime.dir, rootOne)
	}
	if liveSessionHandle(secondSession).runtime.dir != rootTwo {
		t.Fatalf("second runtime dir = %q, want %q", liveSessionHandle(secondSession).runtime.dir, rootTwo)
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
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                   rootOne,
		ExecutionBaseDir:      rootOne,
		RuntimeMode:           interfaces.RuntimeModeService,
		CommandRunnerOverride: runner,
		Logger:                zap.NewNop(),
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

	if liveSessionHandle(defaultSession).runtime.recordPath != recordPath {
		t.Fatalf("default record path = %q, want %q", liveSessionHandle(defaultSession).runtime.recordPath, recordPath)
	}
	if liveSessionHandle(firstBeta).runtime.recordPath == "" || liveSessionHandle(secondBeta).runtime.recordPath == "" {
		t.Fatalf("background record paths must be set, got %q and %q", liveSessionHandle(firstBeta).runtime.recordPath, liveSessionHandle(secondBeta).runtime.recordPath)
	}
	if liveSessionHandle(firstBeta).runtime.recordPath == liveSessionHandle(secondBeta).runtime.recordPath {
		t.Fatalf("background sessions shared record path %q", liveSessionHandle(firstBeta).runtime.recordPath)
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
	r.requests = append(r.requests, workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req)))
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
	requests []interfaces.ProviderInferenceRequest
}

func (p *sessionCapturingProvider) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, interfaces.CloneProviderInferenceRequest(req))
	p.mu.Unlock()
	return interfaces.InferenceResponse{Content: "ok"}, nil
}

func (p *sessionCapturingProvider) waitForRequests(t *testing.T, want int, wait time.Duration) []interfaces.ProviderInferenceRequest {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		if len(p.requests) >= want {
			requests := append([]interfaces.ProviderInferenceRequest(nil), p.requests...)
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
	case <-liveSessionHandle(firstBeta).runDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stopped beta session to exit")
	}

	secondBetaSessionID := harness.openFactorySession(t, "beta")
	secondBeta := harness.requireSession(t, secondBetaSessionID)
	if firstBetaSessionID == secondBetaSessionID {
		t.Fatalf("reopened beta session id = %q, want a new session identity", secondBetaSessionID)
	}
	if liveSessionHandle(firstBeta).runtime.recordPath == liveSessionHandle(secondBeta).runtime.recordPath {
		t.Fatalf("reopened beta sessions shared record path %q", liveSessionHandle(secondBeta).runtime.recordPath)
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
	case <-liveSessionHandle(defaultSession).runDone:
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
	case <-liveSessionHandle(defaultSession).runDone:
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

func TestFactoryService_LegacyRuntimeSurfaceTargetsDefaultSessionAlias(t *testing.T) {
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
	request := requests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
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
	if liveSessionHandle(defaultSession).runtime.dir != harness.factoryDirs["alpha"] {
		t.Fatalf("restarted default runtime dir = %q, want %q", liveSessionHandle(defaultSession).runtime.dir, harness.factoryDirs["alpha"])
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
	stream, err := liveSessionHandle(betaSession).runtime.factory.SubscribeFactoryEvents(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
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

	if liveSessionHandle(betaSession).runtime.localModels == nil {
		t.Fatal("opened session runtime localModels = nil, want model catalog seam")
	}
	if _, err := localmodels.ListModels(liveSessionHandle(betaSession).runtime.runtimeCfg); err != nil {
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
	if liveSessionHandle(session).runtime.dir != harness.factoryDirs["alpha"] {
		t.Fatalf("opened session runtime dir = %q, want %q", liveSessionHandle(session).runtime.dir, harness.factoryDirs["alpha"])
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
	if liveSessionHandle(defaultSession).runtime.dir != harness.rootDir {
		t.Fatalf("default target runtime dir = %q, want %q", liveSessionHandle(defaultSession).runtime.dir, harness.rootDir)
	}
	if liveSessionHandle(betaSessionOne).runtime.dir != harness.factoryDirs["beta"] || liveSessionHandle(betaSessionTwo).runtime.dir != harness.factoryDirs["beta"] {
		t.Fatalf("beta target runtime dirs = %q and %q, want %q", liveSessionHandle(betaSessionOne).runtime.dir, liveSessionHandle(betaSessionTwo).runtime.dir, harness.factoryDirs["beta"])
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
	if liveSessionHandle(session).runtime.dir != harness.factoryDirs["alpha"] {
		t.Fatalf("opened session runtime dir = %q, want %q", liveSessionHandle(session).runtime.dir, harness.factoryDirs["alpha"])
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
		ErrorTargets() []factoryapi.FactoryValidationTarget
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
	if target.Subject.Id != wantField {
		t.Fatalf("validation target subject id = %q, want %q", target.Subject.Id, wantField)
	}
}

func assertFactorySessionConfigLoadFailure(t *testing.T, err error, wantTargetID string) {
	t.Helper()

	reason, field, ok := factorysessions.ValidationReasonFromError(err)
	if !ok || reason != factorysessions.ValidationReasonConfigLoadFailed || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want config_load_failed folderPath", reason, field, ok)
	}

	var targetedErr interface {
		ErrorTargets() []factoryapi.FactoryValidationTarget
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
	if target.Subject.Id != wantTargetID {
		t.Fatalf("config load target subject id = %q, want %q", target.Subject.Id, wantTargetID)
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
	defaultCtx := runtime.WorkflowContext(defaultBundle.factory)
	if defaultCtx == nil || defaultCtx.SessionID != factorysessions.DefaultSessionID {
		t.Fatalf("default workflow context = %#v, want SessionID %q", defaultCtx, factorysessions.DefaultSessionID)
	}

	namedBundle, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, "session-beta")
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime(named): %v", err)
	}
	namedCtx := runtime.WorkflowContext(namedBundle.factory)
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
	if session.Id != defaultFactorySessionID {
		t.Fatalf("session id = %q, want %q", session.Id, defaultFactorySessionID)
	}
	if session.Runtime.OrchestratorKind != factoryapi.PETRI {
		t.Fatalf("orchestrator kind = %q, want PETRI", session.Runtime.OrchestratorKind)
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
	found := false
	for _, summary := range listed.Sessions {
		if summary.Id != defaultFactorySessionID {
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
}

func newFactoryServiceForDurableLifecycleTest(t *testing.T, fixtureName, workflowName string) *FactoryService {
	t.Helper()
	projectRoot := setupDurableLifecycleWorkflowFixture(t, fixtureName, workflowName)
	return &FactoryService{
		cfg: &FactoryServiceConfig{
			Dir: projectRoot,
		},
		factoryRootDir: projectRoot,
	}
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
	})
	harness.svc.logger = zap.New(core)
	defer harness.stop(t)

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
	})
	harness.svc.logger = zap.New(core)
	defer harness.stop(t)

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
	})
	harness.svc.logger = zap.New(core)
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

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control")
	assertLogField(t, entry, "operation", "RESUME")
	assertLogField(t, entry, "outcome", "NO_OP")
}

func TestObserveLiveLifecycleControl_LogsNotFound(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	harness.svc.logger = zap.New(core)
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

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control rejected")
	assertLogField(t, entry, "session_id", "live-session-missing-001")
	assertLogField(t, entry, "outcome", "NOT_FOUND")
}

func TestObserveLiveLifecycleControl_EmitsAcceptedPauseMetric(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	session := harness.svc.sessionByID(defaultFactorySessionID)
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).runtime == nil || liveSessionHandle(session).runtime.metricsSink == nil {
		t.Fatal("live session runtime metrics sink is required")
	}
	metricsPath := liveSessionHandle(session).runtime.metricsSink.Path()

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

func assertLogField(t *testing.T, entry observer.LoggedEntry, key, want string) {
	t.Helper()
	for _, field := range entry.Context {
		if field.Key != key {
			continue
		}
		if field.String == want {
			return
		}
		t.Fatalf("log field %q = %q, want %q", key, field.String, want)
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

	first := svc.sessionResponseStream(session)
	second := svc.sessionResponseStream(session)
	if first == nil || second == nil {
		t.Fatal("session response stream = nil, want live session runtime instance")
	}
	if first != second {
		t.Fatal("session response stream instances differ, want one stream per live session")
	}

	state := liveSessionRuntimeState(session)
	if state == nil || state.responseStream != first {
		t.Fatalf("live session state stream = %#v, want %p", state.responseStream, first)
	}
	if svc.sessionResponseStream(nil) != nil {
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
		&liveSessionState{},
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

	session := sessions.Get(sessionID)
	stream := (&FactoryService{sessions: sessions}).sessionResponseStream(session)
	if stream == nil {
		t.Fatal("session stream = nil, want live session stream")
	}
	events := stream.Events()
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("stream events = %#v, want ascending internal sequences", events)
	}

	var factoryEventType factoryapi.FactoryEventType
	_ = factoryEventType
	if string(events[0].Kind) == string(factoryapi.FactoryEventTypeInferenceResponse) {
		t.Fatal("internal stream event must not alias canonical inference response events")
	}
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
		&liveSessionState{},
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
	stream := svc.sessionResponseStream(session)
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
