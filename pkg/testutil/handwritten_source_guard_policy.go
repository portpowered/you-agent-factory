package testutil

import "github.com/portpowered/infinite-you/internal/handwrittensourceguard"

// ShouldSkipHandwrittenSourceDir reports whether a filesystem-walking
// handwritten-source guard should skip the provided directory.
func ShouldSkipHandwrittenSourceDir(guardFile, walkRoot, path string) bool {
	return handwrittensourceguard.ShouldSkipDir(guardFile, walkRoot, path)
}
