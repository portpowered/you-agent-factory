package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/work"
)

// commandHelperSpawnTimeoutBudget allows slow CI hosts (especially Windows) to
// start the helper, spawn the child, and write the pid file before the test
// context deadline fires. spawn-child sleeps 10s after spawning.
const commandHelperSpawnTimeoutBudget = 3 * time.Second

func canonicalWorkerTestPath(value string) string {
	if value == "" {
		return ""
	}

	cleaned := filepath.Clean(value)
	current := cleaned
	var suffix []string
	for {
		if _, err := os.Stat(current); err == nil {
			if resolved, err := filepath.EvalSymlinks(current); err == nil && resolved != "" {
				parts := append([]string{resolved}, suffix...)
				return filepath.Join(parts...)
			}
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
	return cleaned
}

type recordingCommandLogger struct {
	infos    []recordedCommandLog
	verboses []recordedCommandLog
	warns    []recordedCommandLog
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
func (l *recordingCommandLogger) Warn(msg string, keysAndValues ...any) {
	l.warns = append(l.warns, recordedCommandLog{
		msg:    msg,
		fields: commandLogFieldsMap(keysAndValues...),
	})
}
func (l *recordingCommandLogger) Error(_ string, _ ...any) {}
func (l *recordingCommandLogger) Verbose(msg string, keysAndValues ...any) {
	l.verboses = append(l.verboses, recordedCommandLog{
		msg:    msg,
		fields: commandLogFieldsMap(keysAndValues...),
	})
}

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
		Stdin: []byte("stdin-value"),
		Env: append(os.Environ(),
			"GO_WANT_COMMAND_HELPER=1",
			"COMMAND_HELPER_WANT_STDIN=stdin-value",
			"COMMAND_HELPER_WANT_CWD="+canonicalWorkerTestPath(workDir),
		),
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
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want %v", err, context.DeadlineExceeded)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want zero value for system error", result.ExitCode)
	}
}

func TestExecCommandRunner_SuccessfulExitTerminatesSpawnedChildProcess(t *testing.T) {
	testExecCommandRunnerAgentStyleSuccessLeavesNoChildProcess(t)
}

// TestExecCommandRunner_AgentStyleSuccessLeavesNoChildProcess is the US-005 regression
// guard: a command that spawns a background service and exits 0 must not leave the child
// running after Run returns. Uses commandTestProcessRunning from command_process_test_unix.go
// or command_process_test_windows.go for platform liveness checks.
func TestExecCommandRunner_AgentStyleSuccessLeavesNoChildProcess(t *testing.T) {
	testExecCommandRunnerAgentStyleSuccessLeavesNoChildProcess(t)
}

func testExecCommandRunnerAgentStyleSuccessLeavesNoChildProcess(t *testing.T) {
	t.Helper()

	const testGrace = 250 * time.Millisecond
	postRunCleanupGracePeriodForTest = testGrace
	t.Cleanup(func() {
		postRunCleanupGracePeriodForTest = 0
	})

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	result, err := ExecCommandRunner{}.Run(context.Background(), CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestExecCommandRunner_HelperProcess",
			"--",
			"spawn-child-success",
		},
		Env: append(os.Environ(),
			"GO_WANT_COMMAND_HELPER=1",
			"COMMAND_HELPER_PID_FILE="+pidFile,
		),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	// Antivirus and indexing processes can briefly hold a newly renamed file on
	// Windows even after the helper has closed it. Treat a transient sharing
	// violation like the other pid-publication readiness states.
	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})

	waitBudget := testGrace + 3*time.Second
	if !waitForCommandHelperProcessExit(childPID, waitBudget) {
		t.Fatalf("spawned child process %d is still running after parent exit 0", childPID)
	}
}

func TestExecCommandRunner_ContextDeadlineTerminatesSpawnedChildProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), commandHelperSpawnTimeoutBudget)
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
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want %v; stdout=%q stderr=%q", err, context.DeadlineExceeded, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want zero value for timeout system error", result.ExitCode)
	}

	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})
	if !waitForCommandHelperProcessExit(childPID, 2*time.Second) {
		t.Fatalf("spawned child process %d is still running after command timeout", childPID)
	}
}

func TestExecCommandRunner_ContextCancelTerminatesSpawnedChildProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runOutcome struct {
		result CommandResult
		err    error
	}
	runDone := make(chan runOutcome, 1)
	go func() {
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
		runDone <- runOutcome{result: result, err: err}
	}()

	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})
	cancel()
	outcome := <-runDone
	result, err := outcome.result, outcome.err
	if err == nil {
		t.Fatal("Run error = nil, want context canceled error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want %v; stdout=%q stderr=%q", err, context.Canceled, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want zero value for cancel system error", result.ExitCode)
	}

	if !waitForCommandHelperProcessExit(childPID, 2*time.Second) {
		t.Fatalf("spawned child process %d is still running after context cancel", childPID)
	}
}

func TestExecCommandRunner_PostRunCleanupWaitsForParentWait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group supervision timing; Windows uses job-object post-run path")
	}

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_, _ = ExecCommandRunner{}.Run(ctx, CommandRequest{
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
	}()

	childPID := waitForCommandHelperPID(t, pidFile, 3*time.Second)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})
	if !commandTestProcessRunning(childPID) {
		t.Fatalf("child process %d exited before parent cmd.Wait returned", childPID)
	}

	cancel()
	<-runDone
	if !waitForCommandHelperProcessExit(childPID, 2*time.Second) {
		t.Fatalf("child process %d still running after supervised Run ended", childPID)
	}
}

func TestExecCommandRunner_LogsSuccessfulPostRunCleanupNoOp(t *testing.T) {
	logger := &recordingCommandLogger{}
	req := commandCleanupTestRequest(t)
	_, err := ExecCommandRunner{Logger: logger}.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	completed := commandCleanupCompletedLogs(logger)
	if len(completed) == 0 {
		t.Fatal("expected post-run cleanup completion log")
	}
	last := completed[len(completed)-1]
	assertCommandCleanupLogFields(t, last.fields, req, commandProcessCleanupReasonPostRun)
	if last.fields["outcome"] != string(commandProcessCleanupOutcomeNoOp) {
		t.Fatalf("cleanup outcome = %#v, want %q", last.fields["outcome"], commandProcessCleanupOutcomeNoOp)
	}
	if len(logger.warns) != 0 {
		t.Fatalf("unexpected warn logs: %#v", logger.warns)
	}
}

func TestExecCommandRunner_LogsCancelCleanupForceKillSuccess(t *testing.T) {
	logger := &recordingCommandLogger{}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := commandCleanupTestRequest(t)
	req.Args = []string{
		"-test.run=TestExecCommandRunner_HelperProcess",
		"--",
		"spawn-child",
	}
	req.Env = append(os.Environ(),
		"GO_WANT_COMMAND_HELPER=1",
		"COMMAND_HELPER_PID_FILE="+pidFile,
	)
	req.Command = os.Args[0]

	runDone := make(chan error, 1)
	go func() {
		_, err := ExecCommandRunner{Logger: logger}.Run(ctx, req)
		runDone <- err
	}()

	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})
	cancel()
	err := <-runDone
	if err == nil {
		t.Fatal("Run error = nil, want context canceled error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want %v", err, context.Canceled)
	}

	cancelCompleted := commandCleanupCompletedLogsForReason(logger, commandProcessCleanupReasonCancel)
	if len(cancelCompleted) == 0 {
		t.Fatal("expected cancel cleanup completion log")
	}
	lastCancel := cancelCompleted[len(cancelCompleted)-1]
	assertCommandCleanupLogFields(t, lastCancel.fields, req, commandProcessCleanupReasonCancel)
	if lastCancel.fields["outcome"] != string(commandProcessCleanupOutcomeForceKillSuccess) {
		t.Fatalf("cancel cleanup outcome = %#v, want %q", lastCancel.fields["outcome"], commandProcessCleanupOutcomeForceKillSuccess)
	}
	if _, ok := lastCancel.fields["args"]; ok {
		t.Fatalf("cleanup log unexpectedly includes args: %#v", lastCancel.fields["args"])
	}
	if _, ok := lastCancel.fields["env"]; ok {
		t.Fatalf("cleanup log unexpectedly includes env: %#v", lastCancel.fields["env"])
	}
}

func TestCommandProcessCleanupContext_LogsPartialFailureAtWarn(t *testing.T) {
	logger := &recordingCommandLogger{}
	req := commandCleanupTestRequest(t)
	logCtx := newCommandProcessCleanupContext(logger, req, commandProcessCleanupReasonPostRun)
	logCtx.logCompleted(
		commandProcessCleanupOutcomePartialFailure,
		4242,
		errors.New("process group kill failed"),
		"fallback killed parent",
	)
	if len(logger.warns) != 1 {
		t.Fatalf("warn logs = %d, want 1", len(logger.warns))
	}
	fields := logger.warns[0].fields
	if fields["outcome"] != string(commandProcessCleanupOutcomePartialFailure) {
		t.Fatalf("outcome = %#v, want partial_failure", fields["outcome"])
	}
	if fields["process_group_id"] != 4242 {
		t.Fatalf("process_group_id = %#v, want 4242", fields["process_group_id"])
	}
	assertCommandCleanupLogFields(t, fields, req, commandProcessCleanupReasonPostRun)
}

func TestLoggingCommandRunner_LogsPostRunCleanupThroughWrappedExec(t *testing.T) {
	logger := &recordingCommandLogger{}
	req := commandCleanupTestRequest(t)
	runner := CommandRunnerWithLogging(ExecCommandRunner{}, logger)

	_, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	completed := commandCleanupCompletedLogs(logger)
	if len(completed) == 0 {
		t.Fatal("expected command_runner.cleanup_completed log from wrapped exec runner")
	}
	last := completed[len(completed)-1]
	if last.fields["event_name"] != workLogEventCommandRunnerCleanupCompleted {
		t.Fatalf("event_name = %#v, want %q", last.fields["event_name"], workLogEventCommandRunnerCleanupCompleted)
	}
	assertCommandCleanupLogFields(t, last.fields, req, commandProcessCleanupReasonPostRun)
}

func TestCommandRunnerWithLogging_PropagatesLoggerToExecCommandRunner(t *testing.T) {
	logger := &recordingCommandLogger{}
	runner := CommandRunnerWithLogging(ExecCommandRunner{}, logger)
	execRunner, ok := unwrapExecCommandRunner(runner)
	if !ok {
		t.Fatal("expected wrapped ExecCommandRunner")
	}
	if execRunner.Logger == nil {
		t.Fatal("ExecCommandRunner.Logger = nil, want injected logger")
	}
}

func commandCleanupTestRequest(t *testing.T) CommandRequest {
	t.Helper()
	return CommandRequest{
		Command:         os.Args[0],
		Args:            []string{"-test.run=TestExecCommandRunner_HelperProcess", "--", "success"},
		Env:             append(os.Environ(), "GO_WANT_COMMAND_HELPER=1"),
		WorkDir:         t.TempDir(),
		DispatchID:      "dispatch-cleanup-log",
		WorkerType:      "script",
		WorkstationName: "cleanup-test-station",
		Execution: work.ExecutionMetadata{
			RequestID: "request-cleanup-log",
			TraceID:   "trace-cleanup-log",
			WorkIDs:   []string{"work-cleanup-log"},
		},
	}
}

func unwrapExecCommandRunner(runner CommandRunner) (ExecCommandRunner, bool) {
	switch typed := runner.(type) {
	case *LoggingCommandRunner:
		if typed == nil {
			return ExecCommandRunner{}, false
		}
		return unwrapExecCommandRunner(typed.Runner)
	case ExecCommandRunner:
		return typed, true
	case *ExecCommandRunner:
		if typed == nil {
			return ExecCommandRunner{}, false
		}
		return *typed, true
	default:
		return ExecCommandRunner{}, false
	}
}

func commandCleanupCompletedLogs(logger *recordingCommandLogger) []recordedCommandLog {
	return commandCleanupLogsByEvent(logger, workLogEventCommandRunnerCleanupCompleted)
}

func commandCleanupCompletedLogsForReason(logger *recordingCommandLogger, reason commandProcessCleanupReason) []recordedCommandLog {
	var completed []recordedCommandLog
	for _, entry := range commandCleanupCompletedLogs(logger) {
		if entry.fields["cleanup_reason"] == string(reason) {
			completed = append(completed, entry)
		}
	}
	return completed
}

func commandCleanupLogsByEvent(logger *recordingCommandLogger, eventName string) []recordedCommandLog {
	var matched []recordedCommandLog
	for _, bucket := range [][]recordedCommandLog{logger.infos, logger.verboses, logger.warns} {
		for _, entry := range bucket {
			if entry.fields["event_name"] == eventName {
				matched = append(matched, entry)
			}
		}
	}
	return matched
}

func assertCommandCleanupLogFields(
	t *testing.T,
	fields map[string]any,
	req CommandRequest,
	reason commandProcessCleanupReason,
) {
	t.Helper()
	if fields["command"] != req.Command {
		t.Fatalf("command = %#v, want %q", fields["command"], req.Command)
	}
	if fields["dispatch_id"] != req.DispatchID {
		t.Fatalf("dispatch_id = %#v, want %q", fields["dispatch_id"], req.DispatchID)
	}
	if fields["cleanup_reason"] != string(reason) {
		t.Fatalf("cleanup_reason = %#v, want %q", fields["cleanup_reason"], reason)
	}
	if fields["request_id"] != req.Execution.RequestID {
		t.Fatalf("request_id = %#v, want %q", fields["request_id"], req.Execution.RequestID)
	}
	if fields["trace_id"] != req.Execution.TraceID {
		t.Fatalf("trace_id = %#v, want %q", fields["trace_id"], req.Execution.TraceID)
	}
	if fields["args_count"] != len(req.Args) {
		t.Fatalf("args_count = %#v, want %d", fields["args_count"], len(req.Args))
	}
	if req.WorkDir != "" && fields["working_dir"] != req.WorkDir {
		t.Fatalf("working_dir = %#v, want %q", fields["working_dir"], req.WorkDir)
	}
	if req.WorkerType != "" && fields["worker_type"] != req.WorkerType {
		t.Fatalf("worker_type = %#v, want %q", fields["worker_type"], req.WorkerType)
	}
	if req.WorkstationName != "" && fields["workstation_name"] != req.WorkstationName {
		t.Fatalf("workstation_name = %#v, want %q", fields["workstation_name"], req.WorkstationName)
	}
	assertCommandCleanupLogExcludesSensitiveFields(t, fields)
}

func assertCommandCleanupLogExcludesSensitiveFields(t *testing.T, fields map[string]any) {
	t.Helper()
	for _, forbidden := range []string{"stdin", "stdin_bytes", "env"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatalf("cleanup log unexpectedly includes %q", forbidden)
		}
	}
	if _, ok := fields["args"]; ok {
		t.Fatal("cleanup log unexpectedly includes full args slice")
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
			wantCommandData: false,
		},
		{
			name:            "timeout",
			result:          CommandResult{Stderr: []byte("deadline\n")},
			err:             context.DeadlineExceeded,
			wantStatus:      "timed_out",
			wantCommandData: false,
		},
		{
			name:            "system error",
			err:             errors.New("start failed"),
			wantStatus:      "error",
			wantCommandData: false,
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
	if len(logger.verboses) != 2 {
		t.Fatalf("logged verbose records = %d, want 2", len(logger.verboses))
	}

	assertLoggingCommandRunnerRequestLog(t, logger.infos[0].fields)
	assertLoggingCommandRunnerCompletionLog(t, logger.infos[1].fields, req, tc)
	assertLoggingCommandRunnerVerboseRequestLog(t, logger.verboses[0].fields)
	assertLoggingCommandRunnerVerboseCompletionLog(t, logger.verboses[1].fields, req)
}

func loggingCommandRunnerTestRequest() CommandRequest {
	return CommandRequest{
		Command: "script-tool",
		Args:    []string{"--mode", "fixture"},
		Stdin:   []byte("stdin"),
		Env:     []string{"VISIBLE=1"},
		WorkDir: "/tmp/work",
		Execution: work.ExecutionMetadata{
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
	if fields["event_name"] != workLogEventCommandRunnerRequested {
		t.Fatalf("request event_name = %#v, want %q", fields["event_name"], workLogEventCommandRunnerRequested)
	}
	if fields["status"] != "requested" {
		t.Fatalf("request status = %#v, want requested", fields["status"])
	}
	if _, ok := fields["env_count"]; ok {
		t.Fatalf("request event unexpectedly contains env_count: %#v", fields["env_count"])
	}
}

func assertLoggingCommandRunnerVerboseRequestLog(t *testing.T, fields map[string]any) {
	t.Helper()
	if fields["event_name"] != workLogEventCommandRunnerRequestDetails {
		t.Fatalf("verbose request event_name = %#v, want %q", fields["event_name"], workLogEventCommandRunnerRequestDetails)
	}
	if fields["args_count"] != 2 {
		t.Fatalf("verbose request args_count = %#v, want 2", fields["args_count"])
	}
	if fields["stdin_bytes"] != len([]byte("stdin")) {
		t.Fatalf("verbose request stdin_bytes = %#v, want %d", fields["stdin_bytes"], len([]byte("stdin")))
	}
}

func assertLoggingCommandRunnerCompletionLog(t *testing.T, fields map[string]any, req CommandRequest, tc loggingCommandRunnerCase) {
	t.Helper()
	if fields["event_name"] != workLogEventCommandRunnerCompleted {
		t.Fatalf("completion event_name = %#v, want %q", fields["event_name"], workLogEventCommandRunnerCompleted)
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

func assertLoggingCommandRunnerVerboseCompletionLog(t *testing.T, fields map[string]any, req CommandRequest) {
	t.Helper()
	if fields["event_name"] != workLogEventCommandRunnerOutputDetails {
		t.Fatalf("verbose completion event_name = %#v, want %q", fields["event_name"], workLogEventCommandRunnerOutputDetails)
	}
	if fields["command"] != req.Command {
		t.Fatalf("verbose completion command = %#v, want %q", fields["command"], req.Command)
	}
	if _, ok := fields["args_count"]; ok {
		t.Fatalf("verbose completion unexpectedly includes args_count = %#v", fields["args_count"])
	}
	if fields["stdout_bytes"] == nil || fields["stderr_bytes"] == nil {
		t.Fatalf("verbose completion missing output byte counters: %#v", fields)
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
	case "spawn-child-success":
		spawnCommandHelperChild()
		os.Exit(0)
	case "child-sleep":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "pid-sleep":
		if os.Getenv("COMMAND_HELPER_PID_WRITTEN_BY_PARENT") != "1" {
			writeCommandHelperPID()
		}
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "pid-term-exit":
		writeCommandHelperPID()
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGTERM)
		<-sigc
		os.Exit(0)
	case "pid-ignore-term":
		writeCommandHelperPID()
		signal.Ignore(syscall.SIGTERM)
		select {}
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
	if got, want := canonicalWorkerTestPath(cwd), canonicalWorkerTestPath(wantCWD); got != want {
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
		"child-sleep",
	)
	child.Env = append(os.Environ(),
		"GO_WANT_COMMAND_HELPER=1",
		"COMMAND_HELPER_PID_FILE="+pidFile,
		"COMMAND_HELPER_PID_WRITTEN_BY_PARENT=1",
	)
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start child: %v\n", err)
		os.Exit(2)
	}
	if err := writeCommandHelperPIDFile(pidFile, child.Process.Pid); err != nil {
		fmt.Fprintf(os.Stderr, "write child pid file: %v\n", err)
		_ = child.Process.Kill()
		os.Exit(2)
	}
}

func writeCommandHelperPID() {
	pidFile := os.Getenv("COMMAND_HELPER_PID_FILE")
	if pidFile == "" {
		fmt.Fprintln(os.Stderr, "missing COMMAND_HELPER_PID_FILE")
		os.Exit(2)
	}
	if err := writeCommandHelperPIDFile(pidFile, os.Getpid()); err != nil {
		fmt.Fprintf(os.Stderr, "write pid file: %v\n", err)
		os.Exit(2)
	}
}

func writeCommandHelperPIDFile(pidFile string, pid int) error {
	temporary := pidFile + ".tmp"
	if err := os.WriteFile(temporary, []byte(strconv.Itoa(pid)), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, pidFile)
}

func waitForCommandHelperPID(t *testing.T, pidFile string, timeout time.Duration) int {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for timeout <= 0 || time.Now().Before(deadline) {
		pid, err := readCommandHelperPIDFile(pidFile)
		if err == nil {
			return pid
		}
		lastErr = err
		if timeout <= 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("child pid file %s is empty", pidFile)
	}
	t.Fatalf("parse child pid from %s: %v", pidFile, lastErr)
	return 0
}

func readCommandHelperPIDFile(pidFile string) (int, error) {
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, err
	}
	if pid <= 0 {
		return 0, fmt.Errorf("child pid must be positive, got %d", pid)
	}
	return pid, nil
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
