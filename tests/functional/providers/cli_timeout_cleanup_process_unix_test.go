//go:build !windows

package providers

import "syscall"

func timeoutCleanupProcessRunning(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func timeoutCleanupTerminateProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
