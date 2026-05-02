package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFlattenFactoryConfig_CollectsSupportedPortableBundledFiles(t *testing.T) {
	projectDir := t.TempDir()
	factoryDir := filepath.Join(projectDir, portableFactoryDirName)

	writePortableBundledTestFile(t, filepath.Join(factoryDir, interfaces.FactoryConfigFile), `{
  "name":"portable-bundled-files-test",
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "resources": [],
  "workers": [{"name":"executor"}],
  "workstations": [{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":{"workType":"task","state":"failed"}
  }]
}`)
	writePortableBundledTestFile(t, filepath.Join(factoryDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName), `---
type: SCRIPT_WORKER
command: powershell
args:
  - -File
  - factory/scripts/execute-story.ps1
---
Execute the story.
`)
	writePortableBundledTestFile(t, filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-story", interfaces.FactoryAgentsFileName), `---
type: MODEL_WORKSTATION
---
Execute {{ (index .Inputs 0).WorkID }}.
`)
	writePortableBundledTestFile(t, filepath.Join(factoryDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	writePortableBundledTestFile(t, filepath.Join(factoryDir, "docs", "README.md"), "# Portable factory\n")
	writePortableBundledTestFile(t, filepath.Join(projectDir, "Makefile"), "test:\n\tgo test ./...\n")
	writePortableBundledTestFile(t, filepath.Join(projectDir, "README.md"), "outside allowlist\n")

	flattened, err := FlattenFactoryConfig(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	cfg, err := FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatal("expected flatten to include bundled files")
	}
	if len(cfg.ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("expected 3 bundled files, got %#v", cfg.ResourceManifest.BundledFiles)
	}

	assertPortableBundledEntry(t, cfg.ResourceManifest.BundledFiles[0], interfaces.BundledFileTypeRootHelper, "Makefile", "test:\n\tgo test ./...\n")
	assertPortableBundledEntry(t, cfg.ResourceManifest.BundledFiles[1], interfaces.BundledFileTypeDoc, "factory/docs/README.md", "# Portable factory\n")
	assertPortableBundledEntry(t, cfg.ResourceManifest.BundledFiles[2], interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n")
}

func TestWriteExpandedFactoryLayout_MaterializesPortableBundledFiles(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	cfg := &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{
				portableBundledFixtureFile(interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n"),
				portableBundledFixtureFile(interfaces.BundledFileTypeDoc, "factory/docs/README.md", "# Portable factory\n"),
				portableBundledFixtureFile(interfaces.BundledFileTypeRootHelper, "Makefile", "test:\n\tgo test ./...\n"),
			},
		},
	}
	canonical := flattenPortableBundledTestFactory(t, cfg)

	if err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("writeExpandedFactoryLayout: %v", err)
	}

	assertPortableBundledExpandedFile(t, targetDir, filepath.Join("docs", "README.md"), "# Portable factory\n")
	assertPortableBundledExpandedFile(t, targetDir, filepath.Join("scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPortableBundledExpandedFile(t, targetDir, "Makefile", "test:\n\tgo test ./...\n")
	assertPortableBundledExecutableScriptMode(t, filepath.Join(targetDir, "scripts", "execute-story.ps1"))
	if _, err := os.Stat(filepath.Join(targetDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("expected expanded factory config: %v", err)
	}
}

func TestPortableBundledTargetPath_MaterializesSupportedPortablePaths(t *testing.T) {
	targetDir := t.TempDir()

	tests := []struct {
		name         string
		targetPath   string
		wantRelative string
	}{
		{
			name:         "script under factory root",
			targetPath:   "factory/scripts/execute-story.ps1",
			wantRelative: filepath.Join("scripts", "execute-story.ps1"),
		},
		{
			name:         "doc under factory root",
			targetPath:   "factory/docs/README.md",
			wantRelative: filepath.Join("docs", "README.md"),
		},
		{
			name:         "root helper",
			targetPath:   "Makefile",
			wantRelative: "Makefile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := portableBundledTargetPath(targetDir, tt.targetPath)
			if err != nil {
				t.Fatalf("portableBundledTargetPath(%q): %v", tt.targetPath, err)
			}
			if target.path != filepath.Join(targetDir, tt.wantRelative) {
				t.Fatalf("target.path = %q, want %q", target.path, filepath.Join(targetDir, tt.wantRelative))
			}
		})
	}
}

func TestWriteExpandedFactoryLayout_RejectsUnsafePortableBundledFileTargetsBeforeWriting(t *testing.T) {
	tests := []struct {
		name       string
		targetPath string
		want       string
	}{
		{
			name:       "absolute target location",
			targetPath: filepath.Join(t.TempDir(), "outside.ps1"),
			want:       "must be relative to the expand target",
		},
		{
			name:       "escaping target location",
			targetPath: "../outside.ps1",
			want:       "cannot escape the expand target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceDir := t.TempDir()
			targetDir := t.TempDir()
			cfg := &interfaces.FactoryConfig{
				ResourceManifest: &interfaces.PortableResourceManifestConfig{
					BundledFiles: []interfaces.BundledFileConfig{
						portableBundledFixtureFile(interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n"),
						portableBundledFixtureFile(interfaces.BundledFileTypeScript, tt.targetPath, "Write-Output 'unsafe'\n"),
					},
				},
			}
			canonical := flattenPortableBundledTestFactory(t, cfg)

			err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile))
			if err == nil {
				t.Fatal("expected unsafe bundled file target to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.want)
			}
			if !strings.Contains(err.Error(), filepath.Base(tt.targetPath)) {
				t.Fatalf("error = %q, want offending target file %q", err.Error(), filepath.Base(tt.targetPath))
			}
			if _, statErr := os.Stat(filepath.Join(targetDir, "factory", "scripts", "execute-story.ps1")); !os.IsNotExist(statErr) {
				t.Fatalf("expected no bundled files to be written before validation, stat err = %v", statErr)
			}
			if _, statErr := os.Stat(filepath.Join(targetDir, interfaces.FactoryConfigFile)); !os.IsNotExist(statErr) {
				t.Fatalf("expected no expanded factory config to be written before validation, stat err = %v", statErr)
			}
		})
	}
}

func TestWriteExpandedFactoryLayout_RejectsPortableBundledFileTargetsThatEscapeThroughFilesystemLinks(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	outsideDir := t.TempDir()

	mustCreatePortableBundledDirLink(t, outsideDir, filepath.Join(targetDir, "scripts"))

	cfg := &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{
				portableBundledFixtureFile(interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'unsafe'\n"),
			},
		},
	}
	canonical := flattenPortableBundledTestFactory(t, cfg)

	err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile))
	if err == nil {
		t.Fatal("expected bundled file target to fail when a filesystem link escapes the expand target")
	}
	if !strings.Contains(err.Error(), "cannot escape the expand target through filesystem links") {
		t.Fatalf("error = %q, want filesystem-link escape validation message", err.Error())
	}
	if !strings.Contains(err.Error(), "factory/scripts/execute-story.ps1") {
		t.Fatalf("error = %q, want offending target location", err.Error())
	}
	if _, statErr := os.Stat(filepath.Join(outsideDir, "scripts", "execute-story.ps1")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no escaped bundled file write outside target dir, stat err = %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, interfaces.FactoryConfigFile)); !os.IsNotExist(statErr) {
		t.Fatalf("expected no expanded factory config to be written before validation, stat err = %v", statErr)
	}
}

func writePortableBundledTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func flattenPortableBundledTestFactory(t *testing.T, cfg *interfaces.FactoryConfig) []byte {
	t.Helper()

	canonical, err := NewFactoryConfigMapper().Flatten(cfg)
	if err != nil {
		t.Fatalf("flatten portable bundled test factory: %v", err)
	}
	return canonical
}

func portableBundledFixtureFile(fileType, targetPath, inline string) interfaces.BundledFileConfig {
	return interfaces.BundledFileConfig{
		Type:       fileType,
		TargetPath: targetPath,
		Content: interfaces.BundledFileContentConfig{
			Encoding: interfaces.BundledFileEncodingUTF8,
			Inline:   inline,
		},
	}
}

func assertPortableBundledEntry(t *testing.T, bundledFile interfaces.BundledFileConfig, wantType, wantTargetPath, wantInline string) {
	t.Helper()

	if bundledFile.Type != wantType {
		t.Fatalf("bundled file type = %q, want %q", bundledFile.Type, wantType)
	}
	if bundledFile.TargetPath != wantTargetPath {
		t.Fatalf("bundled file targetPath = %q, want %q", bundledFile.TargetPath, wantTargetPath)
	}
	if bundledFile.Content.Encoding != interfaces.BundledFileEncodingUTF8 {
		t.Fatalf("bundled file encoding = %q, want %q", bundledFile.Content.Encoding, interfaces.BundledFileEncodingUTF8)
	}
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf("bundled file inline = %q, want %q", bundledFile.Content.Inline, wantInline)
	}
}

func assertPortableBundledExpandedFile(t *testing.T, targetDir, relativePath, want string) {
	t.Helper()

	path := filepath.Join(targetDir, relativePath)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, string(got), want)
	}
}

func assertPortableBundledExecutableScriptMode(t *testing.T, path string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("%s mode = %#o, want executable bit set", path, info.Mode().Perm())
	}
}

func mustCreatePortableBundledDirLink(t *testing.T, targetPath, linkPath string) {
	t.Helper()

	if err := os.Symlink(targetPath, linkPath); err == nil {
		return
	} else if runtime.GOOS != "windows" {
		t.Fatalf("Symlink(%s -> %s): %v", linkPath, targetPath, err)
	}

	cmd := exec.Command("cmd", "/c", "mklink", "/J", linkPath, targetPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J %s %s: %v (%s)", linkPath, targetPath, err, strings.TrimSpace(string(output)))
	}
}
