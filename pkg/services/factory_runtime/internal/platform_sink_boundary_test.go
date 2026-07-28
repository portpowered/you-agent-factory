package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFactoryRuntimeServiceDoesNotImportPlatformSinkImplementations(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"build.go", "runtime_build.go", "assembly.go"} {
		source, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, forbidden := range []string{
			"pkg/platform/logging",
			"pkg/platform/metrics",
			"BuildRuntimeLogger",
			"BuildRuntimeMetricsSink",
			"uuid.NewString",
			"time.Now",
			"os.UserHomeDir",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s contains prohibited sink construction/default %q", name, forbidden)
			}
		}
	}
}
