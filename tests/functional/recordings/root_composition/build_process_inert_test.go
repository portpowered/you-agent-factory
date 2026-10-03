package root_composition_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// Explicit lifecycle bindings use the public Recordings authority from the
// production process. Runtime opening and CLI shutdown have separate witnesses.
func TestFailedRecordingFinalFlushPreservesPeerHistory(t *testing.T) {
	t.Parallel()
	failedPath := filepath.Join(t.TempDir(), "failed.json")
	peerPath := filepath.Join(t.TempDir(), "peer.json")
	writeErr := errors.New("selected scope storage unavailable")
	writeStarted, releaseWrite := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWrite) }) }
	defer release()
	var service recordings.Service
	var providerRuns atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:  functionalReplayCommandRunner{calls: &providerRuns},
		RecordingsRootObserver: func(root recordings.Service) { service = root },
		RecordingWriteFile: func(path string, data []byte) error {
			if path == failedPath {
				close(writeStarted)
				<-releaseWrite
				return writeErr
			}
			return os.WriteFile(path, data, 0o600)
		},
		RecordingReadFile: os.ReadFile,
	})
	if service == nil {
		t.Fatal("BuildProcess did not publish the Recordings authority")
	}
	failed := bindPublicRecording(t, service, "failed", "019a07c0-0000-7000-8000-000000000001", failedPath)
	peer := bindPublicRecording(t, service, "peer", "019a07c0-0000-7000-8000-000000000002", peerPath)
	finishedAt := time.Date(2026, 10, 3, 12, 34, 56, 123456789, time.UTC)
	recordPublicRecordingFact(t, service, failed, "failed-fact", 0, finishedAt.Add(-time.Second))
	first := recordPublicRecordingFact(t, service, peer, "peer-first", 0, finishedAt.Add(-time.Second))
	assertGatedPublicRecordingFailure(t, service, failed.RecordingID, finishedAt, writeStarted, release, writeErr)
	second := recordPublicRecordingFact(t, service, peer, "peer-second", 1, finishedAt)
	finalized, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: peer.RecordingID, FinishedAt: finishedAt,
	})
	if err != nil || finalized.Status.State != recordings.RecordingFinalized ||
		finalized.Status.FlushedThrough == nil || *finalized.Status.FlushedThrough != second.Cursor {
		t.Fatalf("peer finalization = (%#v, %v)", finalized, err)
	}
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: peer.RecordingID, Artifact: recordings.RecordingArtifactReference(peerPath), Scope: peer.Scope,
		},
	})
	if err != nil {
		t.Fatalf("public peer historical read: %v", err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("peer history = %#v, want only peer facts in admitted order", history.Events)
	}
	assertPublicHistoricalFact(t, history.Events[0], first)
	assertPublicHistoricalFact(t, history.Events[1], second)
	assertReadOnlyPeerReplay(t, process, peerPath, &providerRuns)
}

func assertReadOnlyPeerReplay(t *testing.T, process support.Process, peerPath string, providerRuns *atomic.Int32) {
	t.Helper()
	beforeReplay, err := os.ReadFile(peerPath)
	if err != nil {
		t.Fatalf("read replay source: %v", err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", t.TempDir(), "--replay", peerPath, "--no-record"})
	home := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"HOMEDRIVE="+filepath.VolumeName(home), "HOMEPATH="+strings.TrimPrefix(home, filepath.VolumeName(home)))
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(peer replay): %v\n%s", err, inputs.Stderr())
	}
	afterReplay, err := os.ReadFile(peerPath)
	if err != nil || !reflect.DeepEqual(beforeReplay, afterReplay) || providerRuns.Load() != 0 {
		t.Fatalf("read-only peer replay changed source or executed provider: read=%v provider=%d", err, providerRuns.Load())
	}
}

func assertPublicHistoricalFact(t *testing.T, got, want recordings.CanonicalEvent) {
	t.Helper()
	var gotPayload, wantPayload any
	if err := json.Unmarshal([]byte(got.Payload), &gotPayload); err != nil {
		t.Fatalf("historical payload: %v", err)
	}
	if err := json.Unmarshal([]byte(want.Payload), &wantPayload); err != nil {
		t.Fatalf("admitted payload: %v", err)
	}
	if got.ID != want.ID || got.Sequence != want.Sequence || got.Scope != want.Scope ||
		got.RecordedAt != want.RecordedAt || got.Kind != want.Kind || !reflect.DeepEqual(gotPayload, wantPayload) {
		t.Fatalf("historical fact = %#v, want admitted fact %#v", got, want)
	}
}

func assertGatedPublicRecordingFailure(t *testing.T, service recordings.Service, id recordings.RecordingID, finishedAt time.Time, writeStarted <-chan struct{}, release func(), writeErr error) {
	t.Helper()
	type outcome struct {
		result recordings.FinishRecordingResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := service.FinishRecording(recordings.FinishRecordingRequest{
			RecordingID: id, FinishedAt: finishedAt,
		})
		done <- outcome{result, err}
	}()
	select {
	case <-writeStarted:
	case finished := <-done:
		t.Fatalf("finalization returned before writer start: %v", finished.err)
	case <-time.After(30 * time.Second):
		t.Fatal("final writer did not start")
	}
	select {
	case <-done:
		t.Fatal("scope finalization returned before the writer completed")
	default:
	}
	release()
	var failedOutcome outcome
	select {
	case failedOutcome = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("scope finalization did not join the writer")
	}
	status := failedOutcome.result.Status
	if !errors.Is(failedOutcome.err, writeErr) || status.State != recordings.RecordingFailed ||
		status.FlushedThrough != nil || status.FinalizedAt == nil || *status.FinalizedAt != finishedAt {
		t.Fatalf("failed scope = (%#v, %v), want exact UTC metadata and no false durability", status, failedOutcome.err)
	}
}

func bindPublicRecording(t *testing.T, service recordings.Service, id recordings.RecordingID, sessionID, path string) recordings.RecordingStatusFacts {
	t.Helper()
	opened, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: id, Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID}, Artifact: recordings.RecordingArtifactReference(path),
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	t.Cleanup(func() {
		_, _ = service.StopRecording(recordings.StopRecordingRequest{RecordingID: id})
	})
	return opened.Status
}

func recordPublicRecordingFact(t *testing.T, service recordings.Service, status recordings.RecordingStatusFacts, id string, sequence recordings.CanonicalEventSequence, at time.Time) recordings.CanonicalEvent {
	t.Helper()
	event := recordings.CanonicalEvent{
		ID: recordings.CanonicalEventID(id), Scope: status.Scope, RecordedAt: at, Sequence: sequence,
		Cursor: recordings.CanonicalEventCursor{StreamGenerationID: string(status.RecordingID), Sequence: sequence},
		Kind:   "WORK_REQUEST", Payload: `{"type":"FACTORY_REQUEST_BATCH","works":[]}`,
	}
	if sequence == 0 {
		event.Kind = "RUN_REQUEST"
		event.Payload = `{"recordedAt":"` + at.Format(time.RFC3339Nano) + `","factory":{"id":"factory-history","name":"history","metadata":{"factory_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}}}`
	}
	_, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: status.RecordingID, Event: event,
	})
	if err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	return event
}
