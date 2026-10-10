package portableconfig

import (
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotmaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/materialize"
)

func TestCopySupportedFilesCopiesThinAssetsAndPreservesInlineTargets(t *testing.T) {
	t.Parallel()
	source, target := t.TempDir(), t.TempDir()
	files := []factorydefinitions.BundledFileConfig{
		{Type: factorydefinitions.BundledFileTypeScript, TargetPath: "factory/scripts/run.sh"},
		{Type: factorydefinitions.BundledFileTypeDoc, TargetPath: "factory/docs/guide.md"},
		{Type: factorydefinitions.BundledFileTypeInput, TargetPath: "factory/inputs/task/default/seed.md"},
		{Type: factorydefinitions.BundledFileTypeRootHelper, TargetPath: "Makefile"},
		{Type: factorydefinitions.BundledFileTypeDoc, TargetPath: "factory/docs/inline.md", Content: factorydefinitions.BundledFileContentConfig{Inline: "inline"}},
	}
	for _, path := range []string{"scripts/run.sh", "docs/guide.md", "inputs/task/default/seed.md"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(source, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, path), []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	config := &factorydefinitions.FactoryConfig{ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{BundledFiles: files}}
	if err := NewFilesCopier(platformfilesystem.Local{})(source, target, config); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"scripts/run.sh", "docs/guide.md", "inputs/task/default/seed.md"} {
		got, err := os.ReadFile(filepath.Join(target, path))
		if err != nil || string(got) != path {
			t.Fatalf("copied %s = %q, %v", path, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "docs/inline.md")); !os.IsNotExist(err) {
		t.Fatalf("inline target copied: %v", err)
	}
}

func TestSupportedSourceResolverRejectsUnownedTargets(t *testing.T) {
	t.Parallel()
	resolver, err := NewSupportedSourceResolver(platformfilesystem.Local{})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []factorydefinitions.BundledFileConfig{
		{Type: factorydefinitions.BundledFileTypeDoc, TargetPath: "factory/scripts/wrong.md"},
		{Type: factorydefinitions.BundledFileTypeScript, TargetPath: "factory/scripts/"},
		{Type: factorydefinitions.BundledFileTypeRootHelper, TargetPath: "unknown"},
	} {
		if path, ok := resolver(t.TempDir(), file); ok || path != "" {
			t.Fatalf("unowned source = %q, %v", path, ok)
		}
	}
}

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

	replacements, err := snapshotmaterialize.MaterializeFiles(
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

func TestPruneRemovedDocsRequiresFilesystemAndAcceptsAbsentDocs(t *testing.T) {
	t.Parallel()
	if err := PruneRemovedDocs(nil, "selected", nil); err == nil || err.Error() != "portable filesystem is required" {
		t.Fatalf("missing filesystem = %v", err)
	}
	factoryDir := t.TempDir()
	if err := PruneRemovedDocs(platformfilesystem.Local{}, factoryDir, nil); err != nil {
		t.Fatalf("absent docs = %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "docs"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PruneRemovedDocs(platformfilesystem.Local{}, factoryDir, nil); err != nil {
		t.Fatalf("non-directory docs = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(factoryDir, "docs"))
	if err != nil || string(content) != "keep" {
		t.Fatalf("non-directory docs changed: %q, %v", content, err)
	}
}
