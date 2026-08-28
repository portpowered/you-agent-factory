//go:build !windows

package process_test

import "syscall"

func contextCancellationProcessRunning(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func terminateContextCancellationProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
