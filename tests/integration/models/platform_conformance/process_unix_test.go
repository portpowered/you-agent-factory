//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package platform_conformance

import (
	"errors"
	"os/exec"
	"syscall"
)

// controlledProcessTree is one process group created exclusively for a
// controlled attempt. Negative-pgid signals cannot reach an unrelated peer
// process because the group is made before the child starts.
type controlledProcessTree struct {
	pgid int
}

func configureControlledProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachControlledProcessTree(command *exec.Cmd) (controlledProcessTree, error) {
	if command == nil || command.Process == nil || command.Process.Pid <= 0 {
		return controlledProcessTree{}, errors.New("controlled process has no attachable pid")
	}
	return controlledProcessTree{pgid: command.Process.Pid}, nil
}

func terminateControlledProcessTree(command *exec.Cmd, tree controlledProcessTree) error {
	pgid := tree.pgid
	if pgid <= 0 && command != nil && command.Process != nil {
		pgid = command.Process.Pid
	}
	if pgid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func requestControlledProcessTreeStop(command *exec.Cmd, tree controlledProcessTree) error {
	pgid := tree.pgid
	if pgid <= 0 && command != nil && command.Process != nil {
		pgid = command.Process.Pid
	}
	if pgid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pgid, syscall.SIGINT); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func closeControlledProcessTree(controlledProcessTree, bool) {}

func controlledProcessTreeAlive(tree controlledProcessTree) bool {
	if tree.pgid <= 0 {
		return false
	}
	err := syscall.Kill(-tree.pgid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
