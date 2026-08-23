//go:build !windows

package metrics

import (
	"os"

	"golang.org/x/sys/unix"
)

func tryLockRuntimeMetricsFile(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == unix.EWOULDBLOCK || err == unix.EAGAIN {
		return false, nil
	}
	return err == nil, err
}

func unlockRuntimeMetricsFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
