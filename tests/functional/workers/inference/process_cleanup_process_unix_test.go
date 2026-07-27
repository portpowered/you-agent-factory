//go:build !windows

package inference_test

import "syscall"

func processCleanupProcessRunning(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func processCleanupTerminateProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
