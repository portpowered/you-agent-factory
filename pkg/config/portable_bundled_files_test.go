package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestMergePortableBundledFiles_ManifestAuthoritativeDocsSkipsUnlistedDiskDocs(t *testing.T) {
	existing := []interfaces.BundledFileConfig{{
		Type:       interfaces.BundledFileTypeDoc,
		TargetPath: "factory/docs/listed.md",
		Content: interfaces.BundledFileContentConfig{
			Encoding: interfaces.BundledFileEncodingUTF8,
		},
	}}
	collected := []interfaces.BundledFileConfig{
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/listed.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "listed content",
			},
		},
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/orphan.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "orphan content",
			},
		},
	}

	merged := mergePortableBundledFiles(existing, collected, false)
	if len(merged) != 1 {
		t.Fatalf("merged bundled files = %#v, want one listed doc", merged)
	}
	if merged[0].TargetPath != "factory/docs/listed.md" {
		t.Fatalf("merged doc target = %q, want factory/docs/listed.md", merged[0].TargetPath)
	}
	if merged[0].Content.Inline != "listed content" {
		t.Fatalf("merged doc inline = %q, want listed content", merged[0].Content.Inline)
	}
}

func TestMergePortableBundledFiles_DiscoverUnlistedDocsAddsDiskOnlyDocs(t *testing.T) {
	existing := []interfaces.BundledFileConfig{{
		Type:       interfaces.BundledFileTypeDoc,
		TargetPath: "factory/docs/listed.md",
		Content: interfaces.BundledFileContentConfig{
			Encoding: interfaces.BundledFileEncodingUTF8,
		},
	}}
	collected := []interfaces.BundledFileConfig{
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/listed.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "listed content",
			},
		},
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/orphan.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "orphan content",
			},
		},
	}

	merged := mergePortableBundledFiles(existing, collected, true)
	if len(merged) != 2 {
		t.Fatalf("merged bundled files = %#v, want listed and orphan docs", merged)
	}
}

func TestPruneRemovedPortableBundledDocs_RemovesDocsMissingFromManifest(t *testing.T) {
	factoryDir := filepath.Join(t.TempDir(), "factory")
	docsDir := filepath.Join(factoryDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", docsDir, err)
	}
	writePortableBundledDocsTestFile(t, filepath.Join(docsDir, "keep.md"), "keep")
	writePortableBundledDocsTestFile(t, filepath.Join(docsDir, "remove.md"), "remove")

	cfg := &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/keep.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			}},
		},
	}
	if err := pruneRemovedPortableBundledDocs(factoryDir, cfg); err != nil {
		t.Fatalf("pruneRemovedPortableBundledDocs: %v", err)
	}
	assertPortableBundledDocsTestFile(t, filepath.Join(docsDir, "keep.md"), "keep")
	if _, err := os.Stat(filepath.Join(docsDir, "remove.md")); !os.IsNotExist(err) {
		t.Fatalf("removed doc stat error = %v, want not exist", err)
	}
}

func writePortableBundledDocsTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func assertPortableBundledDocsTestFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}
