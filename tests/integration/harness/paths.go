package harness

import (
	"fmt"
	"os"
	"path/filepath"
)

func canonicalPath(path, base string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if !filepath.IsAbs(path) {
		if base == "" {
			base, _ = os.Getwd()
		}
		path = filepath.Join(base, path)
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Abs(filepath.Clean(resolved))
	}

	return canonicalPathWithMissingSuffix(path)
}

func canonicalPathWithMissingSuffix(path string) (string, error) {
	missing := make([]string, 0, 4)
	candidate := path
	for {
		if _, err := os.Lstat(candidate); err == nil {
			resolved, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				return "", err
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Abs(filepath.Clean(resolved))
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return path, nil
		}
		missing = append(missing, filepath.Base(candidate))
		candidate = parent
	}
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	if relative == "." {
		return true
	}
	parentPrefix := ".." + string(filepath.Separator)
	return relative != ".." && !hasPathPrefix(relative, parentPrefix)
}

func hasPathPrefix(path, prefix string) bool {
	return len(path) >= len(prefix) && path[:len(prefix)] == prefix
}
