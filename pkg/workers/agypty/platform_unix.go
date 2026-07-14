//go:build linux || darwin

package agypty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
	"github.com/portpowered/infinite-you/pkg/workers/process"
)

// posixPTYOpener opens a POSIX master/slave PTY pair via openpty semantics.
// Tests inject a mock opener to exercise allocation seams without a live Agy binary.
type posixPTYOpener func() (master, slave *os.File, err error)

// POSIXPTYAllocator allocates a POSIX openpty master/slave pair for supervised Agy children.
type POSIXPTYAllocator struct {
	Open posixPTYOpener
}

// NewPOSIXPTYAllocator returns a POSIX PTY allocator that uses openpty via creack/pty.
func NewPOSIXPTYAllocator() *POSIXPTYAllocator {
	return &POSIXPTYAllocator{Open: pty.Open}
}

// Allocate implements PTYAllocator.
func (a *POSIXPTYAllocator) Allocate(ctx context.Context, launch ProcessLaunch, cfg SessionConfig) (PTYSession, error) {
	if err := checkAllocateContext(ctx); err != nil {
		return nil, err
	}
	if err := validateProcessLaunch(launch); err != nil {
		return nil, err
	}
	opener := a.Open
	if opener == nil {
		opener = pty.Open
	}

	master, slave, err := opener()
	if err != nil {
		return nil, wrapPTYAllocationFailure(err)
	}

	allocation := &posixPTYAllocation{
		master: master,
		slave:  slave,
	}
	return newPlatformSession(launch, normalizeSessionConfig(cfg), PTYKindPOSIX, allocation)
}

type posixPTYAllocation struct {
	master *os.File
	slave  *os.File
}

func (p *posixPTYAllocation) Close() error {
	var firstErr error
	if p.master != nil {
		if err := p.master.Close(); err != nil && firstErr == nil {
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

// Master returns the POSIX PTY master fd used for host-side reads.
func (p *posixPTYAllocation) Master() *os.File {
	if p == nil {
		return nil
	}
	return p.master
}

// Slave returns the POSIX PTY slave TTY attached to the supervised child.
func (p *posixPTYAllocation) Slave() *os.File {
	if p == nil {
		return nil
	}
	return p.slave
}

func newPlatformPTYAllocator() PTYAllocator {
	return NewPOSIXPTYAllocator()
}

func runPlatformSession(ctx context.Context, session *platformSession) (SessionResult, error) {
	if session == nil {
		return SessionResult{}, errors.New("agypty: session is required")
	}
	defer closeSessionPTY(session)

	posix, ok := session.pty.(*posixPTYAllocation)
	if !ok || posix == nil {
		return SessionResult{}, errors.New("agypty: POSIX PTY allocation is required")
	}

	proc, reader, err := startPOSIXSessionProcess(session.launch, posix)
	if err != nil {
		return SessionResult{}, err
	}

	return executeSessionRun(ctx, session.cfg, reader, proc)
}

func startPOSIXSessionProcess(launch ProcessLaunch, alloc *posixPTYAllocation) (*sessionProcess, io.ReadCloser, error) {
	slave := alloc.Slave()
	master := alloc.Master()
	if slave == nil || master == nil {
		return nil, nil, errors.New("agypty: POSIX PTY handles are required")
	}

	args := launch.Argv
	if len(args) < 1 {
		return nil, nil, errors.New("agypty: argv is required")
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = launch.WorkDir
	if len(launch.Env) > 0 {
		cmd.Env = launch.Env
	}
	cmd.Stdout = slave
	cmd.Stdin = slave
	cmd.Stderr = slave
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Setctty = true

	process.ConfigureSubprocessTree(cmd)
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	_ = slave.Close()

	tree, err := process.AttachSubprocessTree(cmd)
	if err != nil {
		_ = process.TerminateSubprocessTree(cmd, tree)
		_ = cmd.Wait()
		return nil, nil, err
	}

	return &sessionProcess{cmd: cmd, tree: tree}, master, nil
}

func (p *sessionProcess) Wait() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return errors.New("agypty: process is not started")
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

func sessionProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func terminateSessionTestProcess(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
