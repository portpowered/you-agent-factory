package agypty

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveWorkspaceDir normalizes and validates a workspace directory under
// factoryRoot (T2 control in agy-pty-threat-review.md). The returned path is
// suitable for cmd.Dir and argv path fields after Story 17 lands execution.
func ResolveWorkspaceDir(factoryRoot, rawPath string) (string, error) {
	factoryRoot = strings.TrimSpace(factoryRoot)
	if factoryRoot == "" {
		return "", fmt.Errorf("agypty: factory root is required")
	}
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("agypty: workspace path is required")
	}

	factoryRoot = filepath.Clean(factoryRoot)
	normalized := filepath.Clean(filepath.FromSlash(rawPath))

	var resolved string
	if filepath.IsAbs(normalized) {
		resolved = normalized
	} else {
		if err := rejectRelativeTraversal(normalized); err != nil {
			return "", err
		}
		resolved = filepath.Clean(filepath.Join(factoryRoot, normalized))
	}

	if !pathContainedIn(factoryRoot, resolved) {
		return "", fmt.Errorf("agypty: workspace path must remain under factory root")
	}
	return resolved, nil
}

func rejectRelativeTraversal(normalized string) error {
	if normalized == ".." {
		return fmt.Errorf("agypty: workspace path must not traverse outside factory root")
	}
	if strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
		return fmt.Errorf("agypty: workspace path must not traverse outside factory root")
	}
	return nil
}

func pathContainedIn(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)

	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}
