package http_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPackageBoundary_DoesNotImportFactoryDefinitionsInternal(t *testing.T) {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	packageDir := filepath.Dir(filename)

	forbidden := "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read HTTP adapter package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(packageDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("%s must not import %s", entry.Name(), forbidden)
		}
	}
}
