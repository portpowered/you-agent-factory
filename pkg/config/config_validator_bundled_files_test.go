package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRuleBundledFiles_AcceptsSupportedDiskBackedScriptAndDocWithoutInline(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no bundled-file findings, got %#v", findings)
	}
}

func TestValidatePortableResourceManifestOnPath_AcceptsSupportedDiskBackedBundledFilesWithoutInline(t *testing.T) {
	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(scripts): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(factoryDir, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll(docs): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "scripts", "setup-workspace.py"), []byte("print('portable')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "docs", "usage.md"), []byte("# Usage\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(doc): %v", err)
	}

	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	if err := validatePortableResourceManifestOnPath(factoryDir, cfg); err != nil {
		t.Fatalf("validatePortableResourceManifestOnPath: %v", err)
	}
}

func TestConfigValidator_ValidateAcceptsSupportedDiskBackedBundledFilesWithoutInline(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	result := NewConfigValidator().Validate(cfg)
	if result.HasErrors() {
		t.Fatalf("expected config validator to accept supported disk-backed bundled files without inline content, got %#v", result.Errors())
	}
}

func TestConfigValidator_BundledFilesAcceptCanonicalScriptAndDocTargets(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       "SCRIPT",
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "print('portable')\n",
				},
			},
			{
				Type:       "DOC",
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "# Usage\n",
				},
			},
			{
				Type:       interfaces.BundledFileTypeRootHelper,
				TargetPath: "Makefile",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
					Inline:   "test:\n\tgo test ./...\n",
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no bundled-file findings, got %#v", findings)
	}
}

func TestRuleBundledFiles_RejectsTargetOutsideCanonicalRootForType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "DOC",
			TargetPath: "factory/scripts/usage.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: "utf-8",
				Inline:   "# Usage\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root")
}

func TestRuleBundledFiles_RejectsUnsupportedInputTargetShape(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeInput,
			TargetPath: "factory/inputs/task/default/nested/starter.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "starter work\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root")
	if !strings.Contains(findings[0].Message, "factory/inputs/<work-type>/<channel>/<file>") {
		t.Fatalf("expected INPUT shape guidance, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsDuplicateTargetPath(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{
				{
					Type:       interfaces.BundledFileTypeDoc,
					TargetPath: "factory/docs/overview.md",
					Content: interfaces.BundledFileContentConfig{
						Encoding: interfaces.BundledFileEncodingUTF8,
						Inline:   "first",
					},
				},
				{
					Type:       interfaces.BundledFileTypeDoc,
					TargetPath: "factory/docs/overview.md",
					Content: interfaces.BundledFileContentConfig{
						Encoding: interfaces.BundledFileEncodingUTF8,
						Inline:   "second",
					},
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-duplicate")
}

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
