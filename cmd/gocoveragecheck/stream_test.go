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

type streamChildTestCase struct {
	name       string
	stream     bool
	exitCode   int
	wantStream bool
}

type streamCommandResult struct {
	stdout string
	stderr string
	err    error
}

func TestCommandRunnerStreamsCoverageChildOutputWithoutChangingCapturedBytes(t *testing.T) {
	originalCommandRunner := commandRunner
	originalExecCommand := execCommand
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		execCommand = originalExecCommand
	})

	tests := []streamChildTestCase{
		{name: "buffered success", exitCode: 0},
		{name: "streamed success", stream: true, exitCode: 0, wantStream: true},
		{name: "buffered failure", exitCode: 7},
		{name: "streamed failure", stream: true, exitCode: 7, wantStream: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) { runStreamChildCase(t, originalCommandRunner, tc) })
	}
}

func runStreamChildCase(t *testing.T, runner commandRunnerFunc, tc streamChildTestCase) {
	stdoutSink := newStreamCapture(streamChildStdoutOne + streamChildStdoutTwo)
	stderrSink := newStreamCapture(streamChildStderrOne + streamChildStderrTwo)
	resultCh, releaseWriter := startStreamChild(t, runner, tc, stdoutSink, stderrSink)
	if tc.wantStream {
		assertStreamVisibleBeforeCompletion(t, resultCh, stdoutSink, stderrSink)
	}
	if _, err := releaseWriter.Write([]byte{1}); err != nil {
		t.Fatalf("release child: %v", err)
	}
	if err := releaseWriter.Close(); err != nil {
		t.Fatalf("close child release: %v", err)
	}
	result := waitForStreamChild(t, resultCh)
	assertStreamChildResult(t, tc, result, stdoutSink, stderrSink)
}

func startStreamChild(t *testing.T, runner commandRunnerFunc, tc streamChildTestCase, stdoutSink *streamCapture, stderrSink *streamCapture) (<-chan streamCommandResult, *io.PipeWriter) {
	t.Helper()
	releaseReader, releaseWriter := io.Pipe()
	t.Cleanup(func() { _ = releaseWriter.Close() })
	execCommand = func(string, ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestGocoveragecheckStreamChildProcess", "--")
		cmd.Stdin = releaseReader
		return cmd
	}
	commandRunner = runner
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
	resultCh := make(chan streamCommandResult, 1)
	go func() {
		stdout, stderr, err := runCommand(invocation)
		resultCh <- streamCommandResult{stdout: stdout, stderr: stderr, err: err}
	}()
	return resultCh, releaseWriter
}

func assertStreamVisibleBeforeCompletion(t *testing.T, resultCh <-chan streamCommandResult, stdoutSink *streamCapture, stderrSink *streamCapture) {
	t.Helper()
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

func waitForStreamChild(t *testing.T, resultCh <-chan streamCommandResult) streamCommandResult {
	t.Helper()
	select {
	case result := <-resultCh:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for coverage child")
		return streamCommandResult{}
	}
}

func assertStreamChildResult(t *testing.T, tc streamChildTestCase, result streamCommandResult, stdoutSink *streamCapture, stderrSink *streamCapture) {
	t.Helper()
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
		return
	}
	if got := stdoutSink.String(); got != "" {
		t.Fatalf("buffered stdout sink = %q, want no child bytes", got)
	}
	if got := stderrSink.String(); got != "" {
		t.Fatalf("buffered stderr sink = %q, want no child bytes", got)
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

func TestCompletedCoverageStreamModePreservesCoverageAndVerdict(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	configPackage := modulePath + "/pkg/config"
	for _, tc := range []struct {
		name string
		min  float64
	}{
		{name: "passing", min: 80},
		{name: "failing", min: 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observations := make([]completedCoverageObservation, 0, 2)
			for _, stream := range []bool{false, true} {
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				stdoutWriter = &stdout
				stderrWriter = &stderr
				commandRunner = func(invocation commandInvocation) (string, string, error) {
					childStdout, childStderr, err := fakeGoCoverageCommandPassing(invocation)
					if invocation.stdoutWriter != nil {
						if _, writeErr := io.WriteString(invocation.stdoutWriter, childStdout); writeErr != nil {
							return childStdout, childStderr, writeErr
						}
					}
					if invocation.stderrWriter != nil {
						if _, writeErr := io.WriteString(invocation.stderrWriter, childStderr); writeErr != nil {
							return childStdout, childStderr, writeErr
						}
					}
					return childStdout, childStderr, err
				}

				runErr := execute(config{
					min:       tc.min,
					totalOnly: true,
					coverpkg:  configPackage,
					packages:  "./pkg/config",
					stream:    stream,
				})
				match := totalCoveragePattern.FindStringSubmatch(stdout.String())
				if len(match) != 2 {
					t.Fatalf("stream=%v stdout = %q, want total coverage percentage", stream, stdout.String())
				}
				observations = append(observations, completedCoverageObservation{
					stream:     stream,
					percentage: match[1],
					verdict:    errorString(runErr),
				})
				if stderr.Len() != 0 {
					t.Fatalf("stream=%v stderr = %q, want empty", stream, stderr.String())
				}
			}

			if observations[0].percentage != observations[1].percentage {
				t.Fatalf("coverage percentage buffered=%q, streamed=%q, want identical", observations[0].percentage, observations[1].percentage)
			}
			if observations[0].verdict != observations[1].verdict {
				t.Fatalf("coverage verdict buffered=%q, streamed=%q, want identical", observations[0].verdict, observations[1].verdict)
			}
			if tc.min <= 100 && observations[0].verdict != "" {
				t.Fatalf("buffered verdict = %q, want pass", observations[0].verdict)
			}
			if tc.min > 100 && observations[0].verdict == "" {
				t.Fatal("buffered verdict unexpectedly passed")
			}
		})
	}
}

type completedCoverageObservation struct {
	stream     bool
	percentage string
	verdict    string
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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

func waitForStreamCapture(t *testing.T, complete <-chan struct{}) {
	t.Helper()
	select {
	case <-complete:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for streamed child output")
	}
}
