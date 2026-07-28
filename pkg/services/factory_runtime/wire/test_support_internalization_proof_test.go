package wire_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// DEL-RUN-ENGINE-PIPELINE story 003 proves public test-support directories were
// internalized and no module imports reference the former public paths.

var internalizedPublicTestSupportDirs = []string{
	"testkit",
	"exhaustiontests",
}

func TestTestSupportInternalizationProof_NoPublicTestSupportDirectories(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime")
	for _, name := range internalizedPublicTestSupportDirs {
		path := filepath.Join(runtimeRoot, name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("public test-support directory %q still exists at %s", name, path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func TestTestSupportInternalizationProof_InternalizedPathsExist(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime", "internal")
	for _, name := range internalizedPublicTestSupportDirs {
		path := filepath.Join(runtimeRoot, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("internalized test-support directory %q missing at %s: %v", name, path, err)
		}
	}
}

func TestTestSupportInternalizationProof_NoModuleImportsOfFormerPublicPaths(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		for _, moved := range internalizedPublicTestSupportDirs {
			prefix := "github.com/portpowered/infinite-you/pkg/services/factory_runtime/" + moved
			if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
				t.Fatalf("module still imports former public test-support path: %s", importPath)
			}
		}
	}
}
