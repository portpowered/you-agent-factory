package root_composition_test

import (
	"errors"
	"io/fs"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var errRecordingRecordingsEffect = errors.New("recording Recordings effect invoked during BuildProcess")

// TestRecordingsEffectsRemainInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Recordings without invoking portable-recording
// filesystem ports, canonical submission/dispatch recording, or other
// Recordings-owned external effects before runtime lifecycle starts.
func TestRecordingsEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	recorder := newRecordingsEffectRecorder()
	_ = support.BuildProcess(t, recorder.edges())

	if got := recorder.totalPortableRecordingFilesystem(); got != 0 {
		t.Fatalf(
			"portable-recording filesystem effect calls = %d during BuildProcess, want 0",
			got,
		)
	}
	if got := recorder.totalCanonicalRecording(); got != 0 {
		t.Fatalf(
			"canonical submission/dispatch recording effect calls = %d during BuildProcess, want 0",
			got,
		)
	}
}

type recordingsEffectRecorder struct {
	makeDirectories atomic.Int32
	createTempFile  atomic.Int32
	removePath      atomic.Int32
	renamePath      atomic.Int32
	submission      atomic.Int32
	dispatch        atomic.Int32
}

func newRecordingsEffectRecorder() *recordingsEffectRecorder {
	return &recordingsEffectRecorder{}
}

func (recorder *recordingsEffectRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		RecordingMakeDirectories: recorder.recordMakeDirectories,
		RecordingCreateTempFile:  recorder.recordCreateTempFile,
		RecordingRemovePath:      recorder.recordRemovePath,
		RecordingRenamePath:      recorder.recordRenamePath,
		SubmissionRecorder:       recorder.recordSubmission,
		DispatchRecorder:         recorder.recordDispatch,
	}
}

func (recorder *recordingsEffectRecorder) totalPortableRecordingFilesystem() int32 {
	return recorder.makeDirectories.Load() +
		recorder.createTempFile.Load() +
		recorder.removePath.Load() +
		recorder.renamePath.Load()
}

func (recorder *recordingsEffectRecorder) totalCanonicalRecording() int32 {
	return recorder.submission.Load() + recorder.dispatch.Load()
}

func (recorder *recordingsEffectRecorder) recordMakeDirectories(string, fs.FileMode) error {
	recorder.makeDirectories.Add(1)
	return errRecordingRecordingsEffect
}

func (recorder *recordingsEffectRecorder) recordCreateTempFile(string, string) (recordings.RecordingTemporaryFile, error) {
	recorder.createTempFile.Add(1)
	return recordingRecordingsTempFile{}, errRecordingRecordingsEffect
}

func (recorder *recordingsEffectRecorder) recordRemovePath(string) error {
	recorder.removePath.Add(1)
	return errRecordingRecordingsEffect
}

func (recorder *recordingsEffectRecorder) recordRenamePath(string, string) error {
	recorder.renamePath.Add(1)
	return errRecordingRecordingsEffect
}

func (recorder *recordingsEffectRecorder) recordSubmission(work.FactorySubmissionRecord) {
	recorder.submission.Add(1)
}

func (recorder *recordingsEffectRecorder) recordDispatch(recordings.FactoryDispatchRecord) {
	recorder.dispatch.Add(1)
}

type recordingRecordingsTempFile struct{}

func (recordingRecordingsTempFile) Write([]byte) (int, error) {
	return 0, errRecordingRecordingsEffect
}

func (recordingRecordingsTempFile) Name() string { return "" }

func (recordingRecordingsTempFile) Chmod(fs.FileMode) error {
	return errRecordingRecordingsEffect
}

func (recordingRecordingsTempFile) Sync() error { return errRecordingRecordingsEffect }

func (recordingRecordingsTempFile) Close() error { return errRecordingRecordingsEffect }

var _ recordings.RecordingTemporaryFile = recordingRecordingsTempFile{}
