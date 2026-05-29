package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultWorktreesParentName = ".worktrees"

var existingWorktreeParentCandidates = []string{
	".worktrees",
	"worktrees",
	filepath.Join(".claude", "worktrees"),
}

// ResolveFactoryWorktreeParent returns the directory under factoryRoot where new
// git worktree checkouts should be created. When no existing worktrees parent is
// present, the default is <factoryRoot>/.worktrees.
func ResolveFactoryWorktreeParent(factoryRoot string) (string, error) {
	factoryRoot = strings.TrimSpace(factoryRoot)
	if factoryRoot == "" {
		return "", fmt.Errorf("factory root is required")
	}
	factoryRoot = filepath.Clean(factoryRoot)

	for _, candidate := range existingWorktreeParentCandidates {
		candidatePath := filepath.Join(factoryRoot, candidate)
		if isDirectory(candidatePath) {
			return candidatePath, nil
		}
	}

	return filepath.Join(factoryRoot, defaultWorktreesParentName), nil
}

// ResolveFactoryWorktreeCheckoutPath returns the target checkout path for a
// resolved worktree name under the factory-local worktrees parent.
func ResolveFactoryWorktreeCheckoutPath(factoryRoot, worktreeName string) (string, error) {
	parent, err := ResolveFactoryWorktreeParent(factoryRoot)
	if err != nil {
		return "", err
	}

	cleanName, err := cleanWorktreeName(worktreeName)
	if err != nil {
		return "", err
	}

	return filepath.Clean(filepath.Join(parent, cleanName)), nil
}

func cleanWorktreeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("worktree name is required")
	}

	normalized := filepath.FromSlash(name)
	if filepath.IsAbs(normalized) {
		return "", fmt.Errorf("worktree name must be relative")
	}

	cleaned := filepath.Clean(normalized)
	if cleaned == "." || cleaned == ".." {
		return "", fmt.Errorf("worktree name is invalid")
	}
	if strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("worktree name must not traverse outside parent")
	}

	return cleaned, nil
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
