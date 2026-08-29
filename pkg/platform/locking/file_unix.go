//go:build !windows

package locking

import (
	"golang.org/x/sys/unix"
)

func tryLockFile(file File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == unix.EWOULDBLOCK || err == unix.EAGAIN {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(file File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
