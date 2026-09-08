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
	return signalControlledProcessGroup(command, tree, syscall.SIGKILL)
}

func requestControlledProcessTreeStop(command *exec.Cmd, tree controlledProcessTree) error {
	return signalControlledProcessGroup(command, tree, syscall.SIGINT)
}

func signalControlledProcessGroup(command *exec.Cmd, tree controlledProcessTree, signal syscall.Signal) error {
	pgid := tree.pgid
	if pgid <= 0 && command != nil && command.Process != nil {
		pgid = command.Process.Pid
	}
	if pgid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pgid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func closeControlledProcessTree(controlledProcessTree, bool) {}

func controlledProcessTreeAlive(tree controlledProcessTree) bool {
	err := probeControlledProcessGroup(tree)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// probeControlledProcessGroup sends only the read-only zero signal to the
// attached PGID. It deliberately has no fallback to the direct PID, so
// release evidence cannot widen ownership to an unrelated process.
func probeControlledProcessGroup(tree controlledProcessTree) error {
	if tree.pgid <= 0 {
		return syscall.ESRCH
	}
	return syscall.Kill(-tree.pgid, 0)
}
