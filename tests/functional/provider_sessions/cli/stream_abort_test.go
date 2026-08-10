package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
