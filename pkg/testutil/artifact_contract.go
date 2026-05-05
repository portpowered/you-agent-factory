package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func MustRepoRoot(t testing.TB) string {
	t.Helper()

	_, callerFile, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("cannot determine caller file path")
	}

	root, err := findRepoRoot(filepath.Dir(callerFile))
	if err != nil {
		t.Fatalf("find repo root from %s: %v", callerFile, err)
	}
	return root
}

func MustRepoPath(t testing.TB, rel string) string {
	t.Helper()
	return filepath.Join(MustRepoRoot(t), filepath.FromSlash(rel))
}

func findRepoRoot(startDir string) (string, error) {
	current := filepath.Clean(startDir)
	for {
		goModPath := filepath.Join(current, "go.mod")
		if info, err := os.Stat(goModPath); err == nil && !info.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}
