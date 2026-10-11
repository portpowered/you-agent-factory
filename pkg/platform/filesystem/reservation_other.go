//go:build !windows

package filesystem

import (
	"errors"
	"io/fs"
	"os"
)

// RemoveReserved checks original file identity, not just its name or length.
// As with ordinary POSIX unlink, callers must exclude concurrent publishers
// through their existing target ownership before releasing the reservation.
func (Local) RemoveReserved(path string, original fs.FileInfo) error {
	current, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(original, current) || current.Size() != 0 || !original.ModTime().Equal(current.ModTime()) {
		return nil
	}
	return os.Remove(path)
}
