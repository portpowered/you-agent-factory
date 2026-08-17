package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

// commandHelperSpawnTimeoutBudget allows slow CI hosts (especially Windows) to
// start the helper, spawn the child, and write the pid file before the test
// context deadline fires. spawn-child sleeps 10s after spawning.
//
// This bounds a failure, so it only costs wall time when a guard is already
// broken: budgeting generously is free. A tighter 3s budget expired against
// nothing worse than two Windows process creations under package load, which
// surfaced as a missing pid file in whichever guard happened to run then.
const commandHelperSpawnTimeoutBudget = 20 * time.Second

// commandHelperInterruptionBudget bounds how long the guards that interrupt a
// run let it proceed before their deadline fires. Unlike the budget above this
// one is spent on every pass, so it trades wall time for headroom, and it is
// squeezed from both sides: it has to outlast two process creations (the
// helper, then the helper's own child) plus the helper's fixed pre-spawn pause,
// or the run is torn down before the descendant it is meant to interrupt ever
// exists -- yet stay under the 10s the helper sleeps, or the run ends on its
// own and no deadline is ever observed.
const commandHelperInterruptionBudget = 4 * time.Second

// commandHelperDelayedSideEffectDelay is how long an escaped descendant waits
// before producing its side effect. Keeping it above
// commandHelperInterruptionBudget makes the guard's ordering unconditional: the
// descendant starts no earlier than the run, so its side effect is always still
// pending when the interruption lands, on any host and under any load.
const commandHelperDelayedSideEffectDelay = 6 * time.Second

func requireProcessIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("real child-process integration")
	}
}

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
	errors   []recordedCommandLog
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
func (l *recordingCommandLogger) Error(msg string, keysAndValues ...any) {
	l.errors = append(l.errors, recordedCommandLog{
		msg:    msg,
		fields: commandLogFieldsMap(keysAndValues...),
	})
}
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

func testExecCommandRunner(t testing.TB, logger logging.Logger) ExecCommandRunner {
	t.Helper()
	runner, err := NewExecCommandRunner(exec.Command, platformclock.Real{}, logger, nil)
	if err != nil {
		t.Fatalf("NewExecCommandRunner() error = %v", err)
	}
	return runner
}

type loggingCommandRunnerCase struct {
	name            string
	result          CommandResult
	err             error
	wantStatus      string
	wantCommandData bool
}

func TestExecCommandRunner_SuccessfulProcessCapturesOutputAndInputs(t *testing.T) {
	requireProcessIntegration(t)
	workDir := t.TempDir()
	result, err := testExecCommandRunner(t, nil).Run(context.Background(), CommandRequest{
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
	requireProcessIntegration(t)
	result, err := testExecCommandRunner(t, nil).Run(context.Background(), CommandRequest{
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
	requireProcessIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	result, err := testExecCommandRunner(t, nil).Run(ctx, CommandRequest{
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
	requireProcessIntegration(t)
	testExecCommandRunnerAgentStyleSuccessLeavesNoChildProcess(t)
}

// TestExecCommandRunner_AgentStyleSuccessLeavesNoChildProcess is the US-005 regression
// guard: a command that spawns a background service and exits 0 must not leave the child
// running after Run returns. Uses commandTestProcessRunning from command_process_test_unix.go
// or command_process_test_windows.go for platform liveness checks.
func TestExecCommandRunner_AgentStyleSuccessLeavesNoChildProcess(t *testing.T) {
	requireProcessIntegration(t)
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
	result, err := testExecCommandRunner(t, nil).Run(context.Background(), CommandRequest{
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

// orphanedOutputPipeTestGrace is deliberately far below the descendant helper's
// ten-second lifetime so a passing run can only mean the wait ended on the
// started process exiting, never on the inherited output pipe closing.
const orphanedOutputPipeTestGrace = 300 * time.Millisecond

func useShortOrphanedOutputPipeGrace(t *testing.T) time.Duration {
	t.Helper()
	orphanedOutputPipeGracePeriodForTest = orphanedOutputPipeTestGrace
	postRunCleanupGracePeriodForTest = 250 * time.Millisecond
	t.Cleanup(func() {
		orphanedOutputPipeGracePeriodForTest = 0
		postRunCleanupGracePeriodForTest = 0
	})
	// Both graces plus generous slack for a loaded CI host. The guard only has
	// to separate a bounded return from the descendant's ten-second lifetime.
	return 5 * time.Second
}

// TestExecCommandRunner_ExitedProcessEndsRunWhileDescendantHoldsOutputPipe
// guards the wait boundary that left a worker session RUNNING for 21+ minutes
// with its process already dead. cmd.Wait joins the os/exec output-copy
// goroutines after reaping the process, so a descendant that inherited the
// stdout/stderr write end used to keep Run blocked for as long as that
// descendant lived -- and the work dispatch stayed active behind it. Run must
// end on the started process exiting, not on the pipe closing.
func TestExecCommandRunner_ExitedProcessEndsRunWhileDescendantHoldsOutputPipe(t *testing.T) {
	requireProcessIntegration(t)
	bound := useShortOrphanedOutputPipeGrace(t)

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	started := time.Now()
	result, err := testExecCommandRunner(t, nil).Run(context.Background(), CommandRequest{
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
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("Run() error = %v, want nil for a command that exited 0", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; the exit status survives the wait delay", result.ExitCode)
	}
	if elapsed > bound {
		t.Fatalf("Run() took %v, want a bounded return under %v instead of waiting out the descendant", elapsed, bound)
	}
	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() { commandTestTerminateProcess(childPID) })
}

// TestExecCommandRunner_KilledProcessEndsRunWhileDescendantHoldsOutputPipe is
// the reported incident: the operator kills the worker's process and the run
// never ends because a surviving descendant holds the output pipe. The escaped
// helper child never exits on its own, so before the wait delay this Run could
// only be released by an outer execution timeout. The observer here records the
// process and takes no action, proving the runner bounds its own wait rather
// than depending on a caller turning the exit into a cancellation.
func TestExecCommandRunner_KilledProcessEndsRunWhileDescendantHoldsOutputPipe(t *testing.T) {
	requireProcessIntegration(t)
	bound := useShortOrphanedOutputPipeGrace(t)

	pidFile := filepath.Join(t.TempDir(), "escaped-child.pid")
	observer := &lifecycleObserverRecorder{
		started: make(chan ProcessInfo, 1),
		exited:  make(chan ProcessInfo, 1),
	}
	type runOutcome struct {
		result  CommandResult
		err     error
		elapsed time.Duration
	}
	runDone := make(chan runOutcome, 1)
	go func() {
		startedAt := time.Now()
		result, err := testExecCommandRunner(t, nil).Run(context.Background(), CommandRequest{
			Command: os.Args[0],
			Args: []string{
				"-test.run=TestExecCommandRunner_HelperProcess",
				"--",
				"spawn-child-orphan-pipe",
			},
			Env: append(os.Environ(),
				"GO_WANT_COMMAND_HELPER=1",
				"COMMAND_HELPER_PID_FILE="+pidFile,
			),
			ProcessLifecycleObserver: observer,
		})
		runDone <- runOutcome{result: result, err: err, elapsed: time.Since(startedAt)}
	}()

	var leader ProcessInfo
	select {
	case leader = <-observer.started:
	case <-time.After(commandHelperSpawnTimeoutBudget):
		t.Fatal("command runner did not report the started process")
	}
	// Wait for the escaped descendant before killing the leader, so the run is
	// genuinely blocked on an inherited pipe rather than reaching a clean EOF.
	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() { commandTestTerminateProcess(childPID) })
	commandTestTerminateProcess(leader.PID)

	select {
	case outcome := <-runDone:
		if errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("Run() error = %v, want a process-exit outcome distinguishable from cancellation", outcome.err)
		}
		if outcome.err != nil {
			t.Fatalf("Run() error = %v, want the killed process reported through its exit status", outcome.err)
		}
		if outcome.result.ExitCode == 0 {
			t.Fatal("ExitCode = 0, want the killed process reported as a failing exit status")
		}
		if outcome.elapsed > bound {
			t.Fatalf("Run() took %v, want a bounded return under %v after the process was killed", outcome.elapsed, bound)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run() never returned after its process was killed while a descendant held the output pipe open")
	}
}

func TestExecCommandRunner_ContextDeadlineTerminatesSpawnedChildProcess(t *testing.T) {
	requireProcessIntegration(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), commandHelperInterruptionBudget)
	defer cancel()

	result, err := testExecCommandRunner(t, nil).Run(ctx, CommandRequest{
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
	requireProcessIntegration(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runOutcome struct {
		result CommandResult
		err    error
	}
	runDone := make(chan runOutcome, 1)
	go func() {
		result, err := testExecCommandRunner(t, nil).Run(ctx, CommandRequest{
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

func TestExecCommandRunner_InterruptionPreventsDelayedDescendantSideEffect(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		assertInterruptionPreventsDelayedSideEffect(t, false)
	})
	t.Run("deadline", func(t *testing.T) {
		assertInterruptionPreventsDelayedSideEffect(t, true)
	})
}

func assertInterruptionPreventsDelayedSideEffect(t *testing.T, deadline bool) {
	t.Helper()
	requireProcessIntegration(t)
	tempDir := t.TempDir()
	pidFile := filepath.Join(tempDir, "child.pid")
	sideEffectFile := filepath.Join(tempDir, "delayed-side-effect")

	ctx, cancel := context.WithCancel(context.Background())
	if deadline {
		ctx, cancel = context.WithTimeout(context.Background(), commandHelperInterruptionBudget)
	}
	defer cancel()
	runDone := make(chan error, 1)
	go func() {
		_, err := testExecCommandRunner(t, nil).Run(ctx, CommandRequest{
			Command: os.Args[0],
			Args: []string{
				"-test.run=TestExecCommandRunner_HelperProcess",
				"--",
				"spawn-child-side-effect",
			},
			Env: append(os.Environ(),
				"GO_WANT_COMMAND_HELPER=1",
				"COMMAND_HELPER_PID_FILE="+pidFile,
				"COMMAND_HELPER_SIDE_EFFECT_FILE="+sideEffectFile,
			),
		})
		runDone <- err
	}()

	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})
	if !deadline {
		cancel()
	}
	err := <-runDone
	wantErr := context.Canceled
	if deadline {
		wantErr = context.DeadlineExceeded
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run error = %v, want %v", err, wantErr)
	}
	// Run returned no earlier than the descendant started, so waiting out the
	// descendant's whole delay always outlasts the side effect it was going to
	// produce. A shorter fixed wait would only cover a fast spawn.
	time.Sleep(commandHelperDelayedSideEffectDelay)
	if _, statErr := os.Stat(sideEffectFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("delayed descendant side effect exists after Run returned: %v", statErr)
	}
}

func TestExecCommandRunner_PostRunCleanupWaitsForParentWait(t *testing.T) {
	requireProcessIntegration(t)
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group supervision timing; Windows uses job-object post-run path")
	}

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_, _ = testExecCommandRunner(t, nil).Run(ctx, CommandRequest{
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
	requireProcessIntegration(t)
	logger := &recordingCommandLogger{}
	req := commandCleanupTestRequest(t)
	_, err := testExecCommandRunner(t, logger).Run(context.Background(), req)
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
	requireProcessIntegration(t)
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
		_, err := testExecCommandRunner(t, logger).Run(ctx, req)
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

func commandCleanupTestRequest(t *testing.T) CommandRequest {
	t.Helper()
	return CommandRequest{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestExecCommandRunner_HelperProcess", "--", "success"},
		Env:     append(os.Environ(), "GO_WANT_COMMAND_HELPER=1"),
		WorkDir: t.TempDir(),
	}
}

func commandCleanupCompletedLogs(logger *recordingCommandLogger) []recordedCommandLog {
	return commandCleanupLogsByEvent(logger, "command_runner.cleanup_completed")
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
	if fields["cleanup_reason"] != string(reason) {
		t.Fatalf("cleanup_reason = %#v, want %q", fields["cleanup_reason"], reason)
	}
	if fields["args_count"] != len(req.Args) {
		t.Fatalf("args_count = %#v, want %d", fields["args_count"], len(req.Args))
	}
	if req.WorkDir != "" && fields["working_dir"] != req.WorkDir {
		t.Fatalf("working_dir = %#v, want %q", fields["working_dir"], req.WorkDir)
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
		Clock:  platformclock.Real{},
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

func TestComposedCommandLineLength_MeasuresTheStringTheProcessLoaderReceives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		args    []string
		want    int
	}{
		{name: "bare command", command: "claude", want: len("claude")},
		{
			name:    "plain arguments join with single separators",
			command: "claude",
			args:    []string{"-p", "--verbose"},
			want:    len(`claude -p --verbose`),
		},
		{
			name:    "arguments containing spaces gain surrounding quotes",
			command: "claude",
			args:    []string{"--system-prompt", "be brief"},
			want:    len(`claude --system-prompt "be brief"`),
		},
		{
			name:    "embedded quotes gain an escaping backslash",
			command: "claude",
			args:    []string{`say "hi"`},
			want:    len(`claude "say \"hi\""`),
		},
		{
			name:    "trailing backslashes double before the closing quote",
			command: "claude",
			args:    []string{`C:\work dir\`},
			want:    len(`claude "C:\work dir\\"`),
		},
		{
			name:    "empty arguments are emitted as an empty quoted pair",
			command: "claude",
			args:    []string{""},
			want:    len(`claude ""`),
		},
		{
			name:    "characters outside the basic multilingual plane cost two code units",
			command: "claude",
			args:    []string{"\U0001F600"},
			want:    len("claude ") + 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ComposedCommandLineLength(tc.command, tc.args); got != tc.want {
				t.Fatalf("ComposedCommandLineLength(%q, %#v) = %d, want %d", tc.command, tc.args, got, tc.want)
			}
		})
	}
}

func TestComposedCommandLineLength_GrowsWithInlinedArgumentContent(t *testing.T) {
	t.Parallel()

	small := ComposedCommandLineLength("claude", []string{"-p", strings.Repeat("a", 9152)})
	large := ComposedCommandLineLength("claude", []string{"-p", strings.Repeat("a", 9819)})
	if large-small != 9819-9152 {
		t.Fatalf("inline argument growth = %d, want %d", large-small, 9819-9152)
	}

	viaStdin := ComposedCommandLineLength("claude", []string{"-p"})
	if viaStdin >= small {
		t.Fatalf("command line with the prompt removed = %d, want below the inline measurement %d", viaStdin, small)
	}
}

func TestCommandStartError_NamesAnOversizedCommandLineWithItsMeasuredSize(t *testing.T) {
	t.Parallel()

	overLimit := &CommandStartError{
		Command:           "claude",
		ArgsCount:         12,
		CommandLineLength: 33012,
		CommandLineLimit:  WindowsCommandLineLimit,
		Cause:             errors.New("The filename or extension is too long."),
	}
	if !overLimit.OverCommandLineLimit() {
		t.Fatalf("OverCommandLineLimit() = false, want true for %d against %d", overLimit.CommandLineLength, overLimit.CommandLineLimit)
	}
	message := overLimit.Error()
	for _, want := range []string{"claude", "33012", "12", "32767", "command-line limit", "The filename or extension is too long."} {
		if !strings.Contains(message, want) {
			t.Fatalf("Error() = %q, want it to name %q", message, want)
		}
	}

	underLimit := &CommandStartError{
		Command:           "claude",
		ArgsCount:         3,
		CommandLineLength: 64,
		CommandLineLimit:  WindowsCommandLineLimit,
		Cause:             errors.New("executable file not found"),
	}
	if underLimit.OverCommandLineLimit() {
		t.Fatalf("OverCommandLineLimit() = true, want false for %d against %d", underLimit.CommandLineLength, underLimit.CommandLineLimit)
	}
	if got := underLimit.Error(); strings.Contains(got, "command-line limit") {
		t.Fatalf("Error() = %q, want it not to blame the command-line limit", got)
	}

	unbounded := &CommandStartError{CommandLineLength: 1 << 20, CommandLineLimit: 0, Cause: errors.New("boom")}
	if unbounded.OverCommandLineLimit() {
		t.Fatalf("OverCommandLineLimit() = true, want false when the host states no command-line limit")
	}
}

func TestCommandStartError_UnwrapsTheOperatingSystemCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("fork/exec failed")
	err := error(&CommandStartError{Command: "claude", Cause: cause})
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(%v, cause) = false, want true", err)
	}
}

func TestExecCommandRunner_StartFailureReturnsANamedErrorAndLogsIt(t *testing.T) {
	logger := &recordingCommandLogger{}
	missing := filepath.Join(t.TempDir(), "provider-executable-that-does-not-exist")
	req := CommandRequest{
		Command: missing,
		Args:    []string{"-p", strings.Repeat("prompt ", 512)},
		Stdin:   []byte("stdin payload"),
		WorkDir: t.TempDir(),
	}

	_, err := testExecCommandRunner(t, logger).Run(context.Background(), req)

	var startErr *CommandStartError
	if !errors.As(err, &startErr) {
		t.Fatalf("Run() error = %#v, want a *CommandStartError", err)
	}
	if startErr.Command != missing {
		t.Fatalf("start error command = %q, want %q", startErr.Command, missing)
	}
	if want := ComposedCommandLineLength(req.Command, req.Args); startErr.CommandLineLength != want {
		t.Fatalf("start error command line length = %d, want %d", startErr.CommandLineLength, want)
	}
	if startErr.ArgsCount != len(req.Args) || startErr.StdinBytes != len(req.Stdin) {
		t.Fatalf("start error = %#v, want args %d and stdin %d", startErr, len(req.Args), len(req.Stdin))
	}
	if startErr.Cause == nil {
		t.Fatalf("start error cause = nil, want the operating system failure")
	}

	logged := commandStartFailureLogs(logger)
	if len(logged) != 1 {
		t.Fatalf("start failure logs = %d, want exactly 1: %#v", len(logged), logger.errors)
	}
	fields := logged[0].fields
	if fields["command"] != missing {
		t.Fatalf("start failure log command = %#v, want %q", fields["command"], missing)
	}
	if fields["command_line_chars"] != startErr.CommandLineLength {
		t.Fatalf("start failure log command_line_chars = %#v, want %d", fields["command_line_chars"], startErr.CommandLineLength)
	}
	if fields["over_command_line_limit"] != startErr.OverCommandLineLimit() {
		t.Fatalf("start failure log over_command_line_limit = %#v, want %v", fields["over_command_line_limit"], startErr.OverCommandLineLimit())
	}
	if fields["error"] == nil || fields["error"] == "" {
		t.Fatalf("start failure log error = %#v, want the operating system failure text", fields["error"])
	}
}

// TestExecCommandRunner_StartFailureUsesTheInjectedCommandLineLimit pins the
// classification to the bound injected into the runner rather than to the
// operating system the test happens to run on. That is what keeps the Windows
// over-limit case observable from a non-Windows CI host, which was impossible
// while the limit was computed from the ambient runtime.
func TestExecCommandRunner_StartFailureUsesTheInjectedCommandLineLimit(t *testing.T) {
	logger := &recordingCommandLogger{}
	runner := testExecCommandRunner(t, logger)
	runner.CommandLineLimit = WindowsCommandLineLimit

	_, err := runner.Run(context.Background(), CommandRequest{
		Command: filepath.Join(t.TempDir(), "provider-executable-that-does-not-exist"),
		Args:    []string{"--system-prompt", strings.Repeat("s", 20_000), strings.Repeat("p", 13_000)},
	})

	var startErr *CommandStartError
	if !errors.As(err, &startErr) {
		t.Fatalf("Run() error = %#v, want a *CommandStartError", err)
	}
	if startErr.CommandLineLimit != WindowsCommandLineLimit || !startErr.OverCommandLineLimit() {
		t.Fatalf("start error limit = %d and over-limit = %v, want the injected %d and true",
			startErr.CommandLineLimit, startErr.OverCommandLineLimit(), WindowsCommandLineLimit)
	}
	logged := commandStartFailureLogs(logger)
	if len(logged) != 1 || logged[0].fields["command_line_limit"] != WindowsCommandLineLimit {
		t.Fatalf("start failure logs = %#v, want exactly one carrying the injected limit %d", logged, WindowsCommandLineLimit)
	}
}

func TestExecCommandRunner_SuccessfulStartLogsNoStartFailure(t *testing.T) {
	requireProcessIntegration(t)
	logger := &recordingCommandLogger{}

	if _, err := testExecCommandRunner(t, logger).Run(context.Background(), commandCleanupTestRequest(t)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if logged := commandStartFailureLogs(logger); len(logged) != 0 {
		t.Fatalf("start failure logs = %#v, want none for a command that started", logged)
	}
}

func commandStartFailureLogs(logger *recordingCommandLogger) []recordedCommandLog {
	var matched []recordedCommandLog
	for _, entry := range logger.errors {
		if entry.fields["event_name"] == commandRunnerStartFailedEvent {
			matched = append(matched, entry)
		}
	}
	return matched
}
