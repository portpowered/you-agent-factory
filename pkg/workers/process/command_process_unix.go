//go:build !windows

package process

import (
	"os/exec"
	"syscall"
	"time"
)

type commandProcessTree struct{}

func configureCommandProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachCommandProcessTree(_ *exec.Cmd) (*commandProcessTree, error) {
	return nil, nil
}

// terminateCommandProcessGroup sends a graceful signal to the process group
// (-pgid), waits up to grace for members to exit, then SIGKILLs the group.
// When grace is zero, only the force-kill path runs so cancel/timeout behavior
// matches the prior immediate-SIGKILL semantics.
func terminateCommandProcessGroup(cmd *exec.Cmd, grace time.Duration, logCtx commandProcessCleanupContext) error {
	if cmd == nil || cmd.Process == nil {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, 0, nil, "process already exited")
		return nil
	}
	pgid := cmd.Process.Pid
	if !commandProcessGroupRunning(pgid) {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, pgid, nil, "process group not running")
		return nil
	}

	logCtx.logStarted(pgid)

	if grace > 0 {
		logCtx.logGraceful(pgid)
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		graceDeadline := time.Now().Add(grace)
		for time.Now().Before(graceDeadline) {
			if !commandProcessGroupRunning(pgid) {
				logCtx.logCompleted(commandProcessCleanupOutcomeGracefulSuccess, pgid, nil, "")
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	logCtx.logForceKill(pgid)
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			logCtx.logCompleted(commandProcessCleanupOutcomeFailure, pgid, killErr, "process group and parent kill failed")
			return killErr
		}
		logCtx.logCompleted(
			commandProcessCleanupOutcomePartialFailure,
			pgid,
			err,
			"process group kill failed; parent process killed",
		)
		return errCommandProcessCleanupPartialFailure
	}
	logCtx.logCompleted(commandProcessCleanupOutcomeForceKillSuccess, pgid, nil, "")
	return nil
}

func terminateCommandProcessTree(cmd *exec.Cmd, _ *commandProcessTree, logCtx commandProcessCleanupContext) error {
	return terminateCommandProcessGroup(cmd, 0, logCtx)
}

func closeCommandProcessTree(cmd *exec.Cmd, _ *commandProcessTree, logCtx commandProcessCleanupContext) {
	if cmd == nil {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, 0, nil, "missing command")
		return
	}
	_ = terminateCommandProcessGroup(cmd, postRunCleanupGracePeriod(), logCtx)
}

func commandProcessGroupRunning(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || err == syscall.EPERM
}
