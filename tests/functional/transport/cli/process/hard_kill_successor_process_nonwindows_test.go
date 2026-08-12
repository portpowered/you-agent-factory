//go:build !windows

package process_test

import (
	"errors"
	"os"
	"sync"
	"syscall"
)

type hardKillProcessHandle interface {
	Signal(os.Signal) error
	Kill() error
	Release() error
}

func suspendHardKillProcess(pid int) (hardKillProcessControl, error) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return hardKillProcessControl{}, err
	}
	return newHardKillProcessControl(process)
}

func newHardKillProcessControl(process hardKillProcessHandle) (hardKillProcessControl, error) {
	if err := process.Signal(syscall.SIGSTOP); err != nil {
		return hardKillProcessControl{}, errors.Join(err, process.Release())
	}

	var mu sync.Mutex
	terminal := false
	terminalOperation := func(signal func() error) error {
		mu.Lock()
		defer mu.Unlock()
		if terminal {
			return nil
		}
		terminal = true
		return errors.Join(signal(), process.Release())
	}
	return hardKillProcessControl{
		resume: func() error {
			return terminalOperation(func() error { return process.Signal(syscall.SIGCONT) })
		},
		terminate: func() error {
			return terminalOperation(process.Kill)
		},
	}, nil
}

func isPreRuntimeStagingMetadataUnavailable(error) bool {
	return false
}
