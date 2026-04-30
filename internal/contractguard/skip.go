package contractguard

import (
	"path/filepath"
	"strings"
)

// ShouldSkipDir reports whether a broad handwritten-source guard should skip
// the provided directory because it is hidden metadata or an explicit
// generated/build-output subtree outside the guard's intended surface.
func ShouldSkipDir(scanRoot, path string, explicitSkips ...string) bool {
	rel, err := filepath.Rel(scanRoot, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	for _, skip := range explicitSkips {
		if rel == filepath.ToSlash(filepath.Clean(skip)) {
			return true
		}
	}
	return false
}
