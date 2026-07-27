package output_test

import (
	"bytes"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const slowWriterScenarioTimeout = 30 * time.Second

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

func waitForStdoutWriteAttempt(t *testing.T, writer *gatedStdoutWriter) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if writer.attempts.Load() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for stdout write under backpressure")
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
