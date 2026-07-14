//go:build linux || darwin

package agypty

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"syscall"

	"github.com/portpowered/infinite-you/pkg/workers/process"
)

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
