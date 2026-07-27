package output_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	slowWriterScenarioTimeout    = 30 * time.Second
	writerFailureScenarioTimeout = 30 * time.Second
	writerFailureStdoutError     = "broken stdout pipe"
)

// TestCLISlowWriterDoesNotReorderResponseEvents proves CLI NDJSON
// response-stream output preserves Factory Event and terminal InvocationResult
// emission order when stdout is blocked by a slow consumer, so parsers never
// observe reordered lifecycle or result records under backpressure.
func TestCLISlowWriterDoesNotReorderResponseEvents(t *testing.T) {
	writer := newGatedStdoutWriter()
	stdout := runGoalResponseStreamWithStdout(t, writer)

	waitForStdoutWriteAttempt(t, writer)
	select {
	case <-writer.done:
		t.Fatal("invocation completed while stdout writer remained blocked")
	default:
	}
	writer.release()

	select {
	case <-writer.done:
	case <-time.After(slowWriterScenarioTimeout):
		t.Fatalf("timed out waiting for invocation to finish after releasing slow stdout writer")
	}
	if err := writer.err; err != nil {
		t.Fatalf("Process.Execute error = %v\nstdout:\n%s", err, stdout.String())
	}
	if writer.diagnosticText() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", writer.diagnosticText())
	}

	records := decodeNDJSONRecords(t, stdout.String())
	previousSequence := -1
	previousSessionSequence := -1
	factoryEventCount := 0
	invocationResultSeen := false
	for index, record := range records {
		switch record.RecordType {
		case factoryEventRecordType:
			if invocationResultSeen {
				t.Fatalf("Factory Event record %d follows terminal invocation result", index)
			}
			assertFactoryEventSequenceMonotonic(t, record, index, &previousSequence, &previousSessionSequence)
			factoryEventCount++
		case invocationResultType:
			if invocationResultSeen {
				t.Fatalf("second invocation_result at record %d", index)
			}
			invocationResultSeen = true
			if index != len(records)-1 {
				t.Fatalf("invocation_result record index = %d, want terminal index %d", index, len(records)-1)
			}
			assertInvocationResultRecord(t, record, index)
		default:
			t.Fatalf("record %d has unsupported recordType %q", index, record.RecordType)
		}
	}
	if factoryEventCount == 0 {
		t.Fatal("response stream contains no Factory Event records to order")
	}
	if previousSessionSequence < 0 {
		t.Fatal("Factory Event records contain no public Factory Session sequence data")
	}
	if !invocationResultSeen {
		t.Fatal("response stream missing terminal invocation_result")
	}
}

type gatedStdoutWriter struct {
	gate     chan struct{}
	attempts atomic.Int64
	releaseOnce sync.Once

	mu         sync.Mutex
	buffer     bytes.Buffer
	diagnostic bytes.Buffer

	done chan struct{}
	err  error
}

func newGatedStdoutWriter() *gatedStdoutWriter {
	return &gatedStdoutWriter{
		gate: make(chan struct{}),
		done: make(chan struct{}),
	}
}

func (writer *gatedStdoutWriter) Write(payload []byte) (int, error) {
	writer.attempts.Add(1)
	<-writer.gate
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.Write(payload)
}

func (writer *gatedStdoutWriter) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.String()
}

func (writer *gatedStdoutWriter) diagnosticText() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.diagnostic.String()
}

func (writer *gatedStdoutWriter) release() {
	writer.releaseOnce.Do(func() {
		close(writer.gate)
	})
}

func waitForStdoutWriteAttempt(t *testing.T, writer stdoutWriteObserver) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if writer.writeAttempts() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for stdout write under backpressure")
}

type stdoutWriteObserver interface {
	writeAttempts() int64
}

func (writer *gatedStdoutWriter) writeAttempts() int64 {
	return writer.attempts.Load()
}

func (writer *inFlightFailureStdoutWriter) writeAttempts() int64 {
	return writer.attempts.Load()
}

func runGoalResponseStreamWithStdout(t *testing.T, stdout *gatedStdoutWriter) *gatedStdoutWriter {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, goalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	args := []string{
		"you", "--json", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record", "--output", "response-stream",
		"deterministic slow-writer ordering contract",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Stdout = stdout
	inputs.Input.Stderr = &stdout.diagnostic
	process := support.BuildProcess(t, serviceedges.Edges{})

	go func() {
		stdout.err = process.Execute(inputs.Input)
		close(stdout.done)
	}()

	t.Cleanup(func() {
		stdout.release()
		select {
		case <-stdout.done:
		case <-time.After(slowWriterScenarioTimeout):
			t.Errorf("timed out waiting for invocation cleanup")
		}
	})
	return stdout
}

// TestCLIWriterFailureCancelsInvocation proves a broken stdout writer ends the
// CLI response-stream invocation unsuccessfully and cancels in-flight mock-worker
// external work so no orphaned subprocess remains after the CLI returns.
func TestCLIWriterFailureCancelsInvocation(t *testing.T) {
	externalWork := newCancellableExternalWorkRunner()
	writer := newInFlightFailureStdoutWriter(errors.New(writerFailureStdoutError), externalWork)
	runGoalResponseStreamWriterFailure(t, externalWork, writer)

	waitForExternalWorkStart(t, externalWork)
	waitForStdoutBufferedRecords(t, writer)

	select {
	case <-writer.done:
	case <-time.After(writerFailureScenarioTimeout):
		t.Fatalf("timed out waiting for invocation to finish after stdout writer failure")
	}

	if writer.err == nil {
		t.Fatalf("Process.Execute error = nil, want stdout writer failure\nstdout:\n%s\nstderr:\n%s",
			writer.String(), writer.diagnosticText())
	}
	if !errors.Is(writer.err, writer.failErr) && !strings.Contains(writer.err.Error(), writerFailureStdoutError) {
		t.Fatalf("Process.Execute error = %v, want stdout writer failure", writer.err)
	}

	if err := externalWork.waitFinished(writerFailureScenarioTimeout); err != nil {
		t.Fatalf("external mock-worker work teardown not observed after CLI returned: %v", err)
	}
	if !errors.Is(externalWork.runErr(), context.Canceled) {
		t.Fatalf("external work cancellation error = %v, want context.Canceled", externalWork.runErr())
	}

	records := decodeNDJSONRecords(t, writer.String())
	if len(records) == 0 {
		t.Fatal("stdout empty, want Factory Event records before mid-stream writer failure")
	}
	for index, record := range records {
		if record.RecordType == invocationResultType {
			t.Fatalf("record %d is invocation_result, want writer failure before terminal record", index)
		}
	}
}

type cancellableExternalWorkRunner struct {
	startedCh   chan struct{}
	startedFlag atomic.Bool
	finished    chan struct{}
	finishOnce  sync.Once

	mu  sync.Mutex
	err error
}

func newCancellableExternalWorkRunner() *cancellableExternalWorkRunner {
	return &cancellableExternalWorkRunner{
		startedCh: make(chan struct{}, 1),
		finished:  make(chan struct{}),
	}
}

func (runner *cancellableExternalWorkRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.startedFlag.Store(true)
	select {
	case runner.startedCh <- struct{}{}:
	default:
	}
	<-ctx.Done()
	runner.mu.Lock()
	runner.err = ctx.Err()
	runner.mu.Unlock()
	runner.finishOnce.Do(func() {
		close(runner.finished)
	})
	return platformprocess.CommandResult{}, ctx.Err()
}

func (runner *cancellableExternalWorkRunner) started() bool {
	return runner.startedFlag.Load()
}

func (runner *cancellableExternalWorkRunner) runErr() error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.err
}

func (runner *cancellableExternalWorkRunner) waitFinished(timeout time.Duration) error {
	select {
	case <-runner.finished:
		return nil
	case <-time.After(timeout):
		return errors.New("timed out waiting for external work teardown")
	}
}

type inFlightFailureStdoutWriter struct {
	failErr      error
	externalWork *cancellableExternalWorkRunner
	gate         chan struct{}

	attempts    atomic.Int64
	failArmed   atomic.Bool
	releaseOnce sync.Once

	mu         sync.Mutex
	buffer     bytes.Buffer
	diagnostic bytes.Buffer

	done chan struct{}
	err  error
}

func newInFlightFailureStdoutWriter(
	failErr error,
	externalWork *cancellableExternalWorkRunner,
) *inFlightFailureStdoutWriter {
	writer := &inFlightFailureStdoutWriter{
		failErr:      failErr,
		externalWork: externalWork,
		gate:         make(chan struct{}),
		done:         make(chan struct{}),
	}
	go func() {
		for externalWork != nil && !externalWork.started() {
			time.Sleep(time.Millisecond)
		}
		writer.release()
	}()
	return writer
}

func (writer *inFlightFailureStdoutWriter) release() {
	writer.releaseOnce.Do(func() {
		close(writer.gate)
	})
}

func (writer *inFlightFailureStdoutWriter) Write(payload []byte) (int, error) {
	writer.attempts.Add(1)
	<-writer.gate
	if writer.failArmed.Load() {
		return 0, writer.failErr
	}
	writer.mu.Lock()
	written, err := writer.buffer.Write(payload)
	writer.mu.Unlock()
	if err != nil {
		return written, err
	}
	if writer.externalWork != nil && writer.externalWork.started() {
		writer.failArmed.Store(true)
	}
	return written, nil
}

func (writer *inFlightFailureStdoutWriter) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.String()
}

func (writer *inFlightFailureStdoutWriter) diagnosticText() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.diagnostic.String()
}

func waitForStdoutBufferedRecords(t *testing.T, writer *inFlightFailureStdoutWriter) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.TrimSpace(writer.String()) != "" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for buffered stdout records before mid-stream writer failure")
}

func waitForExternalWorkStart(t *testing.T, runner *cancellableExternalWorkRunner) {
	t.Helper()
	select {
	case <-runner.startedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for mock-worker external work to start")
	}
}

func runGoalResponseStreamWriterFailure(
	t *testing.T,
	externalWork *cancellableExternalWorkRunner,
	stdout *inFlightFailureStdoutWriter,
) {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, goalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, writerFailureGoalMockWorkers())
	args := []string{
		"you", "--json", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record", "--output", "response-stream",
		"deterministic writer-failure cancellation contract",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Stdout = stdout
	inputs.Input.Stderr = &stdout.diagnostic
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: externalWork,
	})

	go func() {
		stdout.err = process.Execute(inputs.Input)
		close(stdout.done)
	}()

	t.Cleanup(func() {
		stdout.release()
		select {
		case <-stdout.done:
		case <-time.After(writerFailureScenarioTimeout):
			t.Errorf("timed out waiting for invocation cleanup")
		}
	})
}

func writerFailureGoalMockWorkers() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeScript,
			ScriptConfig: &workers.MockWorkerScriptConfig{
				Command: "you-writer-failure-external-work",
				Args:    []string{"in-flight"},
			},
		}},
	}
}

var _ platformprocess.CommandRunner = (*cancellableExternalWorkRunner)(nil)
