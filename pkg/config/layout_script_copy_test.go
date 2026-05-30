package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWriteExpandedFactoryLayout_CopiesReferencedScriptForOptedInScriptWorkstation(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	cfg := portableScriptFactoryConfig(true, "python3", []string{"scripts/setup-workspace.py", "--mode", "portable"})
	canonical := flattenLayoutTestFactory(t, cfg)
	scriptPath := filepath.Join(sourceDir, "scripts", "setup-workspace.py")
	writeLayoutScriptTestFile(t, scriptPath, "#!/usr/bin/env python3\nprint('portable setup')\n")

	if _, err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("writeExpandedFactoryLayout: %v", err)
	}

	copiedPath := filepath.Join(targetDir, "scripts", "setup-workspace.py")
	copied, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("read copied script: %v", err)
	}
	if string(copied) != "#!/usr/bin/env python3\nprint('portable setup')\n" {
		t.Fatalf("copied script content = %q", string(copied))
	}

	loaded, err := LoadRuntimeConfig(targetDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded layout): %v", err)
	}
	worker, ok := loaded.Worker("workspace-setup")
	if !ok {
		t.Fatal("expected copied-script worker definition to load")
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "python3" {
		t.Fatalf("loaded worker = %#v", worker)
	}
	if len(worker.Args) < 1 || worker.Args[0] != "scripts/setup-workspace.py" {
		t.Fatalf("loaded worker args = %#v", worker.Args)
	}
}

func TestWriteExpandedFactoryLayout_DoesNotCopyReferencedScriptWhenOptOut(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	cfg := portableScriptFactoryConfig(false, "powershell", []string{"-File", "scripts/execute-story.ps1"})
	canonical := flattenLayoutTestFactory(t, cfg)
	writeLayoutScriptTestFile(t, filepath.Join(sourceDir, "scripts", "execute-story.ps1"), "Write-Output 'portable'\n")

	if _, err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("writeExpandedFactoryLayout: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "scripts", "execute-story.ps1")); !os.IsNotExist(err) {
		t.Fatalf("expected referenced script not to be copied, stat err = %v", err)
	}
}

func TestWriteExpandedFactoryLayout_SkipsInterpreterFlagValuesBeforeScriptPath(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	cfg := portableScriptFactoryConfig(true, "node", []string{"--loader", "ts-node/esm", "scripts/run.ts"})
	canonical := flattenLayoutTestFactory(t, cfg)
	writeLayoutScriptTestFile(t, filepath.Join(sourceDir, "scripts", "run.ts"), "console.log('portable');\n")

	if _, err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("writeExpandedFactoryLayout: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "ts-node", "esm")); !os.IsNotExist(err) {
		t.Fatalf("expected loader value not to be copied, stat err = %v", err)
	}

	copiedPath := filepath.Join(targetDir, "scripts", "run.ts")
	copied, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("read copied script: %v", err)
	}
	if string(copied) != "console.log('portable');\n" {
		t.Fatalf("copied script content = %q", string(copied))
	}
}

func TestWriteExpandedFactoryLayout_RejectsUnsafeReferencedScriptPaths(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		want    string
	}{
		{
			name:    "absolute command path",
			command: "__ABSOLUTE__",
			want:    "must be relative to the factory directory",
		},
		{
			name:    "escaping script arg path",
			command: "python",
			args:    []string{"../scripts/setup.py"},
			want:    "cannot escape the factory directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceDir := t.TempDir()
			targetDir := t.TempDir()
			command := tt.command
			if command == "__ABSOLUTE__" {
				command = filepath.Join(t.TempDir(), "setup.py")
			}
			cfg := portableScriptFactoryConfig(true, command, tt.args)
			canonical := flattenLayoutTestFactory(t, cfg)

			_, err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile))
			if err == nil {
				t.Fatal("expected unsafe referenced script path to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func portableScriptFactoryConfig(copyReferencedScripts bool, command string, args []string) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []interfaces.ResourceConfig{},
		Workers: []interfaces.WorkerConfig{
			{
				Name:    "workspace-setup",
				Type:    interfaces.WorkerTypeScript,
				Command: command,
				Args:    append([]string(nil), args...),
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:                  "setup-workspace",
				Type:                  interfaces.WorkstationTypeModel,
				WorkerTypeName:        "workspace-setup",
				CopyReferencedScripts: copyReferencedScripts,
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "task", StateName: "init"},
				},
				Outputs: []interfaces.IOConfig{
					{WorkTypeName: "task", StateName: "complete"},
				},
			},
		},
	}
}

func flattenLayoutTestFactory(t *testing.T, cfg *interfaces.FactoryConfig) []byte {
	t.Helper()

	canonical, err := NewFactoryConfigMapper().Flatten(cfg)
	if err != nil {
		t.Fatalf("flatten test factory: %v", err)
	}
	return canonical
}

func writeLayoutScriptTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

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
    "onFailure":[{"workType":"task","state":"failed"}]
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

func TestFlattenFactoryConfig_CheckedInFactoryBundlesOverviewDoc(t *testing.T) {
	factoryConfigPath := testpath.MustRepoPathFromCaller(t, 0, "factory", interfaces.FactoryConfigFile)

	flattened, err := FlattenFactoryConfig(factoryConfigPath)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(%s): %v", factoryConfigPath, err)
	}

	cfg, err := FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatal("expected checked-in factory flatten to include bundled files")
	}

	var overview interfaces.BundledFileConfig
	for _, bundledFile := range cfg.ResourceManifest.BundledFiles {
		if bundledFile.TargetPath == "factory/docs/overview.md" {
			overview = bundledFile
			break
		}
	}
	if overview.TargetPath == "" {
		t.Fatalf("expected factory/docs/overview.md in bundled files, got %#v", cfg.ResourceManifest.BundledFiles)
	}
	if overview.Type != interfaces.BundledFileTypeDoc {
		t.Fatalf("overview bundled type = %q, want %q", overview.Type, interfaces.BundledFileTypeDoc)
	}
	for _, want := range []string{
		"# Repository Maintainer Factory Overview",
		"you run --dir ./factory",
		"thoughts:init",
		"factory/inputs/BATCH/default/",
		"you docs agents",
	} {
		if !strings.Contains(overview.Content.Inline, want) {
			t.Fatalf("overview inline missing %q:\n%s", want, overview.Content.Inline)
		}
	}
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
	authoredCfg, err := authoredFactoryConfigForExpandedLayout(cfg)
	if err != nil {
		t.Fatalf("authoredFactoryConfigForExpandedLayout: %v", err)
	}
	canonical := flattenPortableBundledTestFactory(t, authoredCfg)

	if _, err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("writeExpandedFactoryLayout: %v", err)
	}

	assertPortableBundledExpandedFile(t, targetDir, filepath.Join("docs", "README.md"), "# Portable factory\n")
	assertPortableBundledExpandedFile(t, targetDir, filepath.Join("scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPortableBundledExpandedFile(t, targetDir, "Makefile", "test:\n\tgo test ./...\n")
	assertPortableBundledExecutableScriptMode(t, filepath.Join(targetDir, "scripts", "execute-story.ps1"))
	assertPortableBundledPersistedThinManifest(t, filepath.Join(targetDir, interfaces.FactoryConfigFile))
}

func TestMaterializePortableBundledFiles_ReportsDifferingExistingFileReplacement(t *testing.T) {
	targetDir := t.TempDir()
	existingScriptPath := filepath.Join(targetDir, "scripts", "execute-story.ps1")
	writePortableBundledTestFile(t, existingScriptPath, "Write-Output 'stale script'\n")
	writePortableBundledTestFile(t, filepath.Join(targetDir, "docs", "README.md"), "# Portable factory\n")

	cfg := &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{
				portableBundledFixtureFile(interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n"),
				portableBundledFixtureFile(interfaces.BundledFileTypeDoc, "factory/docs/README.md", "# Portable factory\n"),
			},
		},
	}

	replacements, err := materializePortableBundledFiles(targetDir, cfg)
	if err != nil {
		t.Fatalf("materializePortableBundledFiles: %v", err)
	}
	if len(replacements) != 1 {
		t.Fatalf("replacement report = %#v, want one differing bundled file", replacements)
	}
	if replacements[0].TargetPath != "factory/scripts/execute-story.ps1" {
		t.Fatalf("replacement targetPath = %q, want factory/scripts/execute-story.ps1", replacements[0].TargetPath)
	}
	assertPortableBundledExpandedFile(t, targetDir, filepath.Join("scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPortableBundledExpandedFile(t, targetDir, filepath.Join("docs", "README.md"), "# Portable factory\n")
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
		fileType   string
		targetPath string
		want       string
	}{
		{
			name:       "absolute target location",
			fileType:   interfaces.BundledFileTypeScript,
			targetPath: filepath.Join(t.TempDir(), "outside.ps1"),
			want:       "must be factory-relative, not absolute",
		},
		{
			name:       "escaping target location",
			fileType:   interfaces.BundledFileTypeScript,
			targetPath: "../outside.ps1",
			want:       "cannot escape the factory root",
		},
		{
			name:       "script outside script root",
			fileType:   interfaces.BundledFileTypeScript,
			targetPath: "factory/docs/setup-workspace.py",
			want:       `must stay under "factory/scripts/" for SCRIPT bundled files`,
		},
		{
			name:       "unsupported root helper",
			fileType:   interfaces.BundledFileTypeRootHelper,
			targetPath: "README.md",
			want:       "must be one of the supported root helper files",
		},
		{
			name:       "input nested past starter file",
			fileType:   interfaces.BundledFileTypeInput,
			targetPath: "factory/inputs/task/default/nested/starter.md",
			want:       "must use factory/inputs/<work-type>/<channel>/<file>",
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
						portableBundledFixtureFile(tt.fileType, tt.targetPath, "Write-Output 'unsafe'\n"),
					},
				},
			}
			canonical := flattenPortableBundledTestFactory(t, cfg)

			_, err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile))
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

	_, err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile))
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

func assertPortableBundledPersistedThinManifest(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	cfg, err := FactoryConfigFromOpenAPIJSON(data)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON(%s): %v", path, err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatalf("%s resourceManifest = nil, want bundled files", path)
	}
	if len(cfg.ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("%s bundled files = %#v, want 3 entries", path, cfg.ResourceManifest.BundledFiles)
	}
	assertPortableBundledEntry(t, cfg.ResourceManifest.BundledFiles[0], interfaces.BundledFileTypeRootHelper, "Makefile", "test:\n\tgo test ./...\n")
	assertPortableBundledEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[1], interfaces.BundledFileTypeDoc, "factory/docs/README.md")
	assertPortableBundledEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[2], interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1")
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

func assertPortableBundledEntryWithoutInline(t *testing.T, bundledFile interfaces.BundledFileConfig, wantType, wantTargetPath string) {
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
	if bundledFile.Content.Inline != "" {
		t.Fatalf("bundled file inline = %q, want omitted content", bundledFile.Content.Inline)
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
