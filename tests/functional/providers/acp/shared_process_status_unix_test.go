//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package acp_test

import (
	"errors"
	"syscall"
)

func acpHelperProcessExited(pid int) (bool, error) {
	err := syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return false, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return true, nil
	}
	return false, err
}
