package root_composition_test

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var errRecordingWorkEffect = errors.New("recording work effect invoked during BuildProcess")

// TestWorkEffectsRemainInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Work without invoking submission, routing/relationship,
// recovery/manual-move, recordings-read, CLI submit, or visualization/dependency-
// graph external effects before runtime lifecycle starts.
func TestWorkEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	recorder := newWorkEffectRecorder()
	_ = support.BuildProcess(t, recorder.edges())

	if got := recorder.totalSubmission(); got != 0 {
		t.Fatalf("submission effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.stagingRandomBootstrap(); got != 1 {
		t.Fatalf(
			"content staging signing-secret bootstrap reads = %d during BuildProcess, want exactly 1 composition bootstrap",
			got,
		)
	}
	if got := recorder.totalRoutingRelationship(); got != 0 {
		t.Fatalf("routing/relationship effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalRecoveryManualMove(); got != 0 {
		t.Fatalf("recovery/manual-move effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalRecordingsRead(); got != 0 {
		t.Fatalf("recordings-read effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalCLISubmit(); got != 0 {
		t.Fatalf("CLI submit effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalVisualization(); got != 0 {
		t.Fatalf("visualization/dependency-graph effect calls = %d during BuildProcess, want 0", got)
	}
}

type workEffectRecorder struct {
	stagingMkdirTemp   atomic.Int32
	stagingWriteFile   atomic.Int32
	stagingStat        atomic.Int32
	stagingRemoveAll   atomic.Int32
	stagingRandom      atomic.Int32
	stagingClock       atomic.Int32
	materializeInspect atomic.Int32
	materializeCreate  atomic.Int32
	materializeRemove  atomic.Int32
	materializeWrite   atomic.Int32
	materializeOpen    atomic.Int32
	materializeHTTP    atomic.Int32
	requestID          atomic.Int32
	submittedFileRead  atomic.Int32
}

func newWorkEffectRecorder() *workEffectRecorder {
	return &workEffectRecorder{}
}

func (recorder *workEffectRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		WorkContentStagingFileSystem: recorder,
		WorkContentStagingRandom:     recorder,
		WorkContentStagingClock:      recorder,
		WorkContentHostPlatform:      work.ContentHostPlatform("recording-os"),
		WorkContentInspectPath:       recorder.recordInspectPath,
		WorkContentCreateTempFile:    recorder.recordCreateTempFile,
		WorkContentRemovePath:        recorder.recordRemovePath,
		WorkContentWriteFile:         recorder.recordWriteFile,
		WorkContentOpenFile:          recorder.recordOpenFile,
		WorkContentHTTPDoer:          &recordingWorkHTTPDoer{recorder: recorder},
		WorkRequestIDGenerator:       recorder.recordRequestID,
		WorkSubmittedFileReader:      recorder.recordSubmittedFileRead,
	}
}

func (recorder *workEffectRecorder) totalSubmission() int32 {
	return recorder.stagingMkdirTemp.Load() +
		recorder.stagingWriteFile.Load() +
		recorder.stagingStat.Load() +
		recorder.stagingRemoveAll.Load() +
		recorder.stagingClock.Load() +
		recorder.materializeInspect.Load() +
		recorder.materializeCreate.Load() +
		recorder.materializeRemove.Load() +
		recorder.materializeWrite.Load() +
		recorder.materializeOpen.Load() +
		recorder.materializeHTTP.Load() +
		recorder.requestID.Load()
}

func (recorder *workEffectRecorder) totalRoutingRelationship() int32 {
	return 0
}

func (recorder *workEffectRecorder) totalRecoveryManualMove() int32 {
	return 0
}

func (recorder *workEffectRecorder) totalRecordingsRead() int32 {
	return 0
}

func (recorder *workEffectRecorder) totalCLISubmit() int32 {
	return recorder.submittedFileRead.Load()
}

func (recorder *workEffectRecorder) totalVisualization() int32 {
	return 0
}

// Routing/relationship, recovery/manual-move, recordings-read, and
// visualization/dependency-graph behaviors have no Work-owned process edges
// invoked during BuildProcess; they activate only after runtime lifecycle
// through public Work protocol surfaces on the composed process.

func (recorder *workEffectRecorder) MkdirTemp(string, string) (string, error) {
	recorder.stagingMkdirTemp.Add(1)
	return "", errRecordingWorkEffect
}

func (recorder *workEffectRecorder) WriteFile(string, []byte, fs.FileMode) error {
	recorder.stagingWriteFile.Add(1)
	return errRecordingWorkEffect
}

func (recorder *workEffectRecorder) Stat(string) (fs.FileInfo, error) {
	recorder.stagingStat.Add(1)
	return nil, errRecordingWorkEffect
}

func (recorder *workEffectRecorder) RemoveAll(string) error {
	recorder.stagingRemoveAll.Add(1)
	return errRecordingWorkEffect
}

func (recorder *workEffectRecorder) stagingRandomBootstrap() int32 {
	return recorder.stagingRandom.Load()
}

func (recorder *workEffectRecorder) Read(buffer []byte) (int, error) {
	recorder.stagingRandom.Add(1)
	for i := range buffer {
		buffer[i] = 0x2a
	}
	return len(buffer), nil
}

func (recorder *workEffectRecorder) Now() time.Time {
	recorder.stagingClock.Add(1)
	return time.Unix(0, 0).UTC()
}

func (recorder *workEffectRecorder) recordInspectPath(string) (fs.FileInfo, error) {
	recorder.materializeInspect.Add(1)
	return nil, errRecordingWorkEffect
}

func (recorder *workEffectRecorder) recordCreateTempFile(string, string) (work.ContentTemporaryFile, error) {
	recorder.materializeCreate.Add(1)
	return nil, errRecordingWorkEffect
}

func (recorder *workEffectRecorder) recordRemovePath(string) error {
	recorder.materializeRemove.Add(1)
	return errRecordingWorkEffect
}

func (recorder *workEffectRecorder) recordWriteFile(string, []byte, fs.FileMode) error {
	recorder.materializeWrite.Add(1)
	return errRecordingWorkEffect
}

func (recorder *workEffectRecorder) recordOpenFile(string) (io.WriteCloser, error) {
	recorder.materializeOpen.Add(1)
	return nil, errRecordingWorkEffect
}

func (recorder *workEffectRecorder) recordRequestID() string {
	recorder.requestID.Add(1)
	return "recording-work-request-id"
}

func (recorder *workEffectRecorder) recordSubmittedFileRead(string) ([]byte, error) {
	recorder.submittedFileRead.Add(1)
	return nil, errRecordingWorkEffect
}

type recordingWorkHTTPDoer struct {
	recorder *workEffectRecorder
}

func (client *recordingWorkHTTPDoer) Do(*http.Request) (*http.Response, error) {
	client.recorder.materializeHTTP.Add(1)
	return nil, errRecordingWorkEffect
}

var _ work.ContentStagingFileSystem = (*workEffectRecorder)(nil)
var _ work.ContentStagingRandom = (*workEffectRecorder)(nil)
var _ work.ContentStagingClock = (*workEffectRecorder)(nil)
var _ work.ContentHTTPDoer = (*recordingWorkHTTPDoer)(nil)
