package portableconfig

import (
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestMaterializeFilesReportsChangedExistingContent(t *testing.T) {
	factoryDir := t.TempDir()
	targetPath := filepath.Join(factoryDir, "scripts", "run.sh")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("create scripts directory: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed script: %v", err)
	}
	factoryConfig := &factorydefinitions.FactoryConfig{
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			BundledFiles: []factorydefinitions.BundledFileConfig{{
				Type:       factorydefinitions.BundledFileTypeScript,
				TargetPath: "factory/scripts/run.sh",
				Content: factorydefinitions.BundledFileContentConfig{
					Inline: "new\n",
				},
			}},
		},
	}

	replacements, err := MaterializeFiles(
		platformfilesystem.Local{},
		factoryDir,
		factoryConfig,
	)
	if err != nil {
		t.Fatalf("materialize portable files: %v", err)
	}
	if len(replacements) != 1 ||
		replacements[0].TargetPath != "factory/scripts/run.sh" {
		t.Fatalf("replacements = %#v", replacements)
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read materialized script: %v", err)
	}
	if string(content) != "new\n" {
		t.Fatalf("content = %q, want new content", content)
	}
}

func TestPruneRemovedDocsPreservesDeclaredAndIgnoredFiles(t *testing.T) {
	factoryDir := filepath.Join(t.TempDir(), "factory")
	docsDir := filepath.Join(factoryDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("create docs directory: %v", err)
	}
	for name, content := range map[string]string{
		"keep.md":   "keep",
		"remove.md": "remove",
		".gitkeep":  "",
	} {
		if err := os.WriteFile(
			filepath.Join(docsDir, name),
			[]byte(content),
			0o644,
		); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	factoryConfig := &factorydefinitions.FactoryConfig{
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			BundledFiles: []factorydefinitions.BundledFileConfig{{
				Type:       factorydefinitions.BundledFileTypeDoc,
				TargetPath: "factory/docs/keep.md",
			}},
		},
	}

	if err := PruneRemovedDocs(
		platformfilesystem.Local{},
		factoryDir,
		factoryConfig,
	); err != nil {
		t.Fatalf("prune docs: %v", err)
	}
	for _, name := range []string{"keep.md", ".gitkeep"} {
		if _, err := os.Stat(filepath.Join(docsDir, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(docsDir, "remove.md")); !os.IsNotExist(err) {
		t.Fatalf("removed doc stat error = %v, want not-exist", err)
	}
}
