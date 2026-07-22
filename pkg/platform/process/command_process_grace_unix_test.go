//go:build !windows

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
)

func TestTerminateCommandProcessGroup_GracefulChildExit(t *testing.T) {
	// Use a generous grace so -race builds still observe graceful exit before force-kill.
	const testGrace = 2 * time.Second

	pidFile := filepath.Join(t.TempDir(), "graceful.pid")
	cmd, tree := startCommandHelperInProcessGroup(t, "pid-term-exit", pidFile)
	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})

	logger := &recordingCommandLogger{}
	logCtx := newCommandProcessCleanupContext(logger, CommandRequest{}, commandProcessCleanupReasonPostRun)
	if err := terminateCommandProcessGroup(cmd, tree, testGrace, platformclock.Real{}, logCtx); err != nil {
		t.Fatalf("terminateCommandProcessGroup returned error: %v", err)
	}

	assertCommandProcessCleanupOutcome(t, logger, commandProcessCleanupOutcomeGracefulSuccess)
	reapCommandHelperLeader(t, cmd)
	if !waitForCommandHelperProcessExit(childPID, 3*time.Second) {
		t.Fatalf("child process %d still running after graceful cleanup", childPID)
	}
}

func TestTerminateCommandProcessGroup_ForceKillAfterGrace(t *testing.T) {
	if runtime.GOOS == "darwin" {
		// Darwin delivers SIGTERM to the process group even when the leader called signal.Ignore.
		t.Skip("force-kill-after-grace timing is validated on Linux CI")
	}

	const testGrace = 150 * time.Millisecond

	pidFile := filepath.Join(t.TempDir(), "force.pid")
	cmd, tree := startCommandHelperInProcessGroup(t, "pid-ignore-term", pidFile)
	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})
	logger := &recordingCommandLogger{}
	logCtx := newCommandProcessCleanupContext(logger, CommandRequest{}, commandProcessCleanupReasonPostRun)
	started := time.Now()
	if err := terminateCommandProcessGroup(cmd, tree, testGrace, platformclock.Real{}, logCtx); err != nil {
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
	reapCommandHelperLeader(t, cmd)
	if !waitForCommandHelperProcessExit(childPID, 3*time.Second) {
		t.Fatalf("child process %d still running after force cleanup", childPID)
	}
}

func startCommandHelperInProcessGroup(t *testing.T, mode, pidFile string) (*exec.Cmd, *commandProcessTree) {
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
	tree, err := attachCommandProcessTree(cmd)
	if err != nil {
		t.Fatalf("attach process tree: %v", err)
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
			return cmd, tree
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper did not write pid file")
	return nil, nil
}

// reapCommandHelperLeader waits for the helper started by startCommandHelperInProcessGroup
// so zombies are not mistaken for still-running processes by Kill(pid, 0) liveness probes.
func reapCommandHelperLeader(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out reaping helper process %d", cmd.Process.Pid)
	}
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
