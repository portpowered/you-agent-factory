//go:build !windows

package contractstaging

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockRepositoryStagingFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

func unlockRepositoryStagingFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
