package initializer_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Transport composition entrypoints must not use root pkg/service as the primary
// application shell. They should compose through pkg/initializer transport helpers.
func TestTransportComposition_DoesNotUseRootServiceShell(t *testing.T) {
	t.Parallel()

	repoRoot := repoRootFromTest(t)
	files := []string{
		"cmd/factory/main.go",
		"cmd/factory/compose/api_transport.go",
		"cmd/factory/compose/cli_transport.go",
		"cmd/factory/compose/mcp_transport.go",
		"pkg/cli/mcp/serve.go",
		"tests/functional/internal/support/api_server.go",
		"tests/functional/internal/support/cmd/browser_api_harness/main.go",
	}
	forbidden := []string{
		"compose.InjectFactoryService(",
		"service.BuildFactoryService(",
		"InjectFactoryService(ctx",
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
					t.Fatalf("%s must not call root service composition %q; use pkg/initializer transport composition instead", rel, pattern)
				}
			}
		})
	}
}

func TestInitializerComposition_DoesNotDelegateToServiceBuildFactoryCore(t *testing.T) {
	t.Parallel()

	repoRoot := repoRootFromTest(t)
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
					t.Fatalf("%s must not delegate startup composition to root pkg/service; use pkg/initializer BuildCore instead", rel)
				}
			}
		})
	}
}

func repoRootFromTest(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// pkg/initializer/transport_dependency_test.go -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func stripGoComments(source string) string {
	var b strings.Builder
	b.Grow(len(source))
	inBlock := false
	lines := strings.Split(source, "\n")
	for _, line := range lines {
		if inBlock {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				inBlock = false
				line = line[idx+2:]
			} else {
				continue
			}
		}
		for {
			if strings.HasPrefix(line, "/*") {
				end := strings.Index(line, "*/")
				if end < 0 {
					inBlock = true
					line = ""
					break
				}
				line = line[:strings.Index(line, "/*")] + line[end+2:]
				continue
			}
			if idx := strings.Index(line, "//"); idx >= 0 {
				line = line[:idx]
			}
			break
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
