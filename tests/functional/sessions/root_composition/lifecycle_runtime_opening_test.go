package root_composition_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle
// proves session lifecycle and runtime-opening activate through public Sessions
// HTTP surfaces after runtime lifecycle on a process constructed only through
// root.BuildProcess with Sessions effects replaced via published edges.Edges
// fields.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	recorder := newSessionActivationRecorder(t)
	dir := support.ScaffoldFactory(t, sessionsLifecycleRuntimeOpeningFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     recorder.edges(),
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	lifecycleBefore := recorder.totalLifecycle()

	defaultSession := support.GetDefaultSession(t, baseURL)
	if !defaultSession.IsDefault || defaultSession.Id == "" {
		t.Fatalf("default live session = %#v, want non-empty default session identity", defaultSession)
	}

	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !sessionSummaryContains(listed.Sessions, defaultSession.Id) {
		t.Fatalf("list sessions = %#v, want default session %q", listed.Sessions, defaultSession.Id)
	}

	pause := postSessionsLifecycleControl(
		t,
		baseURL,
		factorysessions.DefaultSessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	paused := support.GetDefaultSession(t, baseURL)
	if paused.Runtime.LifecycleControlStatus == nil ||
		*paused.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf(
			"paused live session lifecycleControlStatus = %#v, want %q",
			paused.Runtime.LifecycleControlStatus,
			factoryapi.FactorySessionDurableLifecycleStatusPaused,
		)
	}

	resume := postSessionsLifecycleControl(
		t,
		baseURL,
		factorysessions.DefaultSessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}
	running := support.GetDefaultSession(t, baseURL)
	if running.Runtime.LifecycleControlStatus == nil ||
		*running.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf(
			"resumed live session lifecycleControlStatus = %#v, want %q",
			running.Runtime.LifecycleControlStatus,
			factoryapi.FactorySessionDurableLifecycleStatusRunning,
		)
	}

	opened := postSessionsOpen(t, baseURL, dir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened live session = %#v, want canonical session identity", opened)
	}
	if opened.Session.Id == defaultSession.Id {
		t.Fatalf("opened session id = %q, want distinct live identity from default", opened.Session.Id)
	}

	selected := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+opened.Session.Id,
	)
	resolved, err := selected.AsFactorySession()
	if err != nil {
		t.Fatalf("decode opened live session: %v", err)
	}
	if resolved.Id != opened.Session.Id {
		t.Fatalf("selected session id = %q, want %q", resolved.Id, opened.Session.Id)
	}
	if resolved.Runtime.Status == "" {
		t.Fatalf("opened session missing runtime-opening status markers: %#v", resolved)
	}

	afterOpen := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !sessionSummaryContains(afterOpen.Sessions, opened.Session.Id) {
		t.Fatalf("list sessions after open = %#v, want opened session %q", afterOpen.Sessions, opened.Session.Id)
	}

	closeSessionsSession(t, baseURL, opened.Session.Id)
	assertSessionsSessionNotFound(t, baseURL, opened.Session.Id)

	if got := recorder.totalLifecycle() - lifecycleBefore; got <= 0 {
		t.Fatalf("lifecycle effect calls after public session operations = %d, want > 0 via edges", got)
	}
	if got := recorder.runtimeID.Load(); got <= 0 {
		t.Fatalf("runtime instance id generations = %d, want > 0 via edges during runtime opening", got)
	}
}

func sessionsLifecycleRuntimeOpeningFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func sessionSummaryContains(summaries []factoryapi.FactorySessionSummary, sessionID string) bool {
	for _, summary := range summaries {
		if summary.Id == sessionID {
			return true
		}
	}
	return false
}

func postSessionsOpen(t *testing.T, baseURL, folderPath string) factoryapi.OpenFactorySessionResponse {
	t.Helper()
	return postSessionsJSON[factoryapi.OpenFactorySessionResponse](
		t,
		baseURL+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{FolderPath: folderPath},
		"open Factory Session through public HTTP surface",
	)
}

func postSessionsLifecycleControl(
	t *testing.T,
	baseURL string,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	pathSegment := "pause"
	if operation == factoryapi.FactorySessionLifecycleControlKindResume {
		pathSegment = "resume"
	}
	return postSessionsJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/"+pathSegment,
		factoryapi.FactorySessionLifecycleControlRequest{},
		"apply Factory Session lifecycle control through public HTTP surface",
	)
}

func postSessionsJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", failurePrefix, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload)
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return decoded
}

func closeSessionsSession(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	support.TerminateFactorySessionAt(t, baseURL, sessionID)
	support.WaitForSessionStopped(t, baseURL, sessionID, 10*time.Second)
	request, err := http.NewRequest(http.MethodDelete, baseURL+"/factory-sessions/"+sessionID, nil)
	if err != nil {
		t.Fatalf("construct close session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("DELETE Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("DELETE Factory Session %q status = %d, want 204: %s", sessionID, response.StatusCode, payload)
	}
}

func assertSessionsSessionNotFound(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	response, err := http.Get(baseURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET closed Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("GET closed Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, payload)
	}
}

type sessionActivationRecorder struct {
	home             string
	local            platformfilesystem.Local
	workingDirectory atomic.Int32
	executionGetwd   atomic.Int32
	executionStat    atomic.Int32
	directoryStat    atomic.Int32
	directoryReadDir atomic.Int32
	resolveHome      atomic.Int32
	resolveSymlinks  atomic.Int32
	sessionID        atomic.Int32
	runtimeID        atomic.Int32
	cursorMkdirAll   atomic.Int32
	cursorReadFile   atomic.Int32
	cursorRemove     atomic.Int32
	cursorRename     atomic.Int32
	cursorTempFile   atomic.Int32
	runtimeMkdirAll  atomic.Int32
	runtimeReadFile  atomic.Int32
	runtimeWriteFile atomic.Int32
	contractFixture  atomic.Int32
	replayRecording  atomic.Int32
	runtimeHost      atomic.Int32
}

func newSessionActivationRecorder(t *testing.T) *sessionActivationRecorder {
	t.Helper()
	return &sessionActivationRecorder{home: t.TempDir()}
}

func (recorder *sessionActivationRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		FactorySessionsWorkingDirectory:            &sessionActivationWorkingDirectory{recorder: recorder},
		FactorySessionExecutionOpeningFileSystem:   &sessionActivationExecutionOpening{recorder: recorder},
		FactorySessionDirectoryInspection:          &sessionActivationDirectoryInspection{recorder: recorder},
		FactorySessionResolveHomeDirectory:         recorder.resolveHomeDirectory,
		FactorySessionResolveLogicalTargetSymlinks: recorder.resolveLogicalTargetSymlinks,
		FactorySessionIDGenerator:                  recorder.generateSessionID,
		FactorySessionRuntimeInstanceIDGenerator:   recorder.generateRuntimeID,
		FactorySessionCursorPersistenceFileSystem:  &sessionActivationCursorPersistence{recorder: recorder},
		FactorySessionCursorCreateTemporaryFile:    recorder.createCursorTempFile,
		FactorySessionRuntimePersistenceFileSystem: &sessionActivationRuntimePersistence{recorder: recorder},
		FactorySessionContractFixtureReader:        recorder.readContractFixture,
		FactorySessionReplayRecordingReader:        recorder.readReplayRecording,
		RuntimeHostObserver:                        recorder.observeRuntimeHost,
	}
}

func (recorder *sessionActivationRecorder) totalLifecycle() int32 {
	return recorder.resolveHome.Load() +
		recorder.resolveSymlinks.Load() +
		recorder.sessionID.Load() +
		recorder.runtimeID.Load() +
		recorder.directoryStat.Load() +
		recorder.directoryReadDir.Load() +
		recorder.cursorMkdirAll.Load() +
		recorder.cursorReadFile.Load() +
		recorder.cursorRemove.Load() +
		recorder.cursorRename.Load() +
		recorder.cursorTempFile.Load() +
		recorder.runtimeMkdirAll.Load() +
		recorder.runtimeReadFile.Load() +
		recorder.runtimeWriteFile.Load() +
		recorder.runtimeHost.Load()
}

type sessionActivationWorkingDirectory struct {
	recorder *sessionActivationRecorder
}

func (adapter *sessionActivationWorkingDirectory) Getwd() (string, error) {
	adapter.recorder.workingDirectory.Add(1)
	return adapter.recorder.local.Getwd()
}

type sessionActivationExecutionOpening struct {
	recorder *sessionActivationRecorder
}

func (adapter *sessionActivationExecutionOpening) Getwd() (string, error) {
	adapter.recorder.executionGetwd.Add(1)
	return adapter.recorder.local.Getwd()
}

func (adapter *sessionActivationExecutionOpening) Stat(path string) (fs.FileInfo, error) {
	adapter.recorder.executionStat.Add(1)
	return adapter.recorder.local.Stat(path)
}

type sessionActivationDirectoryInspection struct {
	recorder *sessionActivationRecorder
}

func (adapter *sessionActivationDirectoryInspection) Stat(path string) (fs.FileInfo, error) {
	adapter.recorder.directoryStat.Add(1)
	return adapter.recorder.local.Stat(path)
}

func (adapter *sessionActivationDirectoryInspection) ReadDir(path string) ([]fs.DirEntry, error) {
	adapter.recorder.directoryReadDir.Add(1)
	return adapter.recorder.local.ReadDir(path)
}

type sessionActivationCursorPersistence struct {
	recorder *sessionActivationRecorder
}

func (adapter *sessionActivationCursorPersistence) MkdirAll(path string, mode fs.FileMode) error {
	adapter.recorder.cursorMkdirAll.Add(1)
	return adapter.recorder.local.MkdirAll(path, mode)
}

func (adapter *sessionActivationCursorPersistence) ReadFile(path string) ([]byte, error) {
	adapter.recorder.cursorReadFile.Add(1)
	return adapter.recorder.local.ReadFile(path)
}

func (adapter *sessionActivationCursorPersistence) Remove(path string) error {
	adapter.recorder.cursorRemove.Add(1)
	return adapter.recorder.local.Remove(path)
}

func (adapter *sessionActivationCursorPersistence) Rename(oldPath, newPath string) error {
	adapter.recorder.cursorRename.Add(1)
	return adapter.recorder.local.Rename(oldPath, newPath)
}

type sessionActivationRuntimePersistence struct {
	recorder *sessionActivationRecorder
}

func (adapter *sessionActivationRuntimePersistence) MkdirAll(path string, mode fs.FileMode) error {
	adapter.recorder.runtimeMkdirAll.Add(1)
	return adapter.recorder.local.MkdirAll(path, mode)
}

func (adapter *sessionActivationRuntimePersistence) ReadFile(path string) ([]byte, error) {
	adapter.recorder.runtimeReadFile.Add(1)
	return adapter.recorder.local.ReadFile(path)
}

func (adapter *sessionActivationRuntimePersistence) WriteFile(path string, data []byte, mode fs.FileMode) error {
	adapter.recorder.runtimeWriteFile.Add(1)
	return adapter.recorder.local.WriteFile(path, data, mode)
}

func (recorder *sessionActivationRecorder) resolveHomeDirectory() (string, error) {
	recorder.resolveHome.Add(1)
	return recorder.home, nil
}

func (recorder *sessionActivationRecorder) resolveLogicalTargetSymlinks(path string) (string, error) {
	recorder.resolveSymlinks.Add(1)
	return path, nil
}

func (recorder *sessionActivationRecorder) generateSessionID() string {
	next := recorder.sessionID.Add(1)
	return fmt.Sprintf("fun-sessions-edge-session-%d", next)
}

func (recorder *sessionActivationRecorder) generateRuntimeID() string {
	next := recorder.runtimeID.Add(1)
	return fmt.Sprintf("fun-sessions-edge-runtime-%d", next)
}

func (recorder *sessionActivationRecorder) createCursorTempFile(dir, pattern string) (factorysessions.CursorPersistenceTemporaryFile, error) {
	recorder.cursorTempFile.Add(1)
	return os.CreateTemp(dir, pattern)
}

func (recorder *sessionActivationRecorder) readContractFixture(path string) ([]byte, error) {
	recorder.contractFixture.Add(1)
	return os.ReadFile(path)
}

func (recorder *sessionActivationRecorder) readReplayRecording(path string) ([]byte, error) {
	recorder.replayRecording.Add(1)
	return os.ReadFile(path)
}

func (recorder *sessionActivationRecorder) observeRuntimeHost(factorysessions.RuntimeHostBinding) {
	recorder.runtimeHost.Add(1)
}

var _ platformfilesystem.WorkingDirectory = (*sessionActivationWorkingDirectory)(nil)
var _ factorysessions.ExecutionOpeningFileSystem = (*sessionActivationExecutionOpening)(nil)
var _ factorysessions.DirectoryInspection = (*sessionActivationDirectoryInspection)(nil)
var _ factorysessions.CursorPersistenceFileSystem = (*sessionActivationCursorPersistence)(nil)
var _ factorysessions.RuntimePersistenceFileSystem = (*sessionActivationRuntimePersistence)(nil)
