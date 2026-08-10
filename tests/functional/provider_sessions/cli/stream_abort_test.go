package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess(t *testing.T) {
	streamServer := newAbortedWorkerSessionStreamServer(t)
	defer streamServer.Close()

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), workerSessionAbortArgs(streamServer.URL))
	inputs.Input.Env = functionalEnvironment(t.TempDir())
	inputs.Input.WorkingDirectory = t.TempDir()

	err := process.Execute(inputs.Input)
	var typed *workersessionscli.CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_STREAM_CLOSED" {
		t.Fatalf("Process.Execute() error = %v, want typed WORKER_SESSION_STREAM_CLOSED", err)
	}
	assertWorkerSessionStreamAbortOutput(t, inputs.Stdout(), inputs.Stderr())
}

func TestBuiltWorkerSessionsStreamAbortExitsNonZero(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), workerSessionAbortTestTimeout)
	defer cancel()
	streamServer := newAbortedWorkerSessionStreamServer(t)
	defer streamServer.Close()

	command := exec.CommandContext(ctx, buildWorkerSessionsCLIBinary(t), workerSessionAbortArgs(streamServer.URL)[1:]...)
	command.Dir = t.TempDir()
	command.Env = functionalEnvironment(t.TempDir())
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	var exitErr *exec.ExitError
	if err == nil || !errors.As(err, &exitErr) {
		t.Fatalf("built Worker Sessions stream error = %v, want non-zero process exit", err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("built Worker Sessions stream exit code = %d, want non-zero", exitErr.ExitCode())
	}
	assertWorkerSessionStreamAbortOutput(t, stdout.String(), stderr.String())
}

func TestBuiltWorkerSessionsStreamCancellationExits130(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt is not supported for child processes on Windows")
	}

	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("stream response writer does not support flushing")
			return
		}
		_, _ = fmt.Fprint(w, "data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-cancel\",\"providerSession\":null,\"workIds\":[],\"event\":{\"position\":1,\"sourceType\":\"worker_session\",\"sourceId\":\"worker-session-cancel\",\"sourceSequence\":1,\"sourceEventId\":\"event-1\",\"schemaId\":\"worker_session.started\",\"payload\":{\"state\":\"RUNNING\"}},\"errorCode\":null,\"errorMessage\":null}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer streamServer.Close()

	ctx, cancel := context.WithTimeout(t.Context(), workerSessionAbortTestTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, buildWorkerSessionsCLIBinary(t), workerSessionAbortArgs(streamServer.URL)[1:]...)
	command.Dir = t.TempDir()
	command.Env = functionalEnvironment(t.TempDir())
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open Worker Sessions stdout: %v", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start Worker Sessions stream: %v", err)
	}
	finished := false
	defer func() {
		if !finished {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()

	frame := make(chan string, 1)
	scanErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			frame <- scanner.Text()
			return
		}
		scanErr <- scanner.Err()
	}()
	var retainedFrame string
	select {
	case line := <-frame:
		retainedFrame = line
		if !strings.Contains(line, `"delivery":"RECORD"`) {
			t.Fatalf("Worker Sessions cancellation frame = %q, want retained RECORD frame", line)
		}
	case err := <-scanErr:
		t.Fatalf("Worker Sessions stream ended before retained frame: %v; stderr=%q", err, stderr.String())
	case <-time.After(workerSessionAbortTestTimeout):
		t.Fatal("timed out waiting for retained Worker Sessions frame")
	}

	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt Worker Sessions stream: %v", err)
	}
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
	}()
	select {
	case err := <-waitResult:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
			t.Fatalf("interrupted Worker Sessions stream exit = %v; stderr=%q, want exit code 130", err, stderr.String())
		}
	case <-time.After(workerSessionAbortTestTimeout):
		_ = command.Process.Kill()
		<-waitResult
		t.Fatal("interrupted Worker Sessions stream did not exit")
	}
	finished = true
	if strings.TrimSpace(retainedFrame) == "" {
		t.Fatal("Worker Sessions cancellation did not retain the complete frame")
	}
	if strings.Contains(retainedFrame, "TERMINAL") {
		t.Fatalf("Worker Sessions cancellation synthesized terminal output: %q", retainedFrame)
	}
}

const workerSessionAbortTestTimeout = 30 * time.Second

func newAbortedWorkerSessionStreamServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-abort\",\"providerSession\":null,\"workIds\":[],\"event\":{\"position\":1,\"sourceType\":\"worker_session\",\"sourceId\":\"worker-session-abort\",\"sourceSequence\":1,\"sourceEventId\":\"event-1\",\"schemaId\":\"worker_session.started\",\"payload\":{\"state\":\"RUNNING\"}},\"errorCode\":null,\"errorMessage\":null}\n\n")
	}))
}

func workerSessionAbortArgs(serverURL string) []string {
	return []string{
		"you", "--server", serverURL, "worker-sessions", "stream",
		"--provider", "codex", "--kind", "session_id", "--id", "provider-session-abort", "--output", "json",
	}
}

func assertWorkerSessionStreamAbortOutput(t *testing.T, stdout, stderr string) {
	t.Helper()
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("aborted stream stdout lines = %d, want one retained frame:\n%s", len(lines), stdout)
	}
	var frame streamFrameJSON
	if err := json.Unmarshal([]byte(lines[0]), &frame); err != nil {
		t.Fatalf("decode retained Worker Session frame: %v\nstdout:%s", err, stdout)
	}
	if frame.Delivery != "RECORD" || frame.Event == nil || frame.Event.Position != 1 {
		t.Fatalf("retained stream frame = %#v, want one RECORD at position 1", frame)
	}
	if strings.Contains(stdout, "TERMINAL") || strings.Contains(stdout, "REPLAY_SUMMARY") {
		t.Fatalf("aborted stream synthesized a terminal/success frame:\n%s", stdout)
	}

	var diagnostic struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &diagnostic); err != nil {
		t.Fatalf("decode one Worker Session stream diagnostic: %v\nstderr:%s", err, stderr)
	}
	if diagnostic.Code != "WORKER_SESSION_STREAM_CLOSED" || strings.TrimSpace(diagnostic.Message) == "" {
		t.Fatalf("stream diagnostic = %#v, want one coded safe diagnostic", diagnostic)
	}
	if strings.Count(stderr, "WORKER_SESSION_STREAM_CLOSED") != 1 {
		t.Fatalf("stream diagnostic code occurrences = %d, want exactly one:\n%s", strings.Count(stderr, "WORKER_SESSION_STREAM_CLOSED"), stderr)
	}
}
