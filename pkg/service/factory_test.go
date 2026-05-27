package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"
	"go.uber.org/zap"
)

const servicePortableBundledScriptBody = "Write-Output 'portable script'\n"
const serviceStreamedRecordingTimeout = 5 * time.Second

// minimalFactoryConfig returns a minimal factory.json config for testing.
func minimalFactoryConfig() map[string]any {
	return map[string]any{
		"name": "factory",
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
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func serviceNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return serviceNamedFactoryPayloadWithWorkType(t, project, "task")
}

func serviceNamedFactoryPayloadWithVersion(t *testing.T, project string, version factoryapi.HybridLogicalTimestamp) []byte {
	t.Helper()
	return withServicePayloadVersion(t, serviceNamedFactoryPayload(t, project), version)
}

func serviceNamedFactoryPayloadWithWorkType(t *testing.T, project, workType string) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "worker-a",
			"type": "MODEL_WORKER",
			"body": "You are worker " + project + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": workType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal named factory payload: %v", err)
	}
	return payload
}

func serviceNamedFactoryContract(t *testing.T, name string) factoryapi.Factory {
	t.Helper()
	return serviceNamedFactoryContractWithWorkType(t, name, "task")
}

func serviceNamedFactoryContractWithBundledFiles(t *testing.T, name string) factoryapi.Factory {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "worker-a",
			"type": "MODEL_WORKER",
			"body": "You are worker " + name + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + name + " work.",
		}},
		"supportingFiles": map[string]any{
			"bundledFiles": []map[string]any{
				{
					"type":       "ROOT_HELPER",
					"targetPath": "Makefile",
					"content": map[string]any{
						"encoding": string(factoryapi.Utf8),
						"inline":   "test:\n\tgo test ./...\n",
					},
				},
				{
					"type":       "DOC",
					"targetPath": "factory/docs/README.md",
					"content": map[string]any{
						"encoding": string(factoryapi.Utf8),
						"inline":   "# Portable factory\n",
					},
				},
				{
					"type":       "SCRIPT",
					"targetPath": "factory/scripts/execute-story.ps1",
					"content": map[string]any{
						"encoding": string(factoryapi.Utf8),
						"inline":   servicePortableBundledScriptBody,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal bundled named factory payload: %v", err)
	}

	generated, err := config.GeneratedFactoryFromOpenAPIJSON(payload)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(%s bundled files): %v", name, err)
	}

	generated.Name = factoryapi.FactoryName(name)
	return generated
}

func serviceNamedFactoryContractWithWorkType(t *testing.T, name, workType string) factoryapi.Factory {
	t.Helper()

	generated, err := config.GeneratedFactoryFromOpenAPIJSON([]byte(`{
		"name":"` + name + `",
		"id":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[
			{"name":"init","type":"INITIAL"},
			{"name":"complete","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"You are worker ` + name + `."}],
		"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"Do the ` + name + ` work.","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"complete"}],"onFailure": [{"workType":"` + workType + `","state":"failed"}]}]
		}`))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(%s): %v", name, err)
	}

	generated.Name = factoryapi.FactoryName(name)
	return generated
}

func withServicePayloadVersion(t *testing.T, payload []byte, version factoryapi.HybridLogicalTimestamp) []byte {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal service factory payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  version.Logical,
		"physical": version.Physical.UTC().Format(time.RFC3339Nano),
	}
	updated, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal service factory payload with version: %v", err)
	}
	return updated
}

func submitWorkRequestsToService(ctx context.Context, svc *FactoryService, reqs []interfaces.SubmitRequest) error {
	workRequest := requests.WorkRequestFromSubmitRequests(reqs)
	_, err := svc.SubmitWorkRequest(ctx, workRequest)
	return err
}

func writeWorkRequestFile(t *testing.T, path string, req interfaces.SubmitRequest) {
	t.Helper()
	data, err := json.Marshal(requests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{req}))
	if err != nil {
		t.Fatalf("marshal work request file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write work request file: %v", err)
	}
}

// writeFactoryJSON writes a factory.json into the given directory.
func writeFactoryJSON(t *testing.T, dir string, cfg map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func stopServiceModeRun(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("service-mode run error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode run to stop")
	}
}

type aggregateSnapshotFactory struct {
	engineState              *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	engineStateErr           error
	engineStateSnapshotCalls int
	factoryEvents            []factoryapi.FactoryEvent
	factoryEventsErr         error
	factoryEventsCalls       int
	pauseErr                 error
	submitFunc               func(context.Context, interfaces.WorkRequest) error
	submitCalls              int
	submissions              []interfaces.WorkRequest
	waitToComplete           chan struct{}
}

func (f *aggregateSnapshotFactory) Run(context.Context) error { return nil }
func (f *aggregateSnapshotFactory) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	normalized, err := requests.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{})
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	result := interfaces.WorkRequestSubmitResult{RequestID: request.RequestID, Accepted: true}
	if len(normalized) > 0 {
		result.TraceID = normalized[0].TraceID
	}
	f.submitCalls++
	f.submissions = append(f.submissions, request)
	if f.submitFunc != nil {
		return result, f.submitFunc(ctx, request)
	}
	return result, nil
}
func (f *aggregateSnapshotFactory) SubscribeFactoryEvents(context.Context) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan factoryapi.FactoryEvent)}, nil
}
func (f *aggregateSnapshotFactory) Pause(context.Context) error { return f.pauseErr }
func (f *aggregateSnapshotFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	f.engineStateSnapshotCalls++
	if f.engineStateErr != nil {
		return nil, f.engineStateErr
	}
	return f.engineState, nil
}
func (f *aggregateSnapshotFactory) GetFactoryEvents(context.Context) ([]factoryapi.FactoryEvent, error) {
	f.factoryEventsCalls++
	if f.factoryEventsErr != nil {
		return nil, f.factoryEventsErr
	}
	return append([]factoryapi.FactoryEvent(nil), f.factoryEvents...), nil
}
func (f *aggregateSnapshotFactory) WaitToComplete() <-chan struct{} {
	if f.waitToComplete != nil {
		return f.waitToComplete
	}
	return make(chan struct{})
}

func TestFactoryService_WaitToComplete_ReturnsClosedChannelWithoutRuntime(t *testing.T) {
	svc := &FactoryService{}

	select {
	case <-svc.WaitToComplete():
	default:
		t.Fatal("expected WaitToComplete without runtime to return a closed channel")
	}
}

func TestFactoryService_WaitToComplete_DelegatesToActiveRuntime(t *testing.T) {
	waitCh := make(chan struct{})
	svc := &FactoryService{
		factory: &aggregateSnapshotFactory{
			waitToComplete: waitCh,
		},
	}

	if got := svc.WaitToComplete(); got != waitCh {
		t.Fatalf("WaitToComplete channel = %p, want %p", got, waitCh)
	}
	close(waitCh)
}

func TestFactoryService_Pause_RequiresActiveRuntimeAndWrapsPauseErrors(t *testing.T) {
	svc := &FactoryService{}
	if err := svc.Pause(context.Background()); err == nil || !strings.Contains(err.Error(), "runtime is not available") {
		t.Fatalf("Pause without runtime error = %v, want runtime unavailable", err)
	}

	svc.factory = &aggregateSnapshotFactory{pauseErr: fmt.Errorf("pause failed")}
	if err := svc.Pause(context.Background()); err == nil || !strings.Contains(err.Error(), "pause factory: pause failed") {
		t.Fatalf("Pause wrapped error = %v, want wrapped pause failure", err)
	}

	svc.factory = &aggregateSnapshotFactory{}
	if err := svc.Pause(context.Background()); err != nil {
		t.Fatalf("Pause success error = %v", err)
	}
}

func TestFactoryService_CurrentRuntimeBundleAndDirComparisonHelpers(t *testing.T) {
	if bundle := (*FactoryService)(nil).currentRuntimeBundle(); bundle != nil {
		t.Fatalf("nil service currentRuntimeBundle = %#v, want nil", bundle)
	}

	svc := &FactoryService{}
	if bundle := svc.currentRuntimeBundle(); bundle != nil {
		t.Fatalf("empty service currentRuntimeBundle = %#v, want nil", bundle)
	}

	svc.cfg = &FactoryServiceConfig{Dir: "C:/factory"}
	svc.factory = &aggregateSnapshotFactory{}
	svc.runtimeCfg = &config.LoadedFactoryConfig{}
	bundle := svc.currentRuntimeBundle()
	if bundle == nil {
		t.Fatal("expected populated currentRuntimeBundle")
	}
	if bundle.dir != svc.cfg.Dir || bundle.factory != svc.factory || bundle.runtimeCfg != svc.runtimeCfg {
		t.Fatalf("currentRuntimeBundle = %#v, want service fields copied through", bundle)
	}

	if sameFactoryDir("", svc.cfg.Dir) {
		t.Fatal("sameFactoryDir should reject blank paths")
	}
	if !sameFactoryDir("C:/factory/./named", "C:/factory/named") {
		t.Fatal("sameFactoryDir should normalize equivalent paths")
	}
}

func TestLiveRuntimeHandle_CompletionHelpers(t *testing.T) {
	if !(*liveRuntimeHandle)(nil).completed() {
		t.Fatal("nil liveRuntimeHandle should report completed")
	}
	if err := (*liveRuntimeHandle)(nil).wait(); err != nil {
		t.Fatalf("nil liveRuntimeHandle wait error = %v, want nil", err)
	}

	handle := &liveRuntimeHandle{
		runDone: make(chan struct{}),
	}
	if handle.completed() {
		t.Fatal("open runDone should report incomplete")
	}
	handle.setRunResult(fmt.Errorf("run failed"))
	if !handle.completed() {
		t.Fatal("closed runDone should report completed")
	}
	if err := handle.wait(); err == nil || err.Error() != "run failed" {
		t.Fatalf("wait error = %v, want run failed", err)
	}
}

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
	runtimeLogDir := options.runtimeLogDir
	if runtimeLogDir == "" {
		runtimeLogDir = filepath.Join(t.TempDir(), "runtime-logs")
	}
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
		RuntimeLogDir:     runtimeLogDir,
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
	request := requests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
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

	request := requests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
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
