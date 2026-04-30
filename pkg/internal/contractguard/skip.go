package contractguard

import (
	"path/filepath"
	"strings"
)

// ShouldSkipRelativeDir centralizes the shared handwritten-source guard default:
// skip hidden metadata directories everywhere, then apply explicit call-site
// exceptions for generated or tool-owned directories.
func ShouldSkipRelativeDir(rel string, extraSkips ...string) bool {
	normalized := filepath.ToSlash(filepath.Clean(rel))
	if normalized == "." {
		return false
	}
	if base := filepath.Base(normalized); strings.HasPrefix(base, ".") {
		return true
	}
	for _, extra := range extraSkips {
		if normalized == filepath.ToSlash(filepath.Clean(extra)) {
			return true
		}
	}
	return false
}

func ShouldSkipDir(root, path string, extraSkips ...string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return ShouldSkipRelativeDir(rel, extraSkips...)
}
