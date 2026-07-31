package root_composition_test

import (
	"errors"
	"io/fs"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var errRecordingOperatorSettingsEffect = errors.New("recording operator settings effect invoked during BuildProcess")

// TestOperatorSettingsEffectsRemainInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Operator Settings without invoking operator-config
// filesystem, temporary-file creation, or backend-scope ID generation external
// effects before runtime lifecycle starts.
func TestOperatorSettingsEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	recorder := newOperatorSettingsEffectRecorder()
	_ = support.BuildProcess(t, recorder.edges())

	if got := recorder.fileSystemCalls(); got != 0 {
		t.Fatalf("operator-config filesystem effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.createTemporaryFileCalls(); got != 0 {
		t.Fatalf("operator-config CreateTemporaryFile calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.idGeneratorCalls(); got != 0 {
		t.Fatalf("operator-config IDGenerator calls = %d during BuildProcess, want 0", got)
	}
}

type operatorSettingsEffectRecorder struct {
	readFile            atomic.Int32
	mkdirAll            atomic.Int32
	remove              atomic.Int32
	chmod               atomic.Int32
	rename              atomic.Int32
	createTemporaryFile atomic.Int32
	idGenerator         atomic.Int32
}

func newOperatorSettingsEffectRecorder() *operatorSettingsEffectRecorder {
	return &operatorSettingsEffectRecorder{}
}

func (recorder *operatorSettingsEffectRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		OperatorSettingsFileSystem:          &operatorSettingsFileSystemRecorder{recorder: recorder},
		OperatorSettingsCreateTemporaryFile: recorder.recordCreateTemporaryFile,
		OperatorSettingsIDGenerator:         recorder.recordIDGenerator,
	}
}

func (recorder *operatorSettingsEffectRecorder) fileSystemCalls() int32 {
	return recorder.readFile.Load() +
		recorder.mkdirAll.Load() +
		recorder.remove.Load() +
		recorder.chmod.Load() +
		recorder.rename.Load()
}

func (recorder *operatorSettingsEffectRecorder) createTemporaryFileCalls() int32 {
	return recorder.createTemporaryFile.Load()
}

func (recorder *operatorSettingsEffectRecorder) idGeneratorCalls() int32 {
	return recorder.idGenerator.Load()
}

type operatorSettingsFileSystemRecorder struct {
	recorder *operatorSettingsEffectRecorder
}

func (adapter *operatorSettingsFileSystemRecorder) ReadFile(string) ([]byte, error) {
	adapter.recorder.readFile.Add(1)
	return nil, errRecordingOperatorSettingsEffect
}

func (adapter *operatorSettingsFileSystemRecorder) MkdirAll(string, fs.FileMode) error {
	adapter.recorder.mkdirAll.Add(1)
	return errRecordingOperatorSettingsEffect
}

func (adapter *operatorSettingsFileSystemRecorder) Remove(string) error {
	adapter.recorder.remove.Add(1)
	return errRecordingOperatorSettingsEffect
}

func (adapter *operatorSettingsFileSystemRecorder) Chmod(string, fs.FileMode) error {
	adapter.recorder.chmod.Add(1)
	return errRecordingOperatorSettingsEffect
}

func (adapter *operatorSettingsFileSystemRecorder) Rename(string, string) error {
	adapter.recorder.rename.Add(1)
	return errRecordingOperatorSettingsEffect
}

func (recorder *operatorSettingsEffectRecorder) recordCreateTemporaryFile(string, string) (operatorsettings.TemporaryFile, error) {
	recorder.createTemporaryFile.Add(1)
	return recordingOperatorSettingsTemporaryFile{}, errRecordingOperatorSettingsEffect
}

func (recorder *operatorSettingsEffectRecorder) recordIDGenerator() string {
	recorder.idGenerator.Add(1)
	return "recording-operator-settings-id"
}

type recordingOperatorSettingsTemporaryFile struct{}

func (recordingOperatorSettingsTemporaryFile) Write([]byte) (int, error) {
	return 0, errRecordingOperatorSettingsEffect
}

func (recordingOperatorSettingsTemporaryFile) Name() string { return "" }

func (recordingOperatorSettingsTemporaryFile) Sync() error { return errRecordingOperatorSettingsEffect }

func (recordingOperatorSettingsTemporaryFile) Close() error {
	return errRecordingOperatorSettingsEffect
}

var _ operatorsettings.FileSystem = (*operatorSettingsFileSystemRecorder)(nil)
var _ operatorsettings.TemporaryFile = recordingOperatorSettingsTemporaryFile{}
