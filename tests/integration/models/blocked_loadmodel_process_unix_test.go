//go:build !windows && managed_process_integration

package models_test

import (
	"syscall"
)

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
