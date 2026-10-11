//go:build windows

package filesystem

import "golang.org/x/sys/windows"

// MoveFileEx replaces a file without a remove-and-rename gap. Staging in the
// destination directory prevents a cross-volume copy fallback; write-through
// completes the host move before success is published to the caller.
func (Local) publishReplacement(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
