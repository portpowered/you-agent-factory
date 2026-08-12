//go:build !windows

package process_test

import (
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
		return hardKillProcessControl{}, err
	}
	var resumeOnce sync.Once
	var resumeErr error
	resume := func() error {
		resumeOnce.Do(func() { resumeErr = process.Signal(syscall.SIGCONT) })
		return resumeErr
	}
	var terminateOnce sync.Once
	var terminateErr error
	terminate := func() error {
		terminateOnce.Do(func() { terminateErr = process.Kill() })
		return terminateErr
	}
	return hardKillProcessControl{resume: resume, terminate: terminate}, nil
}

func isPreRuntimeStagingMetadataUnavailable(error) bool {
	return false
}
