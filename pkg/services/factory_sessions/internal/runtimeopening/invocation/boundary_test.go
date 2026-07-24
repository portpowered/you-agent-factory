package invocation

import (
	"os"
	"strings"
	"testing"
)

func TestInvocationOpeningDoesNotImportProcessEdgesBag(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read invocation package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(source), "pkg/services/edges") {
			t.Errorf("%s imports process-edge bag; inject runtimeopening.ExternalEffects instead", entry.Name())
		}
	}
}
