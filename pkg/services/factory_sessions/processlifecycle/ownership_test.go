package processlifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFactorySessionsExclusivelyOwnsProductLifecycleSequencing(t *testing.T) {
	t.Parallel()

	assertSourcesExclude(t, filepath.Join("..", "..", "..", "initializer"), []string{
		"type RuntimeLifecycle struct",
		"NewRuntimeLifecycle",
		"StartRuntime(context.Context)",
		"StartWorkers(context.Context)",
	})
	assertSourcesExclude(t, filepath.Join("..", "..", "..", "wire"), []string{
		`"runtime sidecar"`,
		`"workers sidecar"`,
		`"Factory visualization sidecar"`,
		"buildRuntimeLifecycle",
		"provideRuntimeLifecycleFactory",
	})
}

func assertSourcesExclude(t *testing.T, root string, forbidden []string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "wire_gen.go" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, marker := range forbidden {
			if strings.Contains(string(source), marker) {
				t.Errorf("%s contains Factory Sessions-owned lifecycle policy %q", path, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", root, err)
	}
}
