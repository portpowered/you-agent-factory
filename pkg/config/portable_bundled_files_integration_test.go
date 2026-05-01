package config_test

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPortableBundledFiles_RoundTripAcrossFlattenAndExpand(t *testing.T) {
	_, sourceDir := seedPortableBundledRoundTripFactory(t)

	flattened, err := factoryconfig.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatal("expected flattened config to include resourceManifest")
	}
	if len(cfg.ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("expected 3 bundled files, got %#v", cfg.ResourceManifest.BundledFiles)
	}
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[0], interfaces.BundledFileTypeRootHelper, "Makefile", "test:\n\tgo test ./...\n")
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[1], interfaces.BundledFileTypeDoc, "factory/docs/README.md", "# Portable factory\n")
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[2], interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n")

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}

	targetDir, err := factoryconfig.ExpandFactoryConfigLayout(portablePath)
	if err != nil {
		t.Fatalf("ExpandFactoryConfigLayout: %v", err)
	}

	assertPortableBundledRoundTripFile(t, filepath.Join(targetDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(targetDir, "docs", "README.md"), "# Portable factory\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(targetDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableBundledRoundTripScriptExecutable(t, filepath.Join(targetDir, "scripts", "execute-story.ps1"))
	if _, err := os.Stat(filepath.Join(targetDir, "workers", "executor", interfaces.FactoryAgentsFileName)); err != nil {
		t.Fatalf("expected expanded worker AGENTS.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "workstations", "execute-story", interfaces.FactoryAgentsFileName)); err != nil {
		t.Fatalf("expected expanded workstation AGENTS.md: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(targetDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded layout): %v", err)
	}
	assertPortableBundledLoadedWorker(t, loaded)
}

func TestPortableBundledFiles_LoadRuntimeConfigMaterializesStandalonePortableConfig(t *testing.T) {
	_, sourceDir := seedPortableBundledRoundTripFactory(t)

	flattened, err := factoryconfig.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(portableDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(standalone portable config): %v", err)
	}

	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "docs", "README.md"), "# Portable factory\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableBundledRoundTripScriptExecutable(t, filepath.Join(portableDir, "scripts", "execute-story.ps1"))
	assertPortableBundledLoadedWorker(t, loaded)
}

func seedPortableBundledRoundTripFactory(t *testing.T) (string, string) {
	t.Helper()

	projectDir := t.TempDir()
	sourceDir := filepath.Join(projectDir, "factory")

	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, interfaces.FactoryConfigFile), `{
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers": [{"name":"executor"}],
  "workstations": [{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":{"workType":"task","state":"failed"}
  }]
}`)
	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName), `---
type: SCRIPT_WORKER
command: powershell
args:
  - -File
  - scripts/execute-story.ps1
timeout: 45m
---
Execute the bundled script.
`)
	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, interfaces.WorkstationsDir, "execute-story", interfaces.FactoryAgentsFileName), `---
type: MODEL_WORKSTATION
worker: executor
---
Execute {{ (index .Inputs 0).WorkID }}.
`)
	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, "docs", "README.md"), "# Portable factory\n")
	writePortableBundledRoundTripFile(t, filepath.Join(projectDir, "Makefile"), "test:\n\tgo test ./...\n")
	return projectDir, sourceDir
}

func assertPortableBundledLoadedWorker(t *testing.T, loaded *factoryconfig.LoadedFactoryConfig) {
	t.Helper()

	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected expanded bundled worker definition to load")
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "powershell" {
		t.Fatalf("loaded worker = %#v", worker)
	}
	if len(worker.Args) != 2 || worker.Args[1] != "scripts/execute-story.ps1" {
		t.Fatalf("loaded worker args = %#v", worker.Args)
	}
}

func TestExpandPortableBundledFiles_RejectsUnsafeTargetWithoutEscapedWrite(t *testing.T) {
	portableDir := t.TempDir()
	escapeTarget := "..\\outside.ps1"
	outsidePath := filepath.Join(portableDir, "outside.ps1")

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure:      &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
		}},
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{
				portableBundledFileFixture(interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n"),
				portableBundledFileFixture(interfaces.BundledFileTypeScript, escapeTarget, "Write-Output 'unsafe'\n"),
			},
		},
	}

	mapper := factoryconfig.NewFactoryConfigMapper()
	canonical, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, canonical, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}

	_, err = factoryconfig.ExpandFactoryConfigLayout(portablePath)
	if err == nil {
		t.Fatal("expected ExpandFactoryConfigLayout to reject unsafe bundled file target")
	}
	if !strings.Contains(err.Error(), "must use forward slashes") {
		t.Fatalf("error = %q, want forward-slash validation message", err.Error())
	}
	if !strings.Contains(err.Error(), path.Base(strings.ReplaceAll(escapeTarget, `\`, `/`))) {
		t.Fatalf("error = %q, want offending target file %q", err.Error(), path.Base(strings.ReplaceAll(escapeTarget, `\`, `/`)))
	}
	if _, statErr := os.Stat(outsidePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no escaped bundled file write at %s, stat err = %v", outsidePath, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(portableDir, "factory", "scripts", "execute-story.ps1")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no bundled script write before validation fails, stat err = %v", statErr)
	}
}

func TestLoadPortableBundledFiles_RejectsFilesystemLinkEscapeWithoutEscapedWrite(t *testing.T) {
	portableDir := t.TempDir()
	outsideDir := t.TempDir()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure:      &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
		}},
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{
				portableBundledFileFixture(interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'unsafe'\n"),
			},
		},
	}

	mapper := factoryconfig.NewFactoryConfigMapper()
	canonical, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, canonical, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	mustCreatePortableBundledDirLinkExternal(t, outsideDir, filepath.Join(portableDir, "scripts"))

	_, err = factoryconfig.LoadRuntimeConfig(portableDir, nil)
	if err == nil {
		t.Fatal("expected LoadRuntimeConfig to reject filesystem-link escape target")
	}
	if !strings.Contains(err.Error(), "cannot escape the expand target through filesystem links") {
		t.Fatalf("error = %q, want filesystem-link escape validation message", err.Error())
	}
	if !strings.Contains(err.Error(), "factory/scripts/execute-story.ps1") {
		t.Fatalf("error = %q, want offending target location", err.Error())
	}
	if _, statErr := os.Stat(filepath.Join(outsideDir, "scripts", "execute-story.ps1")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no escaped bundled file write at %s, stat err = %v", filepath.Join(outsideDir, "scripts", "execute-story.ps1"), statErr)
	}
}

func writePortableBundledRoundTripFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertPortableBundledRoundTripFile(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, string(got), want)
	}
}

func assertPortableBundledRoundTripScriptExecutable(t *testing.T, path string) {
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

func assertBundledFileRoundTripEntry(t *testing.T, bundledFile interfaces.BundledFileConfig, wantType, wantTargetPath, wantInline string) {
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

func portableBundledFileFixture(fileType, targetPath, inline string) interfaces.BundledFileConfig {
	return interfaces.BundledFileConfig{
		Type:       fileType,
		TargetPath: targetPath,
		Content: interfaces.BundledFileContentConfig{
			Encoding: interfaces.BundledFileEncodingUTF8,
			Inline:   inline,
		},
	}
}

func mustCreatePortableBundledDirLinkExternal(t *testing.T, targetPath, linkPath string) {
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
