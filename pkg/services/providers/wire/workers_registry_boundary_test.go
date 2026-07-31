package wire_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProvidersWireDoesNotPublishWorkersProviderRegistry(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	root := filepath.Dir(file)
	entries, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatalf("glob providers/wire: %v", err)
	}
	forbidden := []string{
		"NewWorkersRegistry",
		"workers.ProviderRegistry",
		"var _ workers.ProviderRegistry",
	}
	for _, source := range entries {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", source, err)
		}
		body := string(content)
		for _, item := range forbidden {
			if strings.Contains(body, item) {
				t.Fatalf("%s still contains Workers registry edge %q", source, item)
			}
		}
	}
}
