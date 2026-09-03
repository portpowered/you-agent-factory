package inference_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWSRFT009CanonicalRestartRecoversWorkerHistory proves that a fresh
// root-composed process reads the atomically durable Worker sidecar without
// consulting the live Worker Session registry or rerunning the provider.
//
// WSR-FT-009: interrupted prefixes remain readable as INCOMPLETE and a
// durable terminal derives COMPLETE even when completion metadata is absent.
func TestWSRFT009CanonicalRestartRecoversWorkerHistory(t *testing.T) {
	t.Run("interrupted prefix is readable after restart", func(t *testing.T) {
		t.Parallel()
		testWSRFT009InterruptedPrefix(t)
	})
	t.Run("durable terminal derives completion after restart", func(t *testing.T) {
		t.Parallel()
		testWSRFT009TerminalCompletion(t)
	})
}

func testWSRFT009InterruptedPrefix(t *testing.T) {
	const wantPosition = events.AggregateSequence(1)

	sidecarRoot := t.TempDir()
	writer := newWSRFT009DurableWriter(t, sidecarRoot, platformreplay.NewLocal(runtime.GOOS))
	writer.rejectAfterPosition = wantPosition
	writer.rejectFailureMarker = true
	runner := &wsrFT009BlockingProviderRunner{started: make(chan struct{})}
	dir := wsrFT004Factory(t)

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: runner,
		WorkerRecordingWriter: writer,
	})
	if err != nil {
		t.Fatalf("BuildProcess(first run): %v", err)
	}
	support.CleanupProcess(t, process)

	invocationContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(invocationContext, []string{
		"you", "run", "--dir", dir, "--session", uuid.NewString(), "--quiet", "--record", filepath.Join(t.TempDir(), "wsr-ft-009.json"),
	})
	inputs.Input.Env = sharedInferenceProcessEnvironment(t.TempDir())
	inputs.Input.WorkingDirectory = dir
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()

	// These waits synchronize only with the injected durable edge and blocked
	// command runner; they do not poll or sleep while waiting for a product
	// state transition.
	waitWSRFT009Signal(t, writer.openingPersisted, "durable Worker opening")
	waitWSRFT009Signal(t, runner.started, "blocked provider invocation")
	cancel()
	select {
	case executeErr := <-done:
		if executeErr != nil && !errors.Is(executeErr, context.Canceled) {
			t.Fatalf("interrupted Process.Execute() error = %v", executeErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the canceled first process")
	}

	recordingID, workerSessionID := writer.identity()
	if recordingID == "" || workerSessionID == "" {
		t.Fatalf("durable Worker identity = recording %q/session %q, want both identities", recordingID, workerSessionID)
	}
	recovered, recoveryRunner := loadWSRFT009AfterRestart(t, sidecarRoot, recordingID)
	if recoveryRunner.calls.Load() != 0 {
		t.Fatalf("provider calls during interrupted-history recovery = %d, want zero", recoveryRunner.calls.Load())
	}
	if runner.calls.Load() != 1 {
		t.Fatalf("provider calls before interruption = %d, want one", runner.calls.Load())
	}
	if len(recovered.Sessions) != 1 {
		t.Fatalf("recovered Worker snapshot = %#v, want one session", recovered)
	}
	session := recovered.Sessions[0]
	if session.WorkerSessionID != workerSessionID || session.Status != recordings.WorkerRecordingStatusIncomplete {
		t.Fatalf("recovered Worker session = %#v, want %q and INCOMPLETE", session, workerSessionID)
	}
	if session.LastPosition != wantPosition ||
		session.InterruptionReason != recordings.WorkerRecordingInterruptionProcessStopped {
		t.Fatalf(
			"recovered interruption = position %d/reason %q, want position %d/%q",
			session.LastPosition, session.InterruptionReason, wantPosition,
			recordings.WorkerRecordingInterruptionProcessStopped,
		)
	}
	if len(session.Records) != 1 || session.Records[0].ID.Position != wantPosition {
		t.Fatalf("recovered durable prefix = %#v, want only opening position %d", session.Records, wantPosition)
	}
	if session.ExecutionTerminal != nil {
		t.Fatalf("recovered interrupted session fabricated execution terminal: %#v", session.ExecutionTerminal)
	}
}

func testWSRFT009TerminalCompletion(t *testing.T) {
	sidecarRoot := t.TempDir()
	storage := &wsrFT009StripCompletionMetadataStorage{delegate: platformreplay.NewLocal(runtime.GOOS)}
	writer := newWSRFT009DurableWriter(t, sidecarRoot, storage)
	probe := newWSRFT004RecordingProbe(t, false)
	runner := newWSRFT004ProviderRunner(t, probe)
	_ = runWSRFT004FactoryWithProcess(t, wsrFT004Factory(t), serviceedges.Edges{
		ProviderCommandRunner: runner,
		WorkerRecordingWriter: writer,
	})

	recordingID, _ := writer.identity()
	if recordingID == "" {
		t.Fatal("completed run did not expose a durable recording identity")
	}
	recovered, recoveryRunner := loadWSRFT009AfterRestart(t, sidecarRoot, recordingID)
	if recoveryRunner.calls.Load() != 0 {
		t.Fatalf("provider calls during terminal-history recovery = %d, want zero", recoveryRunner.calls.Load())
	}
	if len(recovered.Sessions) != 1 {
		t.Fatalf("recovered completed Worker snapshot = %#v, want one session", recovered)
	}
	session := recovered.Sessions[0]
	if session.Status != recordings.WorkerRecordingStatusComplete || session.InterruptionReason != "" {
		t.Fatalf("recovered terminal session = %#v, want COMPLETE without interruption", session)
	}
	if len(session.Records) < 2 || session.LastPosition != session.Records[len(session.Records)-1].ID.Position {
		t.Fatalf(
			"recovered terminal positions = last=%d records=%#v, want terminal-last history",
			session.LastPosition, session.Records,
		)
	}
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
		Snapshot: recovered,
	})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording(recovered terminal): %v", err)
	}
	if !replayed.Projection.Complete || replayed.Projection.Terminal == nil {
		t.Fatalf("replayed terminal projection = %#v, want COMPLETE with terminal evidence", replayed.Projection)
	}
	if replayed.Projection.Terminal.Position != session.LastPosition {
		t.Fatalf("replayed terminal position = %d, want %d", replayed.Projection.Terminal.Position, session.LastPosition)
	}
}

func newWSRFT009DurableWriter(
	t *testing.T,
	sidecarRoot string,
	storage platformreplay.Storage,
) *wsrFT009DurableWriter {
	t.Helper()
	delegate, err := recordingswire.NewWorkerRecordingFileWriter(storage, sidecarRoot)
	if err != nil {
		t.Fatalf("NewWorkerRecordingFileWriter(): %v", err)
	}
	reader, ok := delegate.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("durable Worker writer does not expose a reader")
	}
	failureWriter, ok := delegate.(recordings.WorkerRecordingFailureWriter)
	if !ok {
		t.Fatal("durable Worker writer does not expose failure persistence")
	}
	return &wsrFT009DurableWriter{
		delegate:         delegate,
		reader:           reader,
		failureWriter:    failureWriter,
		openingPersisted: make(chan struct{}),
	}
}

type wsrFT009DurableWriter struct {
	delegate      recordings.WorkerRecordingWriter
	reader        recordings.WorkerRecordingReader
	failureWriter recordings.WorkerRecordingFailureWriter

	openingOnce         sync.Once
	openingPersisted    chan struct{}
	rejectAfterPosition events.AggregateSequence
	rejectFailureMarker bool

	mu            sync.Mutex
	recordingID   string
	workerSession string
}

func (writer *wsrFT009DurableWriter) PersistWorkerRecord(
	ctx context.Context,
	record recordings.WorkerRecordingRecord,
) error {
	if writer.rejectAfterPosition > 0 && record.Record.ID.Position > writer.rejectAfterPosition {
		return errors.New("injected interrupted-record persistence boundary")
	}
	if err := writer.delegate.PersistWorkerRecord(ctx, record); err != nil {
		return err
	}
	writer.mu.Lock()
	if writer.recordingID == "" {
		writer.recordingID = record.RecordingID
		writer.workerSession = record.WorkerSessionID
	}
	writer.mu.Unlock()
	if record.Record.ID.Position == 1 {
		writer.openingOnce.Do(func() { close(writer.openingPersisted) })
	}
	return nil
}

func (writer *wsrFT009DurableWriter) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	if writer.rejectFailureMarker {
		return errors.New("injected interrupted-record degradation-marker boundary")
	}
	return writer.failureWriter.PersistWorkerRecordingFailure(ctx, failure)
}

func (writer *wsrFT009DurableWriter) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	return writer.reader.LoadWorkerRecording(ctx, recordingID)
}

func (writer *wsrFT009DurableWriter) identity() (string, string) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.recordingID, writer.workerSession
}

type wsrFT009StripCompletionMetadataStorage struct {
	delegate platformreplay.Storage
}

func (storage *wsrFT009StripCompletionMetadataStorage) WriteFile(path string, data []byte) error {
	var snapshot recordings.WorkerRecordingSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return storage.delegate.WriteFile(path, data)
	}
	for index := range snapshot.Sessions {
		snapshot.Sessions[index].Status = ""
		snapshot.Sessions[index].LastPosition = 0
		snapshot.Sessions[index].InterruptionReason = ""
	}
	stripped, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return storage.delegate.WriteFile(path, stripped)
}

func (storage *wsrFT009StripCompletionMetadataStorage) ReadFile(path string) ([]byte, error) {
	return storage.delegate.ReadFile(path)
}

func loadWSRFT009AfterRestart(
	t *testing.T,
	sidecarRoot string,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, *wsrFT009NeverCalledProviderRunner) {
	t.Helper()
	writer := newWSRFT009DurableWriter(t, sidecarRoot, platformreplay.NewLocal(runtime.GOOS))
	runner := &wsrFT009NeverCalledProviderRunner{}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: runner,
		WorkerRecordingWriter: writer,
	})
	if err != nil {
		t.Fatalf("BuildProcess(restart): %v", err)
	}
	support.CleanupProcess(t, process)
	reader := process.WorkerRecordingReader()
	if reader == nil {
		t.Fatal("restart process did not expose Worker recording reader")
	}
	snapshot, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(%q) after restart: %v", recordingID, err)
	}
	return snapshot, runner
}

func waitWSRFT009Signal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

type wsrFT009BlockingProviderRunner struct {
	startedOnce sync.Once
	started     chan struct{}
	calls       atomic.Int32
}

func (runner *wsrFT009BlockingProviderRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	runner.startedOnce.Do(func() { close(runner.started) })
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

type wsrFT009NeverCalledProviderRunner struct {
	calls atomic.Int32
}

func (runner *wsrFT009NeverCalledProviderRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{}, errors.New("provider must not run during Worker recording recovery")
}

var _ platformprocess.CommandRunner = (*wsrFT009BlockingProviderRunner)(nil)
var _ platformprocess.CommandRunner = (*wsrFT009NeverCalledProviderRunner)(nil)
var _ recordings.WorkerRecordingWriter = (*wsrFT009DurableWriter)(nil)
var _ recordings.WorkerRecordingReader = (*wsrFT009DurableWriter)(nil)
var _ recordings.WorkerRecordingFailureWriter = (*wsrFT009DurableWriter)(nil)
var _ platformreplay.Storage = (*wsrFT009StripCompletionMetadataStorage)(nil)
