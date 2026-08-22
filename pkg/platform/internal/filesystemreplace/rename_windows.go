//go:build windows

// Package filesystemreplace contains the policy-free Windows replacement
// algorithm used by the local filesystem adapter.
package filesystemreplace

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

const (
	localReplaceAttempts = 20
	localReplaceDelay    = 10 * time.Millisecond
)

// RenameReplacing publishes oldPath at newPath. Windows cannot rename over an
// existing file, so the replacement path retries the remove-and-rename window
// that can be held briefly by antivirus or indexing processes.
func RenameReplacing(
	oldPath string,
	newPath string,
	allowReplacement bool,
	rename func(string, string) error,
	remove func(string) error,
	stat func(string) (fs.FileInfo, error),
) error {
	lastErr := rename(oldPath, newPath)
	if lastErr == nil {
		return nil
	}
	if !allowReplacement || !errors.Is(lastErr, os.ErrExist) {
		return lastErr
	}
	if _, err := stat(oldPath); err != nil {
		return lastErr
	}
	for range localReplaceAttempts {
		if _, err := stat(oldPath); err != nil {
			return lastErr
		}
		if err := remove(newPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			lastErr = err
		} else {
			lastErr = rename(oldPath, newPath)
			if lastErr == nil {
				return nil
			}
		}
		time.Sleep(localReplaceDelay)
	}
	return lastErr
}
