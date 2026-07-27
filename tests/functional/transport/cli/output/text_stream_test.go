package output_test

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	humanTextStreamScenarioTimeout = 30 * time.Second
	textStreamPrimaryResult        = "mock worker accepted"
)

// TestCLITextStreamSurfacesIncrementalMessages proves a human response-stream
// CLI run surfaces lifecycle progress on stdout while the invocation is still
// in flight, before the terminal primary result is written.
func TestCLITextStreamSurfacesIncrementalMessages(t *testing.T) {
	writer := newFirstChunkGatedStdoutWriter()
	runGoalHumanResponseStreamWithStdout(t, writer)

	waitForFirstChunkStdoutContent(t, writer)
	if !containsHumanLifecycleLine(writer.String()) {
		t.Fatalf("stdout missing incremental human lifecycle output before completion:\n%s", writer.String())
	}
	select {
	case <-writer.done:
		t.Fatal("invocation completed before releasing stdout gate")
	default:
	}

	writer.release()

	select {
	case <-writer.done:
	case <-time.After(humanTextStreamScenarioTimeout):
		t.Fatalf("timed out waiting for invocation to finish after releasing stdout gate")
	}
	if writer.err != nil {
		t.Fatalf("Process.Execute error = %v\nstdout:\n%s", writer.err, writer.String())
	}
	if writer.diagnosticText() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", writer.diagnosticText())
	}

	stdout := writer.String()
	lines := nonEmptyStdoutLines(stdout)
	if len(lines) < 3 {
		t.Fatalf("stdout lines = %#v, want lifecycle, separator, and final response", lines)
	}
	if lines[len(lines)-2] != "--- primary result ---" {
		t.Fatalf("penultimate stdout line = %q, want primary-result separator\nstdout:\n%s", lines[len(lines)-2], stdout)
	}
	if lines[len(lines)-1] != textStreamPrimaryResult {
		t.Fatalf("final stdout line = %q, want %q\nstdout:\n%s", lines[len(lines)-1], textStreamPrimaryResult, stdout)
	}
	for _, line := range lines[:len(lines)-2] {
		if !isHumanFactoryLifecycleLine(line) {
			t.Fatalf("stdout line %q is not canonical customer lifecycle output\nstdout:\n%s", line, stdout)
		}
	}
}

type firstChunkGatedStdoutWriter struct {
	gate        chan struct{}
	releaseOnce sync.Once

	attempts       atomic.Int64
	firstChunkSeen atomic.Bool

	mu         sync.Mutex
	buffer     bytes.Buffer
	diagnostic bytes.Buffer

	done chan struct{}
	err  error
}

func newFirstChunkGatedStdoutWriter() *firstChunkGatedStdoutWriter {
	return &firstChunkGatedStdoutWriter{
		gate: make(chan struct{}),
		done: make(chan struct{}),
	}
}

func (writer *firstChunkGatedStdoutWriter) Write(payload []byte) (int, error) {
	writer.attempts.Add(1)
	if !writer.firstChunkSeen.Swap(true) {
		writer.mu.Lock()
		defer writer.mu.Unlock()
		return writer.buffer.Write(payload)
	}
	<-writer.gate
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.Write(payload)
}

func (writer *firstChunkGatedStdoutWriter) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.String()
}

func (writer *firstChunkGatedStdoutWriter) diagnosticText() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.diagnostic.String()
}

func (writer *firstChunkGatedStdoutWriter) release() {
	writer.releaseOnce.Do(func() {
		close(writer.gate)
	})
}

func waitForFirstChunkStdoutContent(t *testing.T, writer *firstChunkGatedStdoutWriter) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.TrimSpace(writer.String()) != "" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for first human response-stream stdout chunk")
}

func containsHumanLifecycleLine(stdout string) bool {
	for _, line := range nonEmptyStdoutLines(stdout) {
		if isHumanFactoryLifecycleLine(line) {
			return true
		}
	}
	return false
}

func nonEmptyStdoutLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func isHumanFactoryLifecycleLine(line string) bool {
	closingBracket := strings.Index(line, "] ")
	if !strings.HasPrefix(line, "[") || closingBracket < 2 {
		return false
	}
	message := line[closingBracket+2:]
	for _, prefix := range []string{
		"work accepted", "work moved", "Factory Session started", "Factory Session completed",
		"workstation queued", "workstation started", "workstation completed", "workstation failed", "workstation interrupted",
		"inference started", "inference completed", "inference failed", "workflow phase", "workflow checkpoint written",
		"final output updated",
	} {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

func runGoalHumanResponseStreamWithStdout(t *testing.T, stdout *firstChunkGatedStdoutWriter) {
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
		"you", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record", "--output", "response-stream",
		"deterministic human text-stream incremental contract",
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
		case <-time.After(humanTextStreamScenarioTimeout):
			t.Errorf("timed out waiting for invocation cleanup")
		}
	})
}
