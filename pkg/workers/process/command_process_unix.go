//go:build !windows

package process

import (
	"os/exec"
	"syscall"
	"time"
)

// commandProcessGroupGracePeriod is the default bounded wait after a graceful
// process-group signal before force-killing remaining members (post-run cleanup).
const commandProcessGroupGracePeriod = 2 * time.Second

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
func terminateCommandProcessGroup(cmd *exec.Cmd, grace time.Duration) error {
	if cmd.Process == nil {
		return nil
	}
	pgid := cmd.Process.Pid

	if grace > 0 {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		graceDeadline := time.Now().Add(grace)
		for time.Now().Before(graceDeadline) {
			if !commandProcessGroupRunning(pgid) {
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func terminateCommandProcessTree(cmd *exec.Cmd, _ *commandProcessTree) error {
	return terminateCommandProcessGroup(cmd, 0)
}

func closeCommandProcessTree(_ *commandProcessTree) {}

func commandProcessGroupRunning(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || err == syscall.EPERM
}
