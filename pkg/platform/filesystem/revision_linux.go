package filesystem

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// ReadRevision checks readability and includes inode identity and change time,
// so restoring a file's size and write time cannot hide ordinary replacement
// or an in-place update. Unsupported metadata falls back to content reads.
func (Local) ReadRevision(path string) (revision string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			revision, err = "", closeErr
		}
	}()
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &filesystem); err != nil {
		return "", nil
	}
	switch filesystem.Type {
	case unix.EXT4_SUPER_MAGIC, unix.XFS_SUPER_MAGIC, unix.BTRFS_SUPER_MAGIC, unix.TMPFS_MAGIC, unix.OVERLAYFS_SUPER_MAGIC:
	default:
		return "", nil // Coarse/remote metadata must not authorize content reuse.
	}
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return "", nil
	}
	return fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x", stat.Dev, stat.Ino, stat.Size,
		stat.Mtim.Sec, stat.Mtim.Nsec, stat.Ctim.Sec, stat.Ctim.Nsec), nil
}
