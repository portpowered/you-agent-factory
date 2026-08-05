//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package realclient_test

import (
	"errors"
	"os/exec"
	"syscall"
)

type commandProcessTree struct {
	pgid int
}

func configureCommandProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachCommandProcessTree(command *exec.Cmd) (*commandProcessTree, error) {
	if command.Process == nil || command.Process.Pid <= 0 {
		return nil, nil
	}
	return &commandProcessTree{pgid: command.Process.Pid}, nil
}

func closeCommandProcessTree(command *exec.Cmd, tree *commandProcessTree) {
	_ = terminateCommandProcessTree(command, tree)
}

func terminateCommandProcessTree(command *exec.Cmd, tree *commandProcessTree) error {
	if tree != nil && tree.pgid > 0 {
		err := syscall.Kill(-tree.pgid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	if command.Process == nil || command.Process.Pid <= 0 {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func processHasExited(pid int) (bool, error) {
	err := syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return false, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return true, nil
	}
	return false, err
}
