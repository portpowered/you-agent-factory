package run

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/terminalpolicy"
)

func TestFailureBaseline_NormalModeSuppressesRawStructuredTerminalLogs(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	policy := terminalpolicy.Resolve(terminalpolicy.Options{})
	logger, err := policy.BuildLogger()
	if err != nil {
		t.Fatalf("BuildLogger: %v", err)
	}

	var startupOut bytes.Buffer
	stdout, stderr, runErr := runWithCapturedTerminal(t, RunConfig{
		Dir:                dir,
		Port:               0,
		WorkFile:           workFile,
		MockWorkersEnabled: true,
		TerminalPolicy:     policy,
		Logger:             logger,
		StartupOutput:      policy.HumanTerminalWriter(&startupOut),
		DisableDefaultRecording: true,
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	if !strings.Contains(startupOut.String(), "Factory initiated") {
		t.Fatalf("startup output = %q, want human-facing startup lines in normal mode", startupOut.String())
	}
	assertNoRawStructuredTerminalLogs(t, stdout)
	assertNoRawStructuredTerminalLogs(t, stderr)
}

func assertNoRawStructuredTerminalLogs(t *testing.T, output string) {
	t.Helper()

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if looksLikeStructuredLogLine(line) {
			t.Fatalf("terminal output contains raw structured log line %q", line)
		}
	}
}

func looksLikeStructuredLogLine(line string) bool {
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return false
	}
	_, hasLevel := record["level"]
	_, hasMsg := record["msg"]
	_, hasTS := record["ts"]
	_, hasTimestamp := record["timestamp"]
	return hasLevel && hasMsg && (hasTS || hasTimestamp)
}

func runWithCapturedTerminal(t *testing.T, cfg RunConfig) (stdout, stderr string, err error) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}

	os.Stdout = stdoutWrite
	os.Stderr = stderrWrite

	stdoutCh := make(chan []byte, 1)
	stderrCh := make(chan []byte, 1)
	go readPipeIntoChannel(stdoutRead, stdoutCh)
	go readPipeIntoChannel(stderrRead, stderrCh)

	runErr := Run(context.Background(), cfg)

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	if err := stdoutWrite.Close(); err != nil {
		t.Fatalf("close captured stdout writer: %v", err)
	}
	if err := stderrWrite.Close(); err != nil {
		t.Fatalf("close captured stderr writer: %v", err)
	}

	return string(<-stdoutCh), string(<-stderrCh), runErr
}

func readPipeIntoChannel(readPipe *os.File, out chan<- []byte) {
	data, _ := io.ReadAll(readPipe)
	_ = readPipe.Close()
	out <- data
}
