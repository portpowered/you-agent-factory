//go:build !windows

package process

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

type commandProcessTree struct {
	pgid int
}

func configureCommandProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachCommandProcessTree(cmd *exec.Cmd) (*commandProcessTree, error) {
	if cmd == nil || cmd.Process == nil {
		return nil, nil
	}
	// configureCommandProcessTree uses Setpgid so the child becomes its own group leader.
	return &commandProcessTree{pgid: cmd.Process.Pid}, nil
}

// terminateCommandProcessGroup sends a graceful signal to the process group
// (-pgid), waits up to grace for members to exit, then SIGKILLs the group.
// When grace is zero, only the force-kill path runs so cancel/timeout behavior
// matches the prior immediate-SIGKILL semantics.
func commandProcessPGID(cmd *exec.Cmd, tree *commandProcessTree) int {
	if tree != nil && tree.pgid > 0 {
		return tree.pgid
	}
	if cmd == nil || cmd.Process == nil {
		return 0
	}
	return cmd.Process.Pid
}

func terminateCommandProcessGroup(cmd *exec.Cmd, tree *commandProcessTree, grace time.Duration, clock Clock, logCtx commandProcessCleanupContext) error {
	if cmd == nil || cmd.Process == nil {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, 0, nil, "process already exited")
		return nil
	}
	pgid := commandProcessPGID(cmd, tree)
	if pgid <= 0 {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, 0, nil, "missing process group")
		return nil
	}
	if !commandProcessGroupRunning(pgid) {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, pgid, nil, "process group not running")
		return nil
	}

	logCtx.logStarted(pgid)

	if grace > 0 {
		if clock == nil {
			return errors.New("platform process clock is required")
		}
		logCtx.logGraceful(pgid)
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		graceDeadline := clock.Now().Add(grace)
		for clock.Now().Before(graceDeadline) {
			if !commandProcessGroupRunning(pgid) || commandProcessLeaderExited(cmd) {
				logCtx.logCompleted(commandProcessCleanupOutcomeGracefulSuccess, pgid, nil, "")
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	logCtx.logForceKill(pgid)
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			logCtx.logCompleted(commandProcessCleanupOutcomeForceKillSuccess, pgid, nil, "")
			return nil
		}
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

func terminateCommandProcessTree(cmd *exec.Cmd, tree *commandProcessTree, clock Clock, logCtx commandProcessCleanupContext) error {
	return terminateCommandProcessGroup(cmd, tree, 0, clock, logCtx)
}

func closeCommandProcessTree(cmd *exec.Cmd, tree *commandProcessTree, clock Clock, logCtx commandProcessCleanupContext) {
	if cmd == nil {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, 0, nil, "missing command")
		return
	}
	_ = terminateCommandProcessGroup(cmd, tree, postRunCleanupGracePeriod(), clock, logCtx)
}

func commandProcessGroupRunning(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	if err := syscall.Kill(-pgid, 0); err == nil || err == syscall.EPERM {
		return true
	}
	// Darwin often rejects signal 0 to -pgid for a lone Setpgid leader; probe the leader PID.
	err := syscall.Kill(pgid, 0)
	return err == nil || err == syscall.EPERM
}

func commandProcessLeaderExited(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return true
	}
	var status syscall.WaitStatus
	pid, err := syscall.Wait4(cmd.Process.Pid, &status, syscall.WNOHANG, nil)
	return err == nil && pid > 0
}
