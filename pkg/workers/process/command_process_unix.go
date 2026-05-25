//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

type commandProcessTree struct{}

func configureCommandProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachCommandProcessTree(_ *exec.Cmd) (*commandProcessTree, error) {
	return nil, nil
}

func terminateCommandProcessTree(cmd *exec.Cmd, _ *commandProcessTree) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func closeCommandProcessTree(_ *commandProcessTree) {}
