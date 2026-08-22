//go:build !windows

package filesystem

// RenameReplacing publishes oldPath at newPath. Non-Windows hosts replace an
// existing destination as part of the native rename operation.
func (local Local) RenameReplacing(oldPath, newPath string) error {
	return local.Rename(oldPath, newPath)
}
