package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	streamChildEnv       = "GOCOVERAGECHECK_STREAM_CHILD"
	streamChildExitEnv   = "GOCOVERAGECHECK_STREAM_CHILD_EXIT"
	streamChildStdoutOne = "stdout child chunk one\n"
	streamChildStdoutTwo = "stdout child chunk two\n"
	streamChildStderrOne = "stderr child chunk one\n"
	streamChildStderrTwo = "stderr child chunk two\n"
)

func TestCommandRunnerStreamsCoverageChildOutputWithoutChangingCapturedBytes(t *testing.T) {
	originalCommandRunner := commandRunner
	originalExecCommand := execCommand
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		execCommand = originalExecCommand
	})

	tests := []struct {
		name       string
		stream     bool
		exitCode   int
		wantStream bool
	}{
		{name: "buffered success", exitCode: 0},
		{name: "streamed success", stream: true, exitCode: 0, wantStream: true},
		{name: "buffered failure", exitCode: 7},
		{name: "streamed failure", stream: true, exitCode: 7, wantStream: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			releaseReader, releaseWriter := io.Pipe()
			t.Cleanup(func() { _ = releaseWriter.Close() })
			execCommand = func(string, ...string) *exec.Cmd {
				cmd := exec.Command(os.Args[0], "-test.run=TestGocoveragecheckStreamChildProcess", "--")
				cmd.Stdin = releaseReader
				return cmd
			}
			commandRunner = originalCommandRunner

			stdoutSink := newStreamCapture(streamChildStdoutOne + streamChildStdoutTwo)
			stderrSink := newStreamCapture(streamChildStderrOne + streamChildStderrTwo)
			invocation := commandInvocation{
				name: "coverage-test-child",
				env: append(os.Environ(),
					streamChildEnv+"=1",
					streamChildExitEnv+"="+strconv.Itoa(tc.exitCode),
				),
			}
			if tc.stream {
				invocation.stdoutWriter = stdoutSink
				invocation.stderrWriter = stderrSink
			}

			type commandResult struct {
				stdout string
				stderr string
				err    error
			}
			resultCh := make(chan commandResult, 1)
			go func() {
				stdout, stderr, err := runCommand(invocation)
				resultCh <- commandResult{stdout: stdout, stderr: stderr, err: err}
			}()

			if tc.wantStream {
				waitForStreamCapture(t, stdoutSink.complete)
				waitForStreamCapture(t, stderrSink.complete)
				if got := stdoutSink.String(); got != streamChildStdoutOne+streamChildStdoutTwo {
					t.Fatalf("streamed stdout before child completion = %q, want first two chunks", got)
				}
				if got := stderrSink.String(); got != streamChildStderrOne+streamChildStderrTwo {
					t.Fatalf("streamed stderr before child completion = %q, want first two chunks", got)
				}
				select {
				case result := <-resultCh:
					t.Fatalf("child completed before release: %+v", result)
				default:
				}
			}
			if _, err := releaseWriter.Write([]byte{1}); err != nil {
				t.Fatalf("release child: %v", err)
			}
			if err := releaseWriter.Close(); err != nil {
				t.Fatalf("close child release: %v", err)
			}

			var result commandResult
			select {
			case result = <-resultCh:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for coverage child")
			}
			wantStdout := streamChildStdoutOne + streamChildStdoutTwo
			wantStderr := streamChildStderrOne + streamChildStderrTwo
			if result.stdout != wantStdout || result.stderr != wantStderr {
				t.Fatalf("captured child output = (%q, %q), want byte-identical (%q, %q)", result.stdout, result.stderr, wantStdout, wantStderr)
			}
			if tc.exitCode == 0 && result.err != nil {
				t.Fatalf("runCommand() error = %v, want nil", result.err)
			}
			if tc.exitCode != 0 && (result.err == nil || !strings.Contains(result.err.Error(), "exit status "+strconv.Itoa(tc.exitCode))) {
				t.Fatalf("runCommand() error = %v, want exit status %d", result.err, tc.exitCode)
			}
			if tc.wantStream {
				if got := stdoutSink.String(); got != wantStdout {
					t.Fatalf("streamed stdout after child completion = %q, want %q", got, wantStdout)
				}
				if got := stderrSink.String(); got != wantStderr {
					t.Fatalf("streamed stderr after child completion = %q, want %q", got, wantStderr)
				}
			} else {
				if got := stdoutSink.String(); got != "" {
					t.Fatalf("buffered stdout sink = %q, want no child bytes", got)
				}
				if got := stderrSink.String(); got != "" {
					t.Fatalf("buffered stderr sink = %q, want no child bytes", got)
				}
			}
		})
	}
}

func TestGocoveragecheckStreamChildProcess(t *testing.T) {
	if os.Getenv(streamChildEnv) != "1" {
		return
	}
	_, _ = io.WriteString(os.Stdout, streamChildStdoutOne)
	_, _ = io.WriteString(os.Stderr, streamChildStderrOne)
	_, _ = io.WriteString(os.Stdout, streamChildStdoutTwo)
	_, _ = io.WriteString(os.Stderr, streamChildStderrTwo)

	var release [1]byte
	if _, err := io.ReadFull(os.Stdin, release[:]); err != nil {
		os.Exit(8)
	}
	exitCode, err := strconv.Atoi(os.Getenv(streamChildExitEnv))
	if err != nil {
		os.Exit(9)
	}
	os.Exit(exitCode)
}

func TestMainStreamFlagConfiguresOnlyCoverageChildOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stream bool
	}{
		{name: "omitted", stream: false},
		{name: "enabled", stream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotStdoutWriter io.Writer
			var gotStderrWriter io.Writer
			runner := func(invocation commandInvocation) (string, string, error) {
				if len(invocation.args) == 0 || invocation.args[0] != "test" {
					return "", "", fmt.Errorf("unexpected invocation: %v", invocation.args)
				}
				gotStdoutWriter = invocation.stdoutWriter
				gotStderrWriter = invocation.stderrWriter
				profilePath := helperCoverProfilePath(invocation.args)
				if err := writeFakeCoverageProfile(profilePath, "mode: count\n"+modulePath+"/pkg/config/config.go:1.1,2.1 1 1\n"); err != nil {
					return "", "", err
				}
				return "", "", nil
			}
			args := []string{
				"-min=0",
				"-total-only",
				"-coverpkg=" + modulePath + "/pkg/config",
				"-packages=./pkg/config",
			}
			if tc.stream {
				args = append(args, "-stream")
			}
			_, stderr, exitCode := runMainForTest(t, args, runner)
			if exitCode != 0 || stderr != "" {
				t.Fatalf("main() result = exit %d, stderr %q, want success", exitCode, stderr)
			}
			if (gotStdoutWriter != nil) != tc.stream || (gotStderrWriter != nil) != tc.stream {
				t.Fatalf("coverage invocation stream writers = (%v, %v), want enabled=%v", gotStdoutWriter != nil, gotStderrWriter != nil, tc.stream)
			}
		})
	}
}

type streamCapture struct {
	mu          sync.Mutex
	data        bytes.Buffer
	expectedLen int
	complete    chan struct{}
	once        sync.Once
}

func newStreamCapture(expected string) *streamCapture {
	return &streamCapture{
		expectedLen: len(expected),
		complete:    make(chan struct{}),
	}
}

func (w *streamCapture) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.data.Write(data)
	if w.data.Len() >= w.expectedLen {
		w.once.Do(func() { close(w.complete) })
	}
	return n, err
}

func (w *streamCapture) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.data.String()
}

func waitForStreamCapture(t *testing.T, written <-chan struct{}) {
	t.Helper()
	select {
	case <-written:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for streamed child output")
	}
}
