package root_composition_test

import (
	"errors"
	"io"
	"io/fs"
	"sync/atomic"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var errRecordingSessionEffect = errors.New("recording session effect invoked during BuildProcess")

// TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Factory Sessions without invoking lifecycle,
// runtime-opening, work-admission, or response-stream external effects before
// runtime lifecycle starts.
func TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	// This root remains private because the zero-effect assertion is evaluated
	// during this exact BuildProcess call; the package-hosted shared root has
	// already crossed that construction boundary before any test can inspect it.
	recorder := newSessionEffectRecorder()
	process := support.BuildProcess(t, recorder.edges())
	support.CleanupProcess(t, process)

	if got := recorder.totalLifecycle(); got != 0 {
		t.Fatalf("lifecycle effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalRuntimeOpening(); got != 0 {
		t.Fatalf("runtime-opening effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalWorkAdmission(); got != 0 {
		t.Fatalf("work-admission effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalResponseStream(); got != 0 {
		t.Fatalf("response-stream effect calls = %d during BuildProcess, want 0", got)
	}
}

type sessionEffectRecorder struct {
	workingDirectory atomic.Int32
	executionGetwd   atomic.Int32
	executionStat    atomic.Int32
	directoryStat    atomic.Int32
	directoryReadDir atomic.Int32
	resolveHome      atomic.Int32
	resolveSymlinks  atomic.Int32
	sessionID        atomic.Int32
	runtimeID        atomic.Int32
	responseEventID  atomic.Int32
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
	invocationInput  atomic.Int32
	initialWork      atomic.Int32
	invocationMetric atomic.Int32
	runtimeHost      atomic.Int32
}

func newSessionEffectRecorder() *sessionEffectRecorder {
	return &sessionEffectRecorder{}
}

func (recorder *sessionEffectRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		FactorySessionsWorkingDirectory:            &sessionWorkingDirectoryRecorder{recorder: recorder},
		FactorySessionExecutionOpeningFileSystem:   &sessionExecutionOpeningRecorder{recorder: recorder},
		FactorySessionDirectoryInspection:          &sessionDirectoryInspectionRecorder{recorder: recorder},
		FactorySessionResolveHomeDirectory:         recorder.recordResolveHome,
		FactorySessionResolveLogicalTargetSymlinks: recorder.recordResolveSymlinks,
		FactorySessionIDGenerator:                  recorder.recordSessionID,
		FactorySessionRuntimeInstanceIDGenerator:   recorder.recordRuntimeID,
		FactorySessionResponseEventIDGenerator:     recorder.recordResponseEventID,
		FactorySessionCursorPersistenceFileSystem:  &sessionCursorPersistenceRecorder{recorder: recorder},
		FactorySessionCursorCreateTemporaryFile:    recorder.recordCursorTempFile,
		FactorySessionRuntimePersistenceFileSystem: &sessionRuntimePersistenceRecorder{recorder: recorder},
		FactorySessionContractFixtureReader:        recorder.recordContractFixture,
		FactorySessionInvocationInputReader:        recorder.recordInvocationInput,
		FactorySessionReplayRecordingReader:        recorder.recordReplayRecording,
		FactorySessionInitialWorkReader:            recorder.recordInitialWork,
		InvocationMetricsRecorder:                  recorder,
		RuntimeHostObserver:                        recorder.recordRuntimeHost,
	}
}

func (recorder *sessionEffectRecorder) totalLifecycle() int32 {
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

func (recorder *sessionEffectRecorder) totalRuntimeOpening() int32 {
	return recorder.workingDirectory.Load() +
		recorder.executionGetwd.Load() +
		recorder.executionStat.Load() +
		recorder.contractFixture.Load() +
		recorder.replayRecording.Load()
}

func (recorder *sessionEffectRecorder) totalWorkAdmission() int32 {
	return recorder.invocationInput.Load() + recorder.initialWork.Load()
}

func (recorder *sessionEffectRecorder) totalResponseStream() int32 {
	return recorder.responseEventID.Load() + recorder.invocationMetric.Load()
}

type sessionWorkingDirectoryRecorder struct {
	recorder *sessionEffectRecorder
}

func (adapter *sessionWorkingDirectoryRecorder) Getwd() (string, error) {
	adapter.recorder.workingDirectory.Add(1)
	return "", errRecordingSessionEffect
}

type sessionExecutionOpeningRecorder struct {
	recorder *sessionEffectRecorder
}

func (adapter *sessionExecutionOpeningRecorder) Getwd() (string, error) {
	adapter.recorder.executionGetwd.Add(1)
	return "", errRecordingSessionEffect
}

func (adapter *sessionExecutionOpeningRecorder) Stat(string) (fs.FileInfo, error) {
	adapter.recorder.executionStat.Add(1)
	return nil, errRecordingSessionEffect
}

type sessionDirectoryInspectionRecorder struct {
	recorder *sessionEffectRecorder
}

func (adapter *sessionDirectoryInspectionRecorder) Stat(string) (fs.FileInfo, error) {
	adapter.recorder.directoryStat.Add(1)
	return nil, errRecordingSessionEffect
}

func (adapter *sessionDirectoryInspectionRecorder) ReadDir(string) ([]fs.DirEntry, error) {
	adapter.recorder.directoryReadDir.Add(1)
	return nil, errRecordingSessionEffect
}

type sessionCursorPersistenceRecorder struct {
	recorder *sessionEffectRecorder
}

func (adapter *sessionCursorPersistenceRecorder) MkdirAll(string, fs.FileMode) error {
	adapter.recorder.cursorMkdirAll.Add(1)
	return errRecordingSessionEffect
}

func (adapter *sessionCursorPersistenceRecorder) ReadFile(string) ([]byte, error) {
	adapter.recorder.cursorReadFile.Add(1)
	return nil, errRecordingSessionEffect
}

func (adapter *sessionCursorPersistenceRecorder) Remove(string) error {
	adapter.recorder.cursorRemove.Add(1)
	return errRecordingSessionEffect
}

func (adapter *sessionCursorPersistenceRecorder) Rename(string, string) error {
	adapter.recorder.cursorRename.Add(1)
	return errRecordingSessionEffect
}

type sessionRuntimePersistenceRecorder struct {
	recorder *sessionEffectRecorder
}

func (adapter *sessionRuntimePersistenceRecorder) MkdirAll(string, fs.FileMode) error {
	adapter.recorder.runtimeMkdirAll.Add(1)
	return errRecordingSessionEffect
}

func (adapter *sessionRuntimePersistenceRecorder) ReadFile(string) ([]byte, error) {
	adapter.recorder.runtimeReadFile.Add(1)
	return nil, errRecordingSessionEffect
}

func (adapter *sessionRuntimePersistenceRecorder) WriteFile(string, []byte, fs.FileMode) error {
	adapter.recorder.runtimeWriteFile.Add(1)
	return errRecordingSessionEffect
}

func (recorder *sessionEffectRecorder) recordResolveHome() (string, error) {
	recorder.resolveHome.Add(1)
	return "", errRecordingSessionEffect
}

func (recorder *sessionEffectRecorder) recordResolveSymlinks(string) (string, error) {
	recorder.resolveSymlinks.Add(1)
	return "", errRecordingSessionEffect
}

func (recorder *sessionEffectRecorder) recordSessionID() string {
	recorder.sessionID.Add(1)
	return "session-edge-id"
}

func (recorder *sessionEffectRecorder) recordRuntimeID() string {
	recorder.runtimeID.Add(1)
	return "runtime-edge-id"
}

func (recorder *sessionEffectRecorder) recordResponseEventID() string {
	recorder.responseEventID.Add(1)
	return "response-event-edge-id"
}

func (recorder *sessionEffectRecorder) recordCursorTempFile(string, string) (factorysessions.CursorPersistenceTemporaryFile, error) {
	recorder.cursorTempFile.Add(1)
	return recordingSessionCursorTempFile{}, errRecordingSessionEffect
}

func (recorder *sessionEffectRecorder) recordContractFixture(string) ([]byte, error) {
	recorder.contractFixture.Add(1)
	return nil, errRecordingSessionEffect
}

func (recorder *sessionEffectRecorder) recordInvocationInput(string) ([]byte, error) {
	recorder.invocationInput.Add(1)
	return nil, errRecordingSessionEffect
}

func (recorder *sessionEffectRecorder) recordReplayRecording(string) ([]byte, error) {
	recorder.replayRecording.Add(1)
	return nil, errRecordingSessionEffect
}

func (recorder *sessionEffectRecorder) recordInitialWork(string) ([]byte, error) {
	recorder.initialWork.Add(1)
	return nil, errRecordingSessionEffect
}

func (recorder *sessionEffectRecorder) RecordInvocationMetric(factorysessions.InvocationMetric) {
	recorder.invocationMetric.Add(1)
}

func (recorder *sessionEffectRecorder) recordRuntimeHost(factorysessions.RuntimeHostBinding) {
	recorder.runtimeHost.Add(1)
}

var _ platformfilesystem.WorkingDirectory = (*sessionWorkingDirectoryRecorder)(nil)
var _ factorysessions.ExecutionOpeningFileSystem = (*sessionExecutionOpeningRecorder)(nil)
var _ factorysessions.DirectoryInspection = (*sessionDirectoryInspectionRecorder)(nil)
var _ factorysessions.CursorPersistenceFileSystem = (*sessionCursorPersistenceRecorder)(nil)
var _ factorysessions.RuntimePersistenceFileSystem = (*sessionRuntimePersistenceRecorder)(nil)
var _ factorysessions.InvocationMetricsRecorder = (*sessionEffectRecorder)(nil)

type recordingSessionCursorTempFile struct{}

func (recordingSessionCursorTempFile) Write([]byte) (int, error) { return 0, errRecordingSessionEffect }
func (recordingSessionCursorTempFile) Name() string              { return "" }
func (recordingSessionCursorTempFile) Chmod(fs.FileMode) error   { return errRecordingSessionEffect }
func (recordingSessionCursorTempFile) Sync() error               { return errRecordingSessionEffect }
func (recordingSessionCursorTempFile) Close() error              { return errRecordingSessionEffect }

var _ factorysessions.CursorPersistenceTemporaryFile = recordingSessionCursorTempFile{}
var _ io.Writer = recordingSessionCursorTempFile{}
