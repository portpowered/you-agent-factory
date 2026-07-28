package root_composition_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const workAdmissionResponseStreamPrimaryResult = "primary result COMPLETE"

// TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle
// proves work-admission and response-stream activate through public Sessions HTTP
// surfaces after runtime lifecycle on a process composed only through
// root.BuildProcess with Sessions effects replaced via published edges.Edges
// fields.
func TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, sessionsWorkAdmissionResponseStreamFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)

	workFilePath := filepath.Join(dir, "initial-work.json")
	support.WriteWorkRequestFile(t, workFilePath, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title": "fun-sessions work admission"}`),
	})

	recorder := newSessionWorkAdmissionResponseStreamRecorder(t)
	edges := recorder.edges()
	support.ConfigureWorkerCommands(
		t,
		&edges,
		support.NewStaticSuccessCommandRunner(workAdmissionResponseStreamPrimaryResult),
		nil,
	)

	workAdmissionBefore := recorder.totalWorkAdmission()
	responseStreamBefore := recorder.totalResponseStream()

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--work", workFilePath},
		Edges:                     edges,
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	waitForDefaultSessionWorkCount(t, baseURL, 1, 10*time.Second)

	invocation := postSessionsInvocation(
		t,
		baseURL,
		sessionsTextInvocationRequest(t, "prove work-admission and response-stream activation"),
	)
	assertSessionsInvocationPrimaryResultText(t, invocation, workAdmissionResponseStreamPrimaryResult)

	responseEvents := support.GetFactoryResponseEventsAt(
		t,
		baseURL,
		factorysessions.DefaultSessionID,
	)
	if len(responseEvents) == 0 {
		t.Fatalf("response events = 0, want observable response-stream records after invocation")
	}

	if got := recorder.totalWorkAdmission() - workAdmissionBefore; got <= 0 {
		t.Fatalf("work-admission effect calls after public session operations = %d, want > 0 via edges", got)
	}
	if got := recorder.totalResponseStream() - responseStreamBefore; got <= 0 {
		t.Fatalf("response-stream effect calls after public session operations = %d, want > 0 via edges", got)
	}
}

func sessionsWorkAdmissionResponseStreamFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
			"handlingBehavior": []string{"DEFAULT"},
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

func waitForDefaultSessionWorkCount(
	t *testing.T,
	baseURL string,
	wantCount int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		if len(listed.Results) == wantCount {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf("default session work count = %d, want %d within %s", len(listed.Results), wantCount, timeout)
}

func sessionsTextInvocationRequest(t *testing.T, text string) factoryapi.InvocationRequest {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}
}

func postSessionsInvocation(
	t *testing.T,
	baseURL string,
	request factoryapi.InvocationRequest,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	endpoint := baseURL + "/factory-sessions/" + factorysessions.DefaultSessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, payload)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func assertSessionsInvocationPrimaryResultText(
	t *testing.T,
	response factoryapi.InvocationResponse,
	wantText string,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	if part.Text != wantText {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, wantText)
	}
}

type sessionWorkAdmissionResponseStreamRecorder struct {
	home               string
	local              platformfilesystem.Local
	workingDirectory   atomic.Int32
	executionGetwd     atomic.Int32
	executionStat      atomic.Int32
	directoryStat      atomic.Int32
	directoryReadDir   atomic.Int32
	resolveHome        atomic.Int32
	resolveSymlinks    atomic.Int32
	sessionID          atomic.Int32
	runtimeID          atomic.Int32
	responseEventID    atomic.Int32
	cursorMkdirAll     atomic.Int32
	cursorReadFile     atomic.Int32
	cursorRemove       atomic.Int32
	cursorRename       atomic.Int32
	cursorTempFile     atomic.Int32
	runtimeMkdirAll    atomic.Int32
	runtimeReadFile    atomic.Int32
	runtimeWriteFile   atomic.Int32
	contractFixture    atomic.Int32
	replayRecording    atomic.Int32
	invocationInput    atomic.Int32
	initialWork        atomic.Int32
	invocationMetric   atomic.Int32
	runtimeHost        atomic.Int32
}

func newSessionWorkAdmissionResponseStreamRecorder(t *testing.T) *sessionWorkAdmissionResponseStreamRecorder {
	t.Helper()
	return &sessionWorkAdmissionResponseStreamRecorder{home: t.TempDir()}
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		FactorySessionsWorkingDirectory:            &sessionWorkAdmissionWorkingDirectory{recorder: recorder},
		FactorySessionExecutionOpeningFileSystem:   &sessionWorkAdmissionExecutionOpening{recorder: recorder},
		FactorySessionDirectoryInspection:          &sessionWorkAdmissionDirectoryInspection{recorder: recorder},
		FactorySessionResolveHomeDirectory:         recorder.resolveHomeDirectory,
		FactorySessionResolveLogicalTargetSymlinks: recorder.resolveLogicalTargetSymlinks,
		FactorySessionIDGenerator:                  recorder.generateSessionID,
		FactorySessionRuntimeInstanceIDGenerator:   recorder.generateRuntimeID,
		FactorySessionResponseEventIDGenerator:     recorder.generateResponseEventID,
		FactorySessionCursorPersistenceFileSystem:  &sessionWorkAdmissionCursorPersistence{recorder: recorder},
		FactorySessionCursorCreateTemporaryFile:    recorder.createCursorTempFile,
		FactorySessionRuntimePersistenceFileSystem: &sessionWorkAdmissionRuntimePersistence{recorder: recorder},
		FactorySessionContractFixtureReader:        recorder.readContractFixture,
		FactorySessionReplayRecordingReader:        recorder.readReplayRecording,
		FactorySessionInvocationInputReader:        recorder.readInvocationInput,
		FactorySessionInitialWorkReader:            recorder.readInitialWork,
		InvocationMetricsRecorder:                recorder,
		RuntimeHostObserver:                        recorder.observeRuntimeHost,
	}
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) totalWorkAdmission() int32 {
	return recorder.invocationInput.Load() + recorder.initialWork.Load()
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) totalResponseStream() int32 {
	return recorder.responseEventID.Load() + recorder.invocationMetric.Load()
}

type sessionWorkAdmissionWorkingDirectory struct {
	recorder *sessionWorkAdmissionResponseStreamRecorder
}

func (adapter *sessionWorkAdmissionWorkingDirectory) Getwd() (string, error) {
	adapter.recorder.workingDirectory.Add(1)
	return adapter.recorder.local.Getwd()
}

type sessionWorkAdmissionExecutionOpening struct {
	recorder *sessionWorkAdmissionResponseStreamRecorder
}

func (adapter *sessionWorkAdmissionExecutionOpening) Getwd() (string, error) {
	adapter.recorder.executionGetwd.Add(1)
	return adapter.recorder.local.Getwd()
}

func (adapter *sessionWorkAdmissionExecutionOpening) Stat(path string) (fs.FileInfo, error) {
	adapter.recorder.executionStat.Add(1)
	return adapter.recorder.local.Stat(path)
}

type sessionWorkAdmissionDirectoryInspection struct {
	recorder *sessionWorkAdmissionResponseStreamRecorder
}

func (adapter *sessionWorkAdmissionDirectoryInspection) Stat(path string) (fs.FileInfo, error) {
	adapter.recorder.directoryStat.Add(1)
	return adapter.recorder.local.Stat(path)
}

func (adapter *sessionWorkAdmissionDirectoryInspection) ReadDir(path string) ([]fs.DirEntry, error) {
	adapter.recorder.directoryReadDir.Add(1)
	return adapter.recorder.local.ReadDir(path)
}

type sessionWorkAdmissionCursorPersistence struct {
	recorder *sessionWorkAdmissionResponseStreamRecorder
}

func (adapter *sessionWorkAdmissionCursorPersistence) MkdirAll(path string, mode fs.FileMode) error {
	adapter.recorder.cursorMkdirAll.Add(1)
	return adapter.recorder.local.MkdirAll(path, mode)
}

func (adapter *sessionWorkAdmissionCursorPersistence) ReadFile(path string) ([]byte, error) {
	adapter.recorder.cursorReadFile.Add(1)
	return adapter.recorder.local.ReadFile(path)
}

func (adapter *sessionWorkAdmissionCursorPersistence) Remove(path string) error {
	adapter.recorder.cursorRemove.Add(1)
	return adapter.recorder.local.Remove(path)
}

func (adapter *sessionWorkAdmissionCursorPersistence) Rename(oldPath, newPath string) error {
	adapter.recorder.cursorRename.Add(1)
	return adapter.recorder.local.Rename(oldPath, newPath)
}

type sessionWorkAdmissionRuntimePersistence struct {
	recorder *sessionWorkAdmissionResponseStreamRecorder
}

func (adapter *sessionWorkAdmissionRuntimePersistence) MkdirAll(path string, mode fs.FileMode) error {
	adapter.recorder.runtimeMkdirAll.Add(1)
	return adapter.recorder.local.MkdirAll(path, mode)
}

func (adapter *sessionWorkAdmissionRuntimePersistence) ReadFile(path string) ([]byte, error) {
	adapter.recorder.runtimeReadFile.Add(1)
	return adapter.recorder.local.ReadFile(path)
}

func (adapter *sessionWorkAdmissionRuntimePersistence) WriteFile(path string, data []byte, mode fs.FileMode) error {
	adapter.recorder.runtimeWriteFile.Add(1)
	return adapter.recorder.local.WriteFile(path, data, mode)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) resolveHomeDirectory() (string, error) {
	recorder.resolveHome.Add(1)
	return recorder.home, nil
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) resolveLogicalTargetSymlinks(path string) (string, error) {
	recorder.resolveSymlinks.Add(1)
	return path, nil
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) generateSessionID() string {
	next := recorder.sessionID.Add(1)
	return fmt.Sprintf("fun-sessions-edge-session-%d", next)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) generateRuntimeID() string {
	next := recorder.runtimeID.Add(1)
	return fmt.Sprintf("fun-sessions-edge-runtime-%d", next)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) generateResponseEventID() string {
	recorder.responseEventID.Add(1)
	return fmt.Sprintf("fun-sessions-edge-response-event-%d", recorder.responseEventID.Load())
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) createCursorTempFile(dir, pattern string) (factorysessions.CursorPersistenceTemporaryFile, error) {
	recorder.cursorTempFile.Add(1)
	return os.CreateTemp(dir, pattern)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) readContractFixture(path string) ([]byte, error) {
	recorder.contractFixture.Add(1)
	return os.ReadFile(path)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) readReplayRecording(path string) ([]byte, error) {
	recorder.replayRecording.Add(1)
	return os.ReadFile(path)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) readInvocationInput(path string) ([]byte, error) {
	recorder.invocationInput.Add(1)
	return os.ReadFile(path)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) readInitialWork(path string) ([]byte, error) {
	recorder.initialWork.Add(1)
	return os.ReadFile(path)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) RecordInvocationMetric(factorysessions.InvocationMetric) {
	recorder.invocationMetric.Add(1)
}

func (recorder *sessionWorkAdmissionResponseStreamRecorder) observeRuntimeHost(factorysessions.RuntimeHostBinding) {
	recorder.runtimeHost.Add(1)
}

var _ platformfilesystem.WorkingDirectory = (*sessionWorkAdmissionWorkingDirectory)(nil)
var _ factorysessions.ExecutionOpeningFileSystem = (*sessionWorkAdmissionExecutionOpening)(nil)
var _ factorysessions.DirectoryInspection = (*sessionWorkAdmissionDirectoryInspection)(nil)
var _ factorysessions.CursorPersistenceFileSystem = (*sessionWorkAdmissionCursorPersistence)(nil)
var _ factorysessions.RuntimePersistenceFileSystem = (*sessionWorkAdmissionRuntimePersistence)(nil)
var _ factorysessions.InvocationMetricsRecorder = (*sessionWorkAdmissionResponseStreamRecorder)(nil)
