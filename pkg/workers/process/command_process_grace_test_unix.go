//go:build !windows

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestTerminateCommandProcessGroup_GracefulChildExit(t *testing.T) {
	const testGrace = 300 * time.Millisecond

	pidFile := filepath.Join(t.TempDir(), "graceful.pid")
	cmd := startCommandHelperInProcessGroup(t, "pid-term-exit", pidFile)
	childPID := readCommandHelperPID(t, pidFile)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})

	logger := &recordingCommandLogger{}
	logCtx := newCommandProcessCleanupContext(logger, CommandRequest{}, commandProcessCleanupReasonPostRun)
	if err := terminateCommandProcessGroup(cmd, testGrace, logCtx); err != nil {
		t.Fatalf("terminateCommandProcessGroup returned error: %v", err)
	}

	assertCommandProcessCleanupOutcome(t, logger, commandProcessCleanupOutcomeGracefulSuccess)
	if !waitForCommandHelperProcessExit(childPID, time.Second) {
		t.Fatalf("child process %d still running after graceful cleanup", childPID)
	}
}

func TestTerminateCommandProcessGroup_ForceKillAfterGrace(t *testing.T) {
	const testGrace = 150 * time.Millisecond

	pidFile := filepath.Join(t.TempDir(), "force.pid")
	cmd := startCommandHelperInProcessGroup(t, "pid-ignore-term", pidFile)
	childPID := readCommandHelperPID(t, pidFile)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})

	logger := &recordingCommandLogger{}
	logCtx := newCommandProcessCleanupContext(logger, CommandRequest{}, commandProcessCleanupReasonPostRun)
	started := time.Now()
	if err := terminateCommandProcessGroup(cmd, testGrace, logCtx); err != nil {
		t.Fatalf("terminateCommandProcessGroup returned error: %v", err)
	}
	elapsed := time.Since(started)
	if elapsed < testGrace {
		t.Fatalf("cleanup returned in %v, want at least grace period %v before force kill", elapsed, testGrace)
	}
	if elapsed > testGrace+2*time.Second {
		t.Fatalf("cleanup took %v, want bounded wait near grace %v", elapsed, testGrace)
	}

	assertCommandProcessCleanupOutcome(t, logger, commandProcessCleanupOutcomeForceKillSuccess)
	if !waitForCommandHelperProcessExit(childPID, time.Second) {
		t.Fatalf("child process %d still running after force cleanup", childPID)
	}
}

func startCommandHelperInProcessGroup(t *testing.T, mode, pidFile string) *exec.Cmd {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), os.Args[0],
		"-test.run=TestExecCommandRunner_HelperProcess",
		"--",
		mode,
	)
	cmd.Env = append(os.Environ(),
		"GO_WANT_COMMAND_HELPER=1",
		"COMMAND_HELPER_PID_FILE="+pidFile,
	)
	configureCommandProcessTree(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			commandTestTerminateProcess(cmd.Process.Pid)
		}
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			return cmd
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper did not write pid file")
	return nil
}

func assertCommandProcessCleanupOutcome(t *testing.T, logger *recordingCommandLogger, want commandProcessCleanupOutcome) {
	t.Helper()

	completed := commandCleanupCompletedLogs(logger)
	if len(completed) == 0 {
		t.Fatal("expected cleanup completion log")
	}
	last := completed[len(completed)-1]
	if last.fields["outcome"] != string(want) {
		t.Fatalf("cleanup outcome = %#v, want %q", last.fields["outcome"], want)
	}
}
