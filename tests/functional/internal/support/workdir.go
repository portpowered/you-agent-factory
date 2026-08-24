package support

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var workingDirectoryMu sync.Mutex

func ResolvedRuntimePath(factoryDir, configuredPath string) string {
	return filepath.Clean(filepath.Join(factoryDir, filepath.FromSlash(configuredPath)))
}

func SetWorkingDirectory(t *testing.T, dir string) {
	t.Helper()

	workingDirectoryMu.Lock()
	originalDir, err := os.Getwd()
	if err != nil {
		workingDirectoryMu.Unlock()
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		workingDirectoryMu.Unlock()
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
		workingDirectoryMu.Unlock()
	})
}
