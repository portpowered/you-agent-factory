package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRuleResourceUsage_NonexistentResource(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "bogus", Capacity: 1}},
	}}
	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-ref")
}

func TestRuleResourceUsage_ZeroCapacity(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 0}},
	}}
	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-capacity")
}

func TestRuleResourceUsage_ValidConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 2}},
	}}
	findings := ruleResourceUsage(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleResourceUsage_ValidatesWorkerRequirements(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:      "worker-a",
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 0}, {Name: "missing", Capacity: 1}},
	}}

	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-capacity")
	assertFindingExists(t, findings, "resource-usage-ref")
}

func TestRuleResourceDefinitions_RequiresModelMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{
		Name:     "omnivoice-cache",
		Type:     interfaces.ResourceTypeModel,
		Capacity: 1,
	}}

	findings := ruleResourceDefinitions(cfg)
	assertFindingExists(t, findings, "resource-model-model")
	assertFindingExists(t, findings, "resource-model-backend")
	assertFindingExists(t, findings, "resource-model-load-policy")
}

func TestRuleResourceDefinitions_RequiresProviderQuotaMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{
		Name:     "codex-tts-quota",
		Type:     interfaces.ResourceTypeProviderQuota,
		Capacity: 2,
	}}

	findings := ruleResourceDefinitions(cfg)
	assertFindingExists(t, findings, "resource-provider-quota-provider")
	assertFindingExists(t, findings, "resource-provider-quota-model")
}

func TestRuleResourceDefinitions_AcceptsModelResourceMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{
		Name:       "omnivoice-cache",
		Type:       interfaces.ResourceTypeModel,
		Capacity:   1,
		Model:      "OMNIVOICE_Q4_K_M",
		Backend:    "LLAMACPP",
		LoadPolicy: "ON_DEMAND",
	}}

	if findings := ruleResourceDefinitions(cfg); len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleRequiredTools_MissingNameAndCommand(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{}},
	}

	findings := ruleRequiredTools(nil)(cfg)
	assertFindingExists(t, findings, "required-tool-name")
	assertFindingExists(t, findings, "required-tool-command")
}

func TestConfigValidator_RequiredToolsReportsPresentAndMissingCommandsDeterministically(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{
			{Name: "Go toolchain", Command: "go"},
			{Name: "Missing helper", Command: "missing-tool"},
		},
	}

	validator := NewConfigValidator(WithRequiredToolChecker(stubRequiredToolChecker{
		"go":           {ResolvedPath: "/usr/bin/go"},
		"missing-tool": {Err: assertErrString(`required tool "Missing helper" command "missing-tool" was not found on PATH`)},
	}))
	result := validator.Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected missing required tool to produce an error")
	}
	if len(result.Errors()) != 1 {
		t.Fatalf("expected one required-tool error, got %#v", result.Errors())
	}
	finding := result.Errors()[0]
	if finding.Rule != "required-tool-missing" {
		t.Fatalf("expected required-tool-missing rule, got %#v", finding)
	}
	if finding.Path != "resourceManifest.requiredTools[1].command" {
		t.Fatalf("expected path-specific missing-tool finding, got %#v", finding)
	}
	if !strings.Contains(finding.Message, `"missing-tool" was not found on PATH`) {
		t.Fatalf("expected PATH lookup guidance, got %#v", finding)
	}
}

func TestRuleRequiredTools_InvalidVersionProbeUsesVersionArgsPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
			Name:        "Python",
			Command:     "python",
			VersionArgs: []string{"--version"},
		}},
	}

	findings := ruleRequiredTools(stubRequiredToolChecker{
		"python": {
			FailureKind: RequiredToolFailureKindVersionProbe,
			Err:         assertErrString(`required tool "Python" command "python" failed version probe "--version": exit status 1`),
		},
	})(cfg)
	assertFindingExists(t, findings, "required-tool-version-probe")
	if findings[0].Path != "resourceManifest.requiredTools[0].versionArgs" {
		t.Fatalf("expected versionArgs path, got %#v", findings[0])
	}
}

func TestRuleRequiredTools_MissingCommandWithVersionArgsUsesCommandPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
			Name:        "Portable helper",
			Command:     "missing-helper",
			VersionArgs: []string{"--version"},
		}},
	}

	findings := ruleRequiredTools(stubRequiredToolChecker{
		"missing-helper": {
			FailureKind: RequiredToolFailureKindMissing,
			Err:         assertErrString(`required tool "Portable helper" command "missing-helper" was not found on PATH`),
		},
	})(cfg)
	assertFindingExists(t, findings, "required-tool-missing")
	if findings[0].Path != "resourceManifest.requiredTools[0].command" {
		t.Fatalf("expected command path for missing tool, got %#v", findings[0])
	}
}

func TestRuleRequiredTools_RejectsBlankVersionArgsEntries(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
			Name:        "Python",
			Command:     "python",
			VersionArgs: []string{"--version", ""},
		}},
	}

	findings := ruleRequiredTools(nil)(cfg)
	assertFindingExists(t, findings, "required-tool-version-args")
}

func TestRuleBundledFiles_RejectsUnsupportedTypeEncodingAndRoot(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "BINARY",
			TargetPath: "factory/misc/helper.bin",
			Content: interfaces.BundledFileContentConfig{
				Encoding: "base64",
				Inline:   "AA==",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-type")
	assertFindingExists(t, findings, "bundled-file-content-encoding")
}

func TestRuleBundledFiles_RejectsUnsafeTargetPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "SCRIPT",
			TargetPath: "../scripts/setup-workspace.py",
			Content: interfaces.BundledFileContentConfig{
				Encoding: "utf-8",
				Inline:   "print('portable')\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-path")
	if findings[0].Path != "resourceManifest.bundledFiles[0].targetPath" {
		t.Fatalf("expected targetPath-specific finding, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsAbsoluteTargetPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeScript,
			TargetPath: "/factory/scripts/setup-workspace.py",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "print('portable')\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-path")
	if !strings.Contains(findings[0].Message, "not absolute") {
		t.Fatalf("expected absolute-path guidance, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsMissingInlineContent(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeRootHelper,
			TargetPath: "Makefile",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-content-inline")
	if findings[0].Path != "resourceManifest.bundledFiles[0].content.inline" {
		t.Fatalf("expected inline-specific finding, got %#v", findings[0])
	}
}

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

func TestRuleBundledFiles_RejectsUnsupportedRootHelperTarget(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeRootHelper,
			TargetPath: "README.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "outside allowlist\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root-helper")
}
