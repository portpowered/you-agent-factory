package backenddependencygraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderDOTIncludesOnlySelectedPackageDependencies(t *testing.T) {
	t.Parallel()

	packages := []Package{
		{
			ImportPath: "example.com/factory/pkg/service",
			Imports: []string{
				"example.com/factory/pkg/work",
				"fmt",
			},
		},
		{
			ImportPath: "example.com/factory/cmd/factory",
			Imports:    []string{"example.com/factory/pkg/service"},
		},
		{ImportPath: "example.com/factory/pkg/work"},
	}

	got := string(RenderDOT(packages, "example.com/factory"))
	wantFragments := []string{
		`"example.com/factory/cmd/factory" [label="cmd/factory", fillcolor="#fef3c7"]`,
		`"example.com/factory/pkg/service" [label="pkg/service", fillcolor="#dbeafe"]`,
		`"example.com/factory/cmd/factory" -> "example.com/factory/pkg/service"`,
		`"example.com/factory/pkg/service" -> "example.com/factory/pkg/work"`,
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Errorf("RenderDOT() missing %q in:\n%s", fragment, got)
		}
	}
	if strings.Contains(got, `-> "fmt"`) {
		t.Fatalf("RenderDOT() included standard-library dependency in:\n%s", got)
	}
	if strings.Index(got, "cmd/factory") > strings.Index(got, "pkg/service") {
		t.Fatalf("RenderDOT() nodes are not sorted in:\n%s", got)
	}
}

func TestWriteDOTCreatesParentDirectory(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join(t.TempDir(), "nested", "graph.dot")
	if err := WriteDOT(outputPath, []byte("digraph {}\n")); err != nil {
		t.Fatalf("WriteDOT() error = %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "digraph {}\n" {
		t.Fatalf("WriteDOT() contents = %q", got)
	}
}
