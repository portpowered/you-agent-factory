//go:build !windows

package process

import "syscall"

func commandTestProcessRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func commandTestTerminateProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
