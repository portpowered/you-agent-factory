package testutil

import (
	"path/filepath"
	"strings"
)

// ShouldSkipContractGuardDir centralizes the shared hidden-directory policy for
// broad handwritten-source contract guards while keeping package-local
// generated-directory exceptions explicit at the call site.
func ShouldSkipContractGuardDir(moduleRoot, path string, allowedSkipPaths ...string) bool {
	rel, err := filepath.Rel(moduleRoot, path)
	if err != nil {
		return false
	}

	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." {
		return false
	}

	for _, segment := range strings.Split(rel, "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}

	for _, allowed := range allowedSkipPaths {
		if rel == filepath.ToSlash(filepath.Clean(allowed)) {
			return true
		}
	}

	return false
}
