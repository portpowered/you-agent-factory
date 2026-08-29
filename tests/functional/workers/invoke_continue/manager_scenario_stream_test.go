package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type s8StreamCapture struct {
	inputs  *support.CapturedInputs
	writer  *s8SignalWriter
	command *support.ProcessCommand
	closed  func()
}

func startS8LiveStream(
	t *testing.T,
	fixture *invokeContinuePackageFixture,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, factorySessionID, workerSessionID string,
	providerSessionID string,
) s8StreamCapture {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", serverURL, "--json", "worker-sessions", "stream",
		"--worker-session-id", workerSessionID, "--follow",
	})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	writer := newS8SignalWriter(workerSessionID, inputs.Stderr)
	inputs.Input.Stdout = writer
	fixture.streamsOpened.Add(1)
	var closeOnce sync.Once
	closed := func() { closeOnce.Do(func() { fixture.streamsClosed.Add(1) }) }
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() {
		command.Stop(t)
		closed()
	})
	return s8StreamCapture{inputs: inputs, writer: writer, command: command, closed: closed}
}

func waitS8Stream(t *testing.T, stream s8StreamCapture, workerSessionID string) {
	t.Helper()
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case <-stream.command.Done():
		stream.closed()
		if err := stream.command.Err(); err != nil {
			t.Fatalf("live stream for %q: %v\nstdout:\n%s\nstderr:\n%s", workerSessionID, err, stream.writer.bytes(), stream.inputs.Stderr())
		}
	case <-watchdog.C:
		t.Fatalf("deadlock watchdog expired waiting for live stream %q", workerSessionID)
	}
}

type s8SignalWriter struct {
	mu              sync.Mutex
	data            bytes.Buffer
	workerSessionID string
	diagnostics     func() string
	ready           chan struct{}
	readyOnce       sync.Once
}

func newS8SignalWriter(workerSessionID string, diagnostics func() string) *s8SignalWriter {
	return &s8SignalWriter{workerSessionID: workerSessionID, diagnostics: diagnostics, ready: make(chan struct{})}
}

func (writer *s8SignalWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	_, _ = writer.data.Write(data)
	ready := writer.hasWorkerSessionFrameLocked()
	writer.mu.Unlock()
	if ready {
		writer.readyOnce.Do(func() { close(writer.ready) })
	}
	return len(data), nil
}

func (writer *s8SignalWriter) waitWorkerSessionFrame(t *testing.T, workerSessionID string) {
	t.Helper()
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case <-writer.ready:
	case <-watchdog.C:
		diagnostics := ""
		if writer.diagnostics != nil {
			diagnostics = writer.diagnostics()
		}
		t.Fatalf("deadlock watchdog expired waiting for complete public live stream frame %q\nstdout:\n%s\nstderr:\n%s", workerSessionID, writer.bytes(), diagnostics)
	}
}

func (writer *s8SignalWriter) hasWorkerSessionFrameLocked() bool {
	for _, line := range strings.Split(writer.data.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var frame struct {
			WorkerSessionID string `json:"workerSessionId"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err == nil && frame.WorkerSessionID == writer.workerSessionID {
			return true
		}
	}
	return false
}

func (writer *s8SignalWriter) bytes() []byte {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return append([]byte(nil), writer.data.Bytes()...)
}
