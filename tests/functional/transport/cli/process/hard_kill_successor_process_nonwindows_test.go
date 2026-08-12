//go:build !windows

package process_test

import (
	"errors"
	"os"
	"sync"
	"syscall"
)

func suspendHardKillProcess(pid int) (hardKillProcessControl, error) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return hardKillProcessControl{}, err
	}
	if err := process.Signal(syscall.SIGSTOP); err != nil {
		return hardKillProcessControl{}, errors.Join(err, process.Release())
	}

	var mu sync.Mutex
	released := false
	resume := func() error {
		mu.Lock()
		defer mu.Unlock()
		if released {
			return nil
		}
		signalErr := process.Signal(syscall.SIGCONT)
		releaseErr := process.Release()
		released = true
		return errors.Join(signalErr, releaseErr)
	}
	terminate := func() error {
		mu.Lock()
		defer mu.Unlock()
		if released {
			return nil
		}
		killErr := process.Kill()
		releaseErr := process.Release()
		released = true
		return errors.Join(killErr, releaseErr)
	}
	return hardKillProcessControl{resume: resume, terminate: terminate}, nil
}

func isPreRuntimeStagingMetadataUnavailable(error) bool {
	return false
}
