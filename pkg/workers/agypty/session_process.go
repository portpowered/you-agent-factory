package agypty

import (
	"os/exec"

	"github.com/portpowered/infinite-you/pkg/workers/process"
)

type sessionProcess struct {
	cmd       *exec.Cmd
	tree      process.SubprocessTree
	winHandle uintptr
	exitCode  int
}

func (p *sessionProcess) Terminate() error {
	if p == nil || p.cmd == nil {
		return nil
	}
	return process.TerminateSubprocessTree(p.cmd, p.tree)
}

func (p *sessionProcess) Close() {
	if p == nil || p.cmd == nil {
		return
	}
	process.CloseSubprocessTree(p.cmd, p.tree)
}

func (p *sessionProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func exitCodeFromWait(waitErr error, proc *sessionProcess) int {
	if waitErr != nil {
		return -1
	}
	if proc != nil {
		return proc.exitCode
	}
	return 0
}
