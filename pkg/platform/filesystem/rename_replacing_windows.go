//go:build windows

package filesystem

import filesystemreplace "github.com/portpowered/infinite-you/pkg/platform/internal/filesystemreplace"

// RenameReplacing publishes oldPath at newPath. Windows cannot rename over an
// existing file, so the replacement path retries the remove-and-rename window
// that can be held briefly by antivirus or indexing processes.
func (local Local) RenameReplacing(oldPath, newPath string) error {
	return filesystemreplace.RenameReplacing(
		oldPath,
		newPath,
		local.AllowRenameReplacement,
		local.Rename,
		local.Remove,
		local.Stat,
	)
}
