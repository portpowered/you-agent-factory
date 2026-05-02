package workers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type recordingCommandLogger struct {
	infos []recordedCommandLog
}

type recordedCommandLog struct {
	msg    string
	fields map[string]any
}

func (l *recordingCommandLogger) Debug(_ string, _ ...any) {}
func (l *recordingCommandLogger) Info(msg string, keysAndValues ...any) {
	l.infos = append(l.infos, recordedCommandLog{
		msg:    msg,
		fields: commandLogFieldsMap(keysAndValues...),
	})
}
func (l *recordingCommandLogger) Warn(_ string, _ ...any)  {}
func (l *recordingCommandLogger) Error(_ string, _ ...any) {}

type fixedCommandRunnerWithError struct {
	result CommandResult
	err    error
}

func (r fixedCommandRunnerWithError) Run(context.Context, CommandRequest) (CommandResult, error) {
	return r.result, r.err
}

type loggingCommandRunnerCase struct {
	name            string
	result          CommandResult
	err             error
	wantStatus      string
	wantCommandData bool
}

func TestExecCommandRunner_SuccessfulProcessCapturesOutputAndInputs(t *testing.T) {
	workDir := t.TempDir()
	result, err := ExecCommandRunner{}.Run(context.Background(), CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestExecCommandRunner_HelperProcess",
			"--",
			"success",
		},
		Stdin:   []byte("stdin-value"),
		Env:     append(os.Environ(), "GO_WANT_COMMAND_HELPER=1", "COMMAND_HELPER_WANT_STDIN=stdin-value", "COMMAND_HELPER_WANT_CWD="+workDir),
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if strings.TrimSpace(string(result.Stdout)) != "command helper success" {
		t.Fatalf("Stdout = %q, want helper success", result.Stdout)
	}
	if len(result.Stderr) != 0 {
		t.Fatalf("Stderr = %q, want empty", result.Stderr)
	}
}

func TestExecCommandRunner_NonZeroExitReturnsResultWithoutError(t *testing.T) {
	result, err := ExecCommandRunner{}.Run(context.Background(), CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestExecCommandRunner_HelperProcess",
			"--",
			"fail",
		},
		Env: append(os.Environ(), "GO_WANT_COMMAND_HELPER=1"),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 17 {
		t.Fatalf("ExitCode = %d, want 17", result.ExitCode)
	}
	if strings.TrimSpace(string(result.Stderr)) != "command helper failed" {
		t.Fatalf("Stderr = %q, want helper failure", result.Stderr)
	}
}

func TestExecCommandRunner_ContextDeadlineReturnsSystemError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	result, err := ExecCommandRunner{}.Run(ctx, CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestExecCommandRunner_HelperProcess",
			"--",
			"sleep",
		},
		Env: append(os.Environ(), "GO_WANT_COMMAND_HELPER=1"),
	})
	if err == nil {
		t.Fatal("Run error = nil, want context deadline error")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("Run error = %v, want %v", err, context.DeadlineExceeded)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want zero value for system error", result.ExitCode)
	}
}

func TestExecCommandRunner_ContextDeadlineTerminatesSpawnedChildProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result, err := ExecCommandRunner{}.Run(ctx, CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestExecCommandRunner_HelperProcess",
			"--",
			"spawn-child",
		},
		Env: append(os.Environ(),
			"GO_WANT_COMMAND_HELPER=1",
			"COMMAND_HELPER_PID_FILE="+pidFile,
		),
	})
	if err == nil {
		t.Fatal("Run error = nil, want context deadline error")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("Run error = %v, want %v; stdout=%q stderr=%q", err, context.DeadlineExceeded, result.Stdout, result.Stderr)
	}

	childPID := readCommandHelperPID(t, pidFile)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})
	if !waitForCommandHelperProcessExit(childPID, 2*time.Second) {
		t.Fatalf("spawned child process %d is still running after command timeout", childPID)
	}
}

func TestLoggingCommandRunner_LogsRequestAndCompletionStatuses(t *testing.T) {
	cases := []loggingCommandRunnerCase{
		{
			name:            "success",
			result:          CommandResult{Stdout: []byte("ok\n")},
			wantStatus:      "succeeded",
			wantCommandData: false,
		},
		{
			name:            "non-zero exit",
			result:          CommandResult{Stdout: []byte("partial\n"), Stderr: []byte("failed\n"), ExitCode: 17},
			wantStatus:      "failed",
			wantCommandData: true,
		},
		{
			name:            "timeout",
			result:          CommandResult{Stderr: []byte("deadline\n")},
			err:             context.DeadlineExceeded,
			wantStatus:      "timed_out",
			wantCommandData: true,
		},
		{
			name:            "system error",
			err:             errors.New("start failed"),
			wantStatus:      "error",
			wantCommandData: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertLoggingCommandRunnerCase(t, tc)
		})
	}
}

func assertLoggingCommandRunnerCase(t *testing.T, tc loggingCommandRunnerCase) {
	t.Helper()

	logger := &recordingCommandLogger{}
	runner := LoggingCommandRunner{
		Runner: fixedCommandRunnerWithError{
			result: tc.result,
			err:    tc.err,
		},
		Logger: logger,
	}
	req := loggingCommandRunnerTestRequest()

	result, err := runner.Run(context.Background(), req)
	assertLoggingCommandRunnerOutcome(t, result, err, tc)
	if len(logger.infos) != 2 {
		t.Fatalf("logged info records = %d, want 2", len(logger.infos))
	}

	assertLoggingCommandRunnerRequestLog(t, logger.infos[0].fields)
	assertLoggingCommandRunnerCompletionLog(t, logger.infos[1].fields, req, tc)
}

func loggingCommandRunnerTestRequest() CommandRequest {
	return CommandRequest{
		Command: "script-tool",
		Args:    []string{"--mode", "fixture"},
		Stdin:   []byte("stdin"),
		Env:     []string{"VISIBLE=1"},
		WorkDir: "/tmp/work",
		Execution: interfaces.ExecutionMetadata{
			RequestID: "request-command",
			TraceID:   "trace-command",
			WorkIDs:   []string{"work-command"},
		},
	}
}

func assertLoggingCommandRunnerOutcome(t *testing.T, result CommandResult, err error, tc loggingCommandRunnerCase) {
	t.Helper()
	if !errors.Is(err, tc.err) {
		t.Fatalf("Run error = %v, want %v", err, tc.err)
	}
	if string(result.Stdout) != string(tc.result.Stdout) || string(result.Stderr) != string(tc.result.Stderr) || result.ExitCode != tc.result.ExitCode {
		t.Fatalf("Run result = %#v, want %#v", result, tc.result)
	}
}

func assertLoggingCommandRunnerRequestLog(t *testing.T, fields map[string]any) {
	t.Helper()
	if fields["event_name"] != WorkLogEventCommandRunnerRequested {
		t.Fatalf("request event_name = %#v, want %q", fields["event_name"], WorkLogEventCommandRunnerRequested)
	}
	if fields["status"] != "requested" {
		t.Fatalf("request status = %#v, want requested", fields["status"])
	}
	if _, ok := fields["env_count"]; ok {
		t.Fatalf("request event unexpectedly contains env_count: %#v", fields["env_count"])
	}
}

func assertLoggingCommandRunnerCompletionLog(t *testing.T, fields map[string]any, req CommandRequest, tc loggingCommandRunnerCase) {
	t.Helper()
	if fields["event_name"] != WorkLogEventCommandRunnerCompleted {
		t.Fatalf("completion event_name = %#v, want %q", fields["event_name"], WorkLogEventCommandRunnerCompleted)
	}
	if fields["status"] != tc.wantStatus {
		t.Fatalf("completion status = %#v, want %q", fields["status"], tc.wantStatus)
	}
	if fields["request_id"] != req.Execution.RequestID || fields["trace_id"] != req.Execution.TraceID || fields["work_id"] != req.Execution.WorkIDs[0] {
		t.Fatalf("completion correlation fields = %#v", fields)
	}
	assertLoggingCommandRunnerCommandData(t, fields, tc)
}

func assertLoggingCommandRunnerCommandData(t *testing.T, fields map[string]any, tc loggingCommandRunnerCase) {
	t.Helper()

	stdout, hasStdout := fields["stdout"]
	stderr, hasStderr := fields["stderr"]
	if tc.wantCommandData {
		if !hasStdout || stdout != string(tc.result.Stdout) {
			t.Fatalf("completion stdout = %#v, want %q", stdout, tc.result.Stdout)
		}
		if !hasStderr || stderr != string(tc.result.Stderr) {
			t.Fatalf("completion stderr = %#v, want %q", stderr, tc.result.Stderr)
		}
		return
	}
	if hasStdout {
		t.Fatalf("completion unexpectedly includes stdout = %#v", stdout)
	}
	if hasStderr {
		t.Fatalf("completion unexpectedly includes stderr = %#v", stderr)
	}
}

func TestExecCommandRunner_HelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_COMMAND_HELPER") != "1" {
		return
	}
	if len(os.Args) == 0 {
		fmt.Fprintln(os.Stderr, "missing args")
		os.Exit(2)
	}

	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "success":
		assertCommandHelperInputs()
		fmt.Fprintln(os.Stdout, "command helper success")
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stderr, "command helper failed")
		os.Exit(17)
	case "sleep":
		time.Sleep(time.Second)
		os.Exit(0)
	case "spawn-child":
		spawnCommandHelperChild()
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "pid-sleep":
		writeCommandHelperPID()
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
		os.Exit(2)
	}
}

func commandLogFieldsMap(keysAndValues ...any) map[string]any {
	fields := make(map[string]any, len(keysAndValues)/2)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[i+1]
	}
	return fields
}

func assertCommandHelperInputs() {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
		os.Exit(2)
	}
	if want := os.Getenv("COMMAND_HELPER_WANT_STDIN"); string(stdin) != want {
		fmt.Fprintf(os.Stderr, "stdin = %q, want %q\n", stdin, want)
		os.Exit(2)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "get cwd: %v\n", err)
		os.Exit(2)
	}
	wantCWD := os.Getenv("COMMAND_HELPER_WANT_CWD")
	if got, want := filepath.Clean(cwd), filepath.Clean(wantCWD); got != want {
		fmt.Fprintf(os.Stderr, "cwd = %q, want %q\n", got, want)
		os.Exit(2)
	}
}

func spawnCommandHelperChild() {
	pidFile := os.Getenv("COMMAND_HELPER_PID_FILE")
	if pidFile == "" {
		fmt.Fprintln(os.Stderr, "missing COMMAND_HELPER_PID_FILE")
		os.Exit(2)
	}
	time.Sleep(100 * time.Millisecond)
	child := exec.Command(os.Args[0],
		"-test.run=TestExecCommandRunner_HelperProcess",
		"--",
		"pid-sleep",
	)
	child.Env = append(os.Environ(), "GO_WANT_COMMAND_HELPER=1", "COMMAND_HELPER_PID_FILE="+pidFile)
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start child: %v\n", err)
		os.Exit(2)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "child did not write pid file")
	os.Exit(2)
}

func writeCommandHelperPID() {
	pidFile := os.Getenv("COMMAND_HELPER_PID_FILE")
	if pidFile == "" {
		fmt.Fprintln(os.Stderr, "missing COMMAND_HELPER_PID_FILE")
		os.Exit(2)
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write pid file: %v\n", err)
		os.Exit(2)
	}
}

func readCommandHelperPID(t *testing.T, pidFile string) int {
	t.Helper()

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse child pid %q: %v", raw, err)
	}
	return pid
}

func waitForCommandHelperProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !commandTestProcessRunning(pid) {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return !commandTestProcessRunning(pid)
}
