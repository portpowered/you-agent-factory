package inference_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	trace := &wsrFT009Trace{started: time.Now()}
	writer := newWSRFT009DurableWriter(t, sidecarRoot, platformreplay.NewLocal(runtime.GOOS))
	writer.trace = trace
	writer.rejectAfterPosition = wantPosition
	writer.rejectFailureMarker = true
	runner := &wsrFT009BlockingProviderRunner{started: make(chan struct{}), trace: trace}
	dir := wsrFT004Factory(t)
	// Prepare an idle host before measuring the opening barrier. Cold process
	// initialization is not Worker admission, and must not consume its ceiling.
	support.ClearSeedInputs(t, dir)
	api := support.NewProcessAPIServer()
	trace.fixture = dir
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: runner,
		WorkerRecordingWriter: writer,
		FactoryRuntimeInputs:  &wsrFT009InputObserver{delegate: platformfilesystem.Local{}, trace: trace},
	})
	if err != nil {
		t.Fatalf("BuildProcess(first run): %v", err)
	}
	support.CleanupProcess(t, process)
	invocationContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(invocationContext, []string{
		"you", "run", "--dir", dir, "--continuously", "--with-server", "--server", "http://127.0.0.1:1", "--quiet", "--record", filepath.Join(t.TempDir(), "wsr-ft-009.json"),
	})
	inputs.Input.Env = sharedInferenceProcessEnvironment(t.TempDir())
	inputs.Input.WorkingDirectory = dir
	var stdout, stderr wsrFT009Output
	inputs.Input.Stdout, inputs.Input.Stderr = &stdout, &stderr
	trace.observeInput(t, dir, sidecarRoot, inputs.Input.Args, inputs.Input.Env)
	done := make(chan struct{})
	var executeErr error // Published once by closing done; never read before done.
	go func() {
		trace.add("Execute entry")
		executeErr = process.Execute(inputs.Input)
		trace.add("Execute exit error=%v", executeErr)
		close(done)
	}()
	t.Cleanup(func() {
		trace.add("cleanup cancellation requested")
		cancel()
		select {
		case <-done:
			trace.add("cleanup Execute joined error=%v", executeErr)
		case <-time.After(5 * time.Second): //nolint:testsleep // Cleanup joins the owned Execute completion channel; five seconds is only the retained failure ceiling.
			t.Error("cleanup could not join Execute; no recovery attempted")
		}
		t.Logf("diagnostic stdout=%q stderr=%q provider_calls=%d\n%s", stdout.snapshot(), stderr.snapshot(), runner.calls.Load(), trace.snapshot())
	})
	// These waits synchronize only with the injected durable edge and blocked
	// command runner; they do not poll or sleep while waiting for a product
	// state transition.
	baseURL := api.WaitForURL(t)
	support.WaitForStatus(t, baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	opened := support.OpenFactorySessionAt(t, baseURL, dir)
	trace.add("idle host ready; submitting scenario Work")
	support.SubmitSessionWorkAt(t, baseURL, opened.Session.Id, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Payload: "WSR-FT-009 interrupted durable opening",
	})
	trace.wait(t, writer.openingPersisted, done, &executeErr, "durable Worker opening", &stdout, &stderr)
	trace.wait(t, runner.started, done, &executeErr, "blocked provider invocation", &stdout, &stderr)
	trace.add("cancellation requested")
	cancel()
	select {
	case <-done:
		trace.add("canceled Execute joined error=%v", executeErr)
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
	trace.add("fresh root read recording=%q worker=%q sessions=%d recovery_calls=%d", recordingID, workerSessionID, len(recovered.Sessions), recoveryRunner.calls.Load())
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
			session.LastPosition, session.InterruptionReason, wantPosition, recordings.WorkerRecordingInterruptionProcessStopped,
		)
	}
	if len(session.Records) != 1 || session.Records[0].ID.Position != wantPosition {
		t.Fatalf("recovered durable prefix = %#v, want only opening position %d", session.Records, wantPosition)
	}
	if session.ExecutionTerminal != nil {
		t.Fatalf("recovered interrupted session fabricated execution terminal: %#v", session.ExecutionTerminal)
	}
	trace.add("unchanged recovery assertions passed: INCOMPLETE/process-stopped, position 1 only, no terminal")
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
	appender, ok := storage.(platformreplay.Appender)
	if !ok {
		t.Fatal("test recording storage must support append")
	}
	delegate, err := recordingswire.NewWorkerRecordingFileWriter(storage, appender, platformreplay.NewLocal(runtime.GOOS), platformclock.Real{}, sidecarRoot, uuid.NewString())
	if err != nil {
		t.Fatalf("NewWorkerRecordingFileWriter(): %v", err)
	}
	return &wsrFT009DurableWriter{
		WorkerControlOperationStore:  delegate,
		WorkerRestartInputStore:      delegate,
		delegate:                     delegate,
		WorkerCapturedActivityReader: delegate,
		reader:                       delegate,
		failureWriter:                delegate,
		openingPersisted:             make(chan struct{}),
	}
}

type wsrFT009DurableWriter struct {
	recordings.WorkerCapturedActivityReader
	recordings.WorkerControlOperationStore
	recordings.WorkerRestartInputStore
	trace         *wsrFT009Trace
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
) (resultErr error) {
	writer.trace.add("persist entry recording=%q worker=%q position=%d", record.RecordingID, record.WorkerSessionID, record.Record.ID.Position)
	defer func() { writer.trace.add("persist exit position=%d error=%v", record.Record.ID.Position, resultErr) }()
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
		writer.trace.add("durable opening succeeded")
		writer.openingOnce.Do(func() { close(writer.openingPersisted) })
	}
	return nil
}

func (writer *wsrFT009DurableWriter) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) (resultErr error) {
	writer.trace.add("failure marker entry")
	defer func() { writer.trace.add("failure marker exit error=%v", resultErr) }()
	if writer.rejectFailureMarker {
		return errors.New("injected interrupted-record degradation-marker boundary")
	}
	return writer.failureWriter.PersistWorkerRecordingFailure(ctx, failure)
}

func (writer *wsrFT009DurableWriter) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	writer.trace.add("reader entry recording=%q", recordingID)
	snapshot, err := writer.reader.LoadWorkerRecording(ctx, recordingID)
	writer.trace.add("reader exit recording=%q sessions=%d error=%v", recordingID, len(snapshot.Sessions), err)
	return snapshot, err
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

type wsrFT009BlockingProviderRunner struct {
	trace       *wsrFT009Trace
	startedOnce sync.Once
	started     chan struct{}
	calls       atomic.Int32
}

func (runner *wsrFT009BlockingProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	// Do not log provider stdin or inherited environment; these may contain secrets.
	runner.trace.add("provider entry command=%q workdir=%q scope=%q call=%d", request.Command, request.WorkDir, request.ExecutionScopeID, runner.calls.Load())
	runner.startedOnce.Do(func() { close(runner.started) })
	<-ctx.Done()
	runner.trace.add("provider exit error=%v", ctx.Err())
	return platformprocess.CommandResult{}, ctx.Err()
}

// The observer is invocation-owned. Locks protect only snapshots/appends, never
// Execute, persistence, or readiness; the real edge results remain authoritative.
type wsrFT009Trace struct {
	mu        sync.Mutex
	started   time.Time
	fixture   string
	entries   []wsrFT009TraceEntry
	truncated bool
}

type wsrFT009TraceEntry struct {
	elapsed time.Duration
	format  string
	args    []any
	input   *wsrFT009InputObservation
}

// Only the already-published input filesystem edge is observed. Directory
// walking, Work admission and scheduler progress are outside this scope.
// Each call delegates once, without holding a trace lock or inspecting content.
type wsrFT009InputObserver struct {
	delegate platformfilesystem.Local
	trace    *wsrFT009Trace
	calls    atomic.Int64
}

type wsrFT009InputObservation struct {
	call                int64
	method, state, path string
	count               int
	metadata            bool
	err                 error
}

func (observer *wsrFT009InputObserver) observe(call int64, method, state, path string, count int, metadata bool, err error) {
	observer.trace.append(wsrFT009TraceEntry{
		elapsed: time.Since(observer.trace.started),
		input:   &wsrFT009InputObservation{call: call, method: method, state: state, path: path, count: count, metadata: metadata, err: err},
	})
}

func (observer *wsrFT009InputObserver) ReadDir(path string) ([]fs.DirEntry, error) {
	call := observer.calls.Add(1)
	observer.observe(call, "ReadDir", "entered", path, 0, false, nil)
	entries, err := observer.delegate.ReadDir(path)
	observer.observe(call, "ReadDir", "returned", path, len(entries), false, err)
	return entries, err
}

func (observer *wsrFT009InputObserver) ReadFile(path string) ([]byte, error) {
	call := observer.calls.Add(1)
	observer.observe(call, "ReadFile", "entered", path, 0, false, nil)
	data, err := observer.delegate.ReadFile(path)
	observer.observe(call, "ReadFile", "returned", path, len(data), false, err)
	return data, err
}

func (observer *wsrFT009InputObserver) Stat(path string) (fs.FileInfo, error) {
	call := observer.calls.Add(1)
	observer.observe(call, "Stat", "entered", path, 0, false, nil)
	info, err := observer.delegate.Stat(path)
	observer.observe(call, "Stat", "returned", path, 0, info != nil, err)
	return info, err
}

func wsrFT009InputErrorClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, fs.ErrNotExist):
		return "not-exist"
	case errors.Is(err, fs.ErrPermission):
		return "permission"
	case errors.Is(err, fs.ErrInvalid):
		return "invalid"
	default:
		return "other"
	}
}

func (trace *wsrFT009Trace) add(format string, args ...any) {
	if trace == nil {
		return
	}
	trace.append(wsrFT009TraceEntry{elapsed: time.Since(trace.started), format: format, args: args})
}

func (trace *wsrFT009Trace) append(entry wsrFT009TraceEntry) {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if len(trace.entries) >= 4096 {
		trace.truncated = true
		return
	}
	trace.entries = append(trace.entries, entry)
}

func (trace *wsrFT009Trace) snapshot() string {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	lines := make([]string, 0, len(trace.entries)+1)
	for index, entry := range trace.entries {
		message := fmt.Sprintf(entry.format, entry.args...)
		if entry.input != nil {
			input := entry.input
			relative, err := filepath.Rel(trace.fixture, input.path)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				relative = fmt.Sprintf("outside-fixture:sha256:%x", sha256.Sum256([]byte(input.path)))
			}
			state := input.state
			if input.err != nil {
				state = "error"
			}
			message = fmt.Sprintf("input call=%d method=%s state=%s path=%q count=%d metadata_non_nil=%t error=%s", input.call, input.method, state, filepath.ToSlash(relative), input.count, input.metadata, wsrFT009InputErrorClass(input.err))
		}
		lines = append(lines, fmt.Sprintf("%03d +%s %s", index+1, entry.elapsed, wsrFT009Sanitize(message)))
	}
	if trace.truncated {
		lines = append(lines, "OBSERVATION TRUNCATED: evidence incomplete; no additional diagnostic run authorized")
	}
	return strings.Join(lines, "\n")
}

func (trace *wsrFT009Trace) wait(t *testing.T, signal, done <-chan struct{}, executeErr *error, description string, stdout, stderr *wsrFT009Output) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second) //nolint:testsleep // Injected edge/completion signals drive readiness; preserve the operator-required five-second witness failure ceiling.
	defer timer.Stop()
	trace.add("waiting for %s", description)
	select {
	case <-done:
		t.Fatalf("Execute completed before %s: error=%s stdout=%q stderr=%q\n%s", description, wsrFT009Sanitize(fmt.Sprint(*executeErr)), stdout.snapshot(), stderr.snapshot(), trace.snapshot())
	case <-signal:
		// Completion is also checked when both channels were ready at selection.
		select {
		case <-done:
			t.Fatalf("Execute already completed at %s: error=%s stdout=%q stderr=%q\n%s", description, wsrFT009Sanitize(fmt.Sprint(*executeErr)), stdout.snapshot(), stderr.snapshot(), trace.snapshot())
		default:
			trace.add("observed %s", description)
		}
	case <-timer.C:
		t.Fatalf("timed out waiting for %s; last observed boundaries:\n%s\nstdout=%q stderr=%q", description, trace.snapshot(), stdout.snapshot(), stderr.snapshot())
	}
}

func (trace *wsrFT009Trace) observeInput(t *testing.T, dir, sidecarRoot string, args, env []string) {
	t.Helper()
	trace.add("input argv=%q factory=%q sidecar=%q", args, dir, sidecarRoot)
	for _, value := range env {
		key, _, _ := strings.Cut(value, "=")
		switch key {
		case "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH":
			trace.add("profile %s", value)
		}
	}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		trace.add("fixture file=%q bytes=%d sha256=%x", relative, len(data), sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatalf("observe unchanged fixture identity: %v", err)
	}
}

type wsrFT009Output struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (output *wsrFT009Output) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.Write(data)
}

func (output *wsrFT009Output) snapshot() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return wsrFT009Sanitize(output.buffer.String())
}

func wsrFT009Sanitize(value string) string {
	// Retain diagnostics and test-owned identities, redact credential values.
	credential := regexp.MustCompile(`(?i)(bearer\s+|(?:api[_-]?key|token|password|secret)["']?\s*[:=]\s*["']?)[^\s,"'}]+`)
	return credential.ReplaceAllString(value, "${1}[REDACTED]")
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

// Journals contain no stored projection/completion metadata to strip.
func (storage *wsrFT009StripCompletionMetadataStorage) AppendFile(path string, data []byte) error {
	return storage.delegate.(platformreplay.Appender).AppendFile(path, data)
}
