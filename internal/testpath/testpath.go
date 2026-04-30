package testpath

import (
	"os"
	"path/filepath"
	"runtime"
)

type fatalHelper interface {
	Helper()
	Fatalf(format string, args ...any)
}

// MustRepoRootFromCaller walks upward from the caller's file until it finds the
// module root containing go.mod.
func MustRepoRootFromCaller(t fatalHelper, skip int) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(skip + 2)
	if !ok {
		t.Fatalf("cannot determine caller path")
	}

	dir := filepath.Dir(file)
	for {
		if isRepoRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", file)
		}
		dir = parent
	}
}

func MustRepoPathFromCaller(t fatalHelper, skip int, rel ...string) string {
	t.Helper()
	parts := append([]string{MustRepoRootFromCaller(t, skip)}, rel...)
	return filepath.Join(parts...)
}

func isRepoRoot(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil && !info.IsDir()
}
