//go:build !windows

package filesystem

// The bytes have been flushed before the native same-filesystem rename. Rename
// atomically replaces the destination; no descriptor remains open afterwards.
func (local Local) publishReplacement(source, destination string) error {
	return local.Rename(source, destination)
}
