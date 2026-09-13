//go:build !windows

package tts_clean_install

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type processTree struct {
	pgid int
}

func configureProcessCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func newProcessTree() (processTree, error) {
	return processTree{}, nil
}

func (tree *processTree) attach(process *os.Process) error {
	if process == nil || process.Pid <= 0 {
		return errors.New("process has no attachable pid")
	}
	tree.pgid = process.Pid
	return nil
}

func (tree *processTree) terminate(process *os.Process) error {
	if tree != nil && tree.pgid > 0 {
		if err := syscall.Kill(-tree.pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
	if process == nil {
		return nil
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func (tree *processTree) active() bool {
	return tree != nil && tree.pgid > 0 && processAlive(tree.pgid)
}

func (tree *processTree) close() {
	if tree != nil {
		tree.pgid = 0
	}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
