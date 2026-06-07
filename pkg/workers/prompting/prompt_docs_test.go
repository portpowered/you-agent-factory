package prompting

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeFactoryBundledDocTargetPaths_FiltersAndSorts(t *testing.T) {
	got := NormalizeFactoryBundledDocTargetPaths([]string{
		"factory/docs/guide.md",
		"factory/scripts/setup.py",
		"factory/docs/overview.md",
		"factory/docs/guide.md",
	})

	want := []string{"factory/docs/guide.md", "factory/docs/overview.md"}
	if len(got) != len(want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	for index, targetPath := range want {
		if got[index] != targetPath {
			t.Fatalf("paths[%d] = %q, want %q", index, got[index], targetPath)
		}
	}
}

func TestLoadBundledDocContentsFromFactoryDir_LoadsDocs(t *testing.T) {
	factoryDir := t.TempDir()
	docsDir := filepath.Join(factoryDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "overview.md"), []byte("overview"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	contents := loadBundledDocContentsFromFactoryDir(factoryDir)
	if contents["factory/docs/overview.md"] != "overview" {
		t.Fatalf("contents = %#v, want overview doc content", contents)
	}
}
