//go:build linux || darwin

package pty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"

	creackpty "github.com/creack/pty"
	"github.com/portpowered/infinite-you/pkg/platform/process"
)

type posixPTYOpener func() (master, slave *os.File, err error)

// POSIXHost owns only native POSIX PTY handles and subprocess mechanics.
type POSIXHost struct {
	Open posixPTYOpener
}

// NewHost returns the policy-free PTY host compiled for this platform.
func NewHost() Host { return &POSIXHost{Open: creackpty.Open} }

// Allocate opens an opaque POSIX master/slave pair.
func (h *POSIXHost) Allocate(ctx context.Context) (Allocation, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if h == nil {
		return nil, errors.New("pty: POSIX host is required")
	}
	opener := h.Open
	if opener == nil {
		return nil, errors.New("pty: POSIX opener is required")
	}
	master, slave, err := opener()
	if err != nil {
		return nil, err
	}
	return &posixPTYAllocation{master: master, slave: slave}, nil
}

type posixPTYAllocation struct {
	master *os.File
	slave  *os.File
}

func (*posixPTYAllocation) Kind() Kind { return KindPOSIX }

func (p *posixPTYAllocation) Close() error {
	if p == nil {
		return nil
	}
	var firstErr error
	if p.master != nil {
		if err := p.master.Close(); err != nil {
			firstErr = err
		}
		p.master = nil
	}
	if p.slave != nil {
		if err := p.slave.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.slave = nil
	}
	return firstErr
}

func (p *posixPTYAllocation) Master() *os.File {
	if p == nil {
		return nil
	}
	return p.master
}

func (p *posixPTYAllocation) Slave() *os.File {
	if p == nil {
		return nil
	}
	return p.slave
}

// Start attaches the supplied launch description to the allocated TTY.
func (h *POSIXHost) Start(launch ProcessLaunch, native Allocation) (Process, io.ReadCloser, error) {
	alloc, ok := native.(*posixPTYAllocation)
	if !ok || alloc == nil || alloc.Slave() == nil || alloc.Master() == nil {
		return nil, nil, errors.New("pty: POSIX PTY handles are required")
	}
	if len(launch.Argv) == 0 {
		return nil, nil, errors.New("pty: argv is required")
	}
	cmd := exec.Command(launch.Argv[0], launch.Argv[1:]...)
	cmd.Dir = launch.WorkDir
	if len(launch.Env) > 0 {
		cmd.Env = launch.Env
	}
	cmd.Stdout, cmd.Stdin, cmd.Stderr = alloc.slave, alloc.slave, alloc.slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	process.ConfigureSubprocessTree(cmd)
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	_ = alloc.slave.Close()
	alloc.slave = nil
	tree, err := process.AttachSubprocessTree(cmd)
	if err != nil {
		_ = process.TerminateSubprocessTree(cmd, tree)
		_ = cmd.Wait()
		return nil, nil, err
	}
	return &posixProcess{cmd: cmd, tree: tree}, alloc.master, nil
}

type posixProcess struct {
	cmd      *exec.Cmd
	tree     process.SubprocessTree
	exitCode int
}

func (p *posixProcess) Wait() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return errors.New("pty: process is not started")
	}
	err := p.cmd.Wait()
	if err == nil {
		p.exitCode = 0
		return nil
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		p.exitCode = exitErr.ExitCode()
		return nil
	}
	return err
}
func (p *posixProcess) Terminate() error {
	if p == nil || p.cmd == nil {
		return nil
	}
	return process.TerminateSubprocessTree(p.cmd, p.tree)
}
func (p *posixProcess) Close() {
	if p != nil && p.cmd != nil {
		process.CloseSubprocessTree(p.cmd, p.tree)
	}
}
func (p *posixProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}
func (p *posixProcess) ExitCode() int {
	if p == nil {
		return -1
	}
	return p.exitCode
}
