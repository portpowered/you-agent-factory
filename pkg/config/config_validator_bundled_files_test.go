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
