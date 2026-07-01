package initializer_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitializerComposition_DoesNotDelegateToServiceBuildFactoryCore(t *testing.T) {
	t.Parallel()

	repoRoot := repoRootFromInitializerCompositionTest(t)
	files := []string{
		"pkg/initializer/services.go",
		"pkg/initializer/api_transport.go",
		"pkg/initializer/cli_transport.go",
	}
	forbidden := []string{
		"service.BuildFactoryCore(",
		"service.ComposeFactoryCore(",
	}

	for _, rel := range files {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			content, err := os.ReadFile(filepath.Join(repoRoot, rel))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			source := stripGoComments(string(content))
			for _, pattern := range forbidden {
				if strings.Contains(source, pattern) {
					t.Fatalf("%s must not delegate startup composition to root pkg/service; use pkg/initializer/factorycore BuildCore instead", rel)
				}
			}
		})
	}
}

func repoRootFromInitializerCompositionTest(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
