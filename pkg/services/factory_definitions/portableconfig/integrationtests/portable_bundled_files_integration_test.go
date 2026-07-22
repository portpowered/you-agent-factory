package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestPortableBundledFiles_FlattenAndLoadIncludeNestedDocsUnderFactoryDocs(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	const nestedDocPath = "factory/docs/standards/review.md"
	const nestedDocBody = "# Review standards\n"
	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, "docs", "standards", "review.md"), nestedDocBody)

	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, interfaces.FactoryConfigFile), `{
  "name":"portable-bundled-roundtrip-factory",
  "supportingFiles":{
    "bundledFiles":[
      {"type":"DOC","targetPath":"factory/docs/README.md","content":{}}
    ]
  },
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers": [{"name":"executor"}],
  "workstations": [{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":[{"workType":"task","state":"failed"}]
  }]
}`)

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatal("expected flattened config to include resourceManifest")
	}
	assertBundledFileRoundTripEntry(
		t,
		findPortableBundledFileByTarget(t, cfg.ResourceManifest.BundledFiles, nestedDocPath),
		interfaces.BundledFileTypeDoc,
		nestedDocPath,
		nestedDocBody,
	)

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	copyPortableBundledDiskBackedExport(t, projectDir, sourceDir, portableDir)
	copyPortableBundledExportFile(t, filepath.Join(sourceDir, "docs", "standards", "review.md"), filepath.Join(portableDir, "docs", "standards", "review.md"))

	loaded, err := loadPortableRuntimeConfig(portableDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(nested docs): %v", err)
	}
	if loaded.FactoryConfig() == nil || loaded.FactoryConfig().ResourceManifest == nil {
		t.Fatal("expected loaded config to include resourceManifest")
	}
	nested := findPortableBundledFileByTarget(t, loaded.FactoryConfig().ResourceManifest.BundledFiles, nestedDocPath)
	if nested.TargetPath != nestedDocPath {
		t.Fatalf("loaded nested doc target = %q, want %q", nested.TargetPath, nestedDocPath)
	}
	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "docs", "standards", "review.md"), nestedDocBody)
}

func TestPortableBundledFiles_FlattenOmitsGitkeepFromExportPayload(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, "scripts", ".gitkeep"), "")
	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, "inputs", "task", "default", ".gitkeep"), "")
	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, "inputs", "BATCH", "default", ".gitkeep"), "")

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatal("expected flattened config to include resourceManifest")
	}
	if len(cfg.ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("expected 3 bundled files (gitkeep omitted), got %#v", cfg.ResourceManifest.BundledFiles)
	}
	assertPortableBundledFilesExcludeGitkeep(t, cfg.ResourceManifest.BundledFiles)

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	copyPortableBundledDiskBackedExport(t, projectDir, sourceDir, portableDir)

	loaded, err := loadPortableRuntimeConfig(portableDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(standalone portable config): %v", err)
	}
	if loaded.FactoryConfig() == nil || loaded.FactoryConfig().ResourceManifest == nil {
		t.Fatal("expected loaded config to include resourceManifest")
	}
	assertPortableBundledFilesExcludeGitkeep(t, loaded.FactoryConfig().ResourceManifest.BundledFiles)
}

func TestPortableBundledFiles_RoundTripAcrossFlattenAndExpand(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatal("expected flattened config to include resourceManifest")
	}
	if len(cfg.ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("expected 3 bundled files, got %#v", cfg.ResourceManifest.BundledFiles)
	}
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[0], interfaces.BundledFileTypeDoc, "factory/docs/README.md", "# Portable factory\n")
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[1], interfaces.BundledFileTypeInput, "factory/inputs/task/default/starter.md", "starter work\n")
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[2], interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n")

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	copyPortableBundledDiskBackedExport(t, projectDir, sourceDir, portableDir)

	targetDir, err := expandPortableFactoryConfigLayout(portablePath)
	if err != nil {
		t.Fatalf("ExpandFactoryConfigLayout: %v", err)
	}

	assertPortableBundledRoundTripFile(t, filepath.Join(targetDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(targetDir, "docs", "README.md"), "# Portable factory\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(targetDir, "inputs", "task", "default", "starter.md"), "starter work\n")
	assertPortableBundledRoundTripScriptExecutable(t, filepath.Join(targetDir, "scripts", "execute-story.ps1"))
	if _, err := os.Stat(filepath.Join(targetDir, "Makefile")); !os.IsNotExist(err) {
		t.Fatalf("expected expand to omit implicit Makefile, stat err = %v", err)
	}
	assertPortableBundledPersistedThinManifestWithoutMakefile(t, filepath.Join(targetDir, interfaces.FactoryConfigFile))
	if _, err := os.Stat(filepath.Join(targetDir, "workers", "executor", interfaces.FactoryAgentsFileName)); err != nil {
		t.Fatalf("expected expanded worker AGENTS.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "workstations", "execute-story", interfaces.FactoryAgentsFileName)); err != nil {
		t.Fatalf("expected expanded workstation AGENTS.md: %v", err)
	}

	loaded, err := loadPortableRuntimeConfig(targetDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded layout): %v", err)
	}
	assertPortableBundledLoadedWorker(t, loaded)
}

func TestPortableBundledFiles_LoadRuntimeConfigMaterializesStandalonePortableConfig(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	copyPortableBundledDiskBackedExport(t, projectDir, sourceDir, portableDir)

	loaded, err := loadPortableRuntimeConfig(portableDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(standalone portable config): %v", err)
	}

	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "docs", "README.md"), "# Portable factory\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "inputs", "task", "default", "starter.md"), "starter work\n")
	assertPortableBundledRoundTripScriptExecutable(t, filepath.Join(portableDir, "scripts", "execute-story.ps1"))
	if _, err := os.Stat(filepath.Join(portableDir, "Makefile")); !os.IsNotExist(err) {
		t.Fatalf("expected standalone portable load to omit implicit Makefile, stat err = %v", err)
	}
	assertPortableBundledLoadedWorker(t, loaded)
}

func TestPortableBundledFiles_LoadDoesNotCreateMakefileWhenManifestOmitsIt(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactoryWithoutMakefile(t)

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	copyPortableBundledDiskBackedExport(t, projectDir, sourceDir, portableDir)

	loaded, err := loadPortableRuntimeConfig(portableDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(standalone portable config): %v", err)
	}
	if _, err := os.Stat(filepath.Join(portableDir, "Makefile")); !os.IsNotExist(err) {
		t.Fatalf("expected load to omit implicit Makefile, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "Makefile")); !os.IsNotExist(err) {
		t.Fatalf("expected load to avoid creating project-root Makefile, stat err = %v", err)
	}
	assertPortableBundledReplacementPaths(t, loaded.PortableBundledFileReplacements(), nil)
}

func TestPortableBundledFiles_LoadDoesNotReportMakefileReplacementWhenManifestOmitsIt(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	copyPortableBundledDiskBackedExport(t, projectDir, sourceDir, portableDir)
	writePortableBundledRoundTripFile(t, filepath.Join(portableDir, "Makefile"), "stale makefile\n")

	loaded, err := loadPortableRuntimeConfig(portableDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(standalone portable config with stale Makefile): %v", err)
	}
	assertPortableBundledReplacementPaths(t, loaded.PortableBundledFileReplacements(), nil)
	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "Makefile"), "stale makefile\n")
}

func TestPortableBundledFiles_ExpandRestoresExplicitMakefileFromManifest(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, interfaces.FactoryConfigFile), `{
  "name":"portable-bundled-roundtrip-factory",
  "supportingFiles":{
    "bundledFiles":[
      {"type":"ROOT_HELPER","targetPath":"Makefile","content":{"encoding":"utf-8","inline":"test:\n\tgo test ./...\n"}},
      {"type":"DOC","targetPath":"factory/docs/README.md","content":{}},
      {"type":"INPUT","targetPath":"factory/inputs/task/default/starter.md","content":{}},
      {"type":"SCRIPT","targetPath":"factory/scripts/execute-story.ps1","content":{}}
    ]
  },
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers": [{"name":"executor"}],
  "workstations": [{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":[{"workType":"task","state":"failed"}]
  }]
}`)
	copyPortableBundledDiskBackedExport(t, projectDir, sourceDir, sourceDir)

	targetDir, report, err := expandPortableFactoryConfigLayoutWithReport(
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)
	if err != nil {
		t.Fatalf("ExpandFactoryConfigLayoutWithExpansionReport: %v", err)
	}
	assertPortableBundledRoundTripFile(t, filepath.Join(targetDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableBundledRoundTripFileMode(t, filepath.Join(targetDir, "Makefile"), 0o644)
	if len(report.BundledReplacements) != 0 {
		t.Fatalf("bundled replacements = %#v, want none for fresh explicit Makefile materialization", report.BundledReplacements)
	}
}

func TestPortableBundledFiles_LoadRuntimeConfigOverwritesDifferingExistingFile(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	portableDir := t.TempDir()
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	copyPortableBundledDiskBackedExport(t, projectDir, sourceDir, portableDir)
	writePortableBundledRoundTripFile(t, filepath.Join(portableDir, "scripts", "execute-story.ps1"), "Write-Output 'stale script'\n")
	writePortableBundledRoundTripFile(t, filepath.Join(portableDir, "inputs", "task", "default", "starter.md"), "stale starter work\n")

	loaded, err := loadPortableRuntimeConfig(portableDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(standalone portable config with stale file): %v", err)
	}
	replacements := loaded.PortableBundledFileReplacements()
	if len(replacements) != 2 {
		t.Fatalf("portable bundled replacements = %#v, want two replacements", replacements)
	}
	assertPortableBundledReplacementPaths(t, replacements, []string{
		"factory/inputs/task/default/starter.md",
		"factory/scripts/execute-story.ps1",
	})

	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPortableBundledRoundTripFile(t, filepath.Join(portableDir, "inputs", "task", "default", "starter.md"), "starter work\n")
	assertPortableBundledLoadedWorker(t, loaded)
}

func TestPortableBundledFiles_LoadRuntimeConfigAcceptsThinDiskBackedManifest(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, interfaces.FactoryConfigFile), `{
  "name":"portable-bundled-roundtrip-factory",
  "supportingFiles":{
    "bundledFiles":[
      {"type":"ROOT_HELPER","targetPath":"Makefile","content":{}},
      {"type":"DOC","targetPath":"factory/docs/README.md","content":{}},
      {"type":"INPUT","targetPath":"factory/inputs/task/default/starter.md","content":{}},
      {"type":"SCRIPT","targetPath":"factory/scripts/execute-story.ps1","content":{}}
    ]
  },
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers": [{"name":"executor"}],
  "workstations": [{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":[{"workType":"task","state":"failed"}]
  }]
}`)
	writePortableBundledRoundTripFile(t, filepath.Join(projectDir, "Makefile"), "test:\n\tgo test ./...\n")

	loaded, err := loadPortableRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(thin disk-backed manifest): %v", err)
	}

	assertPortableBundledLoadedWorker(t, loaded)
	assertPortableBundledRoundTripFile(t, filepath.Join(sourceDir, "inputs", "task", "default", "starter.md"), "starter work\n")
	assertPortableBundledLoadedThinManifest(t, loaded.FactoryConfig())
}

func TestPortableBundledFiles_FlattenRehydratesThinDiskBackedManifestFromDisk(t *testing.T) {
	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)

	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, interfaces.FactoryConfigFile), `{
  "name":"portable-bundled-roundtrip-factory",
  "supportingFiles":{
    "bundledFiles":[
      {"type":"ROOT_HELPER","targetPath":"Makefile","content":{"encoding":"utf-8","inline":"stale makefile\n"}},
      {"type":"DOC","targetPath":"factory/docs/README.md","content":{}},
      {"type":"INPUT","targetPath":"factory/inputs/task/default/starter.md","content":{"encoding":"utf-8","inline":"stale starter work\n"}},
      {"type":"SCRIPT","targetPath":"factory/scripts/execute-story.ps1","content":{}}
    ]
  },
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers": [{"name":"executor"}],
  "workstations": [{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":[{"workType":"task","state":"failed"}]
  }]
}`)
	writePortableBundledRoundTripFile(t, filepath.Join(projectDir, "Makefile"), "test:\n\tgo test ./...\n")

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(thin disk-backed manifest): %v", err)
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON(flattened): %v", err)
	}
	if cfg.ResourceManifest == nil || len(cfg.ResourceManifest.BundledFiles) != 4 {
		t.Fatalf("flattened bundled files = %#v, want 4 bundled files", cfg.ResourceManifest)
	}
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[0], interfaces.BundledFileTypeRootHelper, "Makefile", "test:\n\tgo test ./...\n")
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[1], interfaces.BundledFileTypeDoc, "factory/docs/README.md", "# Portable factory\n")
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[2], interfaces.BundledFileTypeInput, "factory/inputs/task/default/starter.md", "starter work\n")
	assertBundledFileRoundTripEntry(t, cfg.ResourceManifest.BundledFiles[3], interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n")
}

func seedPortableBundledRoundTripFactoryWithoutMakefile(t *testing.T) (string, string) {
	t.Helper()

	projectDir, sourceDir := seedPortableBundledRoundTripFactory(t)
	if err := os.Remove(filepath.Join(projectDir, "Makefile")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Remove(project Makefile): %v", err)
	}
	return projectDir, sourceDir
}

func seedPortableBundledRoundTripFactory(t *testing.T) (string, string) {
	t.Helper()

	projectDir := t.TempDir()
	sourceDir := filepath.Join(projectDir, "factory")

	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, interfaces.FactoryConfigFile), `{
  "name":"portable-bundled-roundtrip-factory",
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers": [{"name":"executor"}],
  "workstations": [{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":[{"workType":"task","state":"failed"}]
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
	writePortableBundledRoundTripFile(t, filepath.Join(sourceDir, "inputs", "task", "default", "starter.md"), "starter work\n")
	writePortableBundledRoundTripFile(t, filepath.Join(projectDir, "Makefile"), "test:\n\tgo test ./...\n")
	return projectDir, sourceDir
}

func assertPortableBundledLoadedWorker(t *testing.T, loaded interfaces.MutableLoadedFactorySource) {
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

func assertPortableBundledLoadedThinManifest(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

	if cfg == nil || cfg.ResourceManifest == nil {
		t.Fatal("expected loaded config to include resourceManifest")
	}
	if len(cfg.ResourceManifest.BundledFiles) != 4 {
		t.Fatalf("expected 4 bundled files, got %#v", cfg.ResourceManifest.BundledFiles)
	}
	assertBundledFileRoundTripEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[0], interfaces.BundledFileTypeRootHelper, "Makefile")
	assertBundledFileRoundTripEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[1], interfaces.BundledFileTypeDoc, "factory/docs/README.md")
	assertBundledFileRoundTripEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[2], interfaces.BundledFileTypeInput, "factory/inputs/task/default/starter.md")
	assertBundledFileRoundTripEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[3], interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1")
}

func assertPortableBundledLoadedThinManifestWithoutMakefile(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

	if cfg == nil || cfg.ResourceManifest == nil {
		t.Fatal("expected loaded config to include resourceManifest")
	}
	if len(cfg.ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("expected 3 bundled files, got %#v", cfg.ResourceManifest.BundledFiles)
	}
	assertBundledFileRoundTripEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[0], interfaces.BundledFileTypeDoc, "factory/docs/README.md")
	assertBundledFileRoundTripEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[1], interfaces.BundledFileTypeInput, "factory/inputs/task/default/starter.md")
	assertBundledFileRoundTripEntryWithoutInline(t, cfg.ResourceManifest.BundledFiles[2], interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1")
}

func assertPortableBundledPersistedThinManifestWithoutMakefile(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(data)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON(%s): %v", path, err)
	}
	assertPortableBundledLoadedThinManifestWithoutMakefile(t, cfg)
}

func TestExpandPortableBundledFiles_RejectsUnsafeTargetWithoutEscapedWrite(t *testing.T) {
	tests := []struct {
		name            string
		fileType        string
		targetPath      func(t *testing.T, portableDir string) string
		outsidePath     func(portableDir, unsafeTarget string) string
		wantErrContains string
	}{
		{
			name:     "absolute target location",
			fileType: interfaces.BundledFileTypeScript,
			targetPath: func(t *testing.T, _ string) string {
				return filepath.Join(t.TempDir(), "outside.ps1")
			},
			outsidePath: func(_ string, unsafeTarget string) string {
				return unsafeTarget
			},
			wantErrContains: "must be factory-relative, not absolute",
		},
		{
			name:     "escaping target location",
			fileType: interfaces.BundledFileTypeScript,
			targetPath: func(_ *testing.T, _ string) string {
				return "../outside.ps1"
			},
			outsidePath: func(portableDir, _ string) string {
				return filepath.Join(filepath.Dir(portableDir), "outside.ps1")
			},
			wantErrContains: "cannot escape the factory root",
		},
		{
			name:     "unsupported root helper",
			fileType: interfaces.BundledFileTypeRootHelper,
			targetPath: func(_ *testing.T, _ string) string {
				return "README.md"
			},
			outsidePath: func(portableDir, _ string) string {
				return filepath.Join(portableDir, "factory", "README.md")
			},
			wantErrContains: "must be one of the supported root helper files",
		},
		{
			name:     "input nested past starter file",
			fileType: interfaces.BundledFileTypeInput,
			targetPath: func(_ *testing.T, _ string) string {
				return "factory/inputs/task/default/nested/starter.md"
			},
			outsidePath: func(portableDir, _ string) string {
				return filepath.Join(portableDir, "factory", "inputs", "task", "default", "nested", "starter.md")
			},
			wantErrContains: "must use factory/inputs/<work-type>/<channel>/<file>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portableDir := t.TempDir()
			unsafeTarget := tt.targetPath(t, portableDir)
			outsidePath := tt.outsidePath(portableDir, unsafeTarget)

			writePortableBundledRuntimeFixture(t, portableDir, []interfaces.BundledFileConfig{
				portableBundledFileFixture(interfaces.BundledFileTypeScript, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n"),
				portableBundledFileFixture(tt.fileType, unsafeTarget, "Write-Output 'unsafe'\n"),
			})

			_, err := expandPortableFactoryConfigLayout(
				filepath.Join(portableDir, interfaces.FactoryConfigFile),
			)
			if err == nil {
				t.Fatal("expected ExpandFactoryConfigLayout to reject unsafe bundled file target")
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErrContains)
			}
			if !strings.Contains(err.Error(), filepath.Base(unsafeTarget)) {
				t.Fatalf("error = %q, want offending target file %q", err.Error(), filepath.Base(unsafeTarget))
			}
			if _, statErr := os.Stat(outsidePath); !os.IsNotExist(statErr) {
				t.Fatalf("expected no escaped bundled file write at %s, stat err = %v", outsidePath, statErr)
			}
			if _, statErr := os.Stat(filepath.Join(portableDir, "factory", "scripts", "execute-story.ps1")); !os.IsNotExist(statErr) {
				t.Fatalf("expected no bundled script write before validation fails, stat err = %v", statErr)
			}
		})
	}
}

func TestLoadPortableBundledFiles_RejectsFilesystemLinkEscapeWithoutEscapedWrite(t *testing.T) {
	portableDir := t.TempDir()
	outsideDir := t.TempDir()

	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, []byte(`{
  "name":"portable-bundled-runtime-fixture",
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers": [{"name":"executor"}],
  "workstations": [{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":[{"workType":"task","state":"failed"}]
  }],
  "supportingFiles":{
    "bundledFiles":[
      {"type":"SCRIPT","targetPath":"factory/scripts/execute-story.ps1","content":{"encoding":"utf-8","inline":"Write-Output 'unsafe'\n"}}
    ]
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
	mustCreatePortableBundledDirLinkExternal(t, outsideDir, filepath.Join(portableDir, "scripts"))

	_, err := loadPortableRuntimeConfig(portableDir, nil)
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

func loadPortableRuntimeConfig(
	factoryDir string,
	workstationLoader interfaces.WorkstationLoader,
) (interfaces.MutableLoadedFactorySource, error) {
	return factorydefinitioncomposition.LoadDirectory(
		factoryDir,
		workstationLoader,
	)
}

func expandPortableFactoryConfigLayout(path string) (string, error) {
	targetDir, _, err := expandPortableFactoryConfigLayoutWithReport(path)
	return targetDir, err
}

func expandPortableFactoryConfigLayoutWithReport(
	path string,
) (string, interfaces.LayoutExpansionReport, error) {
	return factorydefinitioncomposition.ExpandLayout(path)
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

func assertPortableBundledRoundTripFileMode(t *testing.T, path string, wantPerm os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if info.Mode().Perm() != wantPerm {
		t.Fatalf("%s mode = %#o, want %#o", path, info.Mode().Perm(), wantPerm)
	}
}

func assertPortableBundledFilesExcludeGitkeep(t *testing.T, bundledFiles []interfaces.BundledFileConfig) {
	t.Helper()

	for _, bundledFile := range bundledFiles {
		if strings.HasSuffix(bundledFile.TargetPath, ".gitkeep") ||
			strings.Contains(bundledFile.TargetPath, "/.gitkeep") {
			t.Fatalf("bundled file targetPath must not include .gitkeep: %#v", bundledFile)
		}
	}
}

func findPortableBundledFileByTarget(t *testing.T, bundledFiles []interfaces.BundledFileConfig, targetPath string) interfaces.BundledFileConfig {
	t.Helper()

	for _, bundledFile := range bundledFiles {
		if bundledFile.TargetPath == targetPath {
			return bundledFile
		}
	}
	t.Fatalf("bundled files missing target %q: %#v", targetPath, bundledFiles)
	return interfaces.BundledFileConfig{}
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

func assertBundledFileRoundTripEntryWithoutInline(t *testing.T, bundledFile interfaces.BundledFileConfig, wantType, wantTargetPath string) {
	t.Helper()

	if bundledFile.Type != wantType {
		t.Fatalf("bundled file type = %q, want %q", bundledFile.Type, wantType)
	}
	if bundledFile.TargetPath != wantTargetPath {
		t.Fatalf("bundled file targetPath = %q, want %q", bundledFile.TargetPath, wantTargetPath)
	}
	if bundledFile.Content.Encoding != "" && bundledFile.Content.Encoding != interfaces.BundledFileEncodingUTF8 {
		t.Fatalf("bundled file encoding = %q, want empty or %q", bundledFile.Content.Encoding, interfaces.BundledFileEncodingUTF8)
	}
	if bundledFile.Content.Inline != "" {
		t.Fatalf("expected bundled file inline content to be omitted after canonical export, got %q", bundledFile.Content.Inline)
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

func writePortableBundledRuntimeFixture(t *testing.T, portableDir string, bundledFiles []interfaces.BundledFileConfig) {
	t.Helper()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: bundledFiles,
		},
	}

	mapper := factorymapping.NewFactoryConfigMapper()
	canonical, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	portablePath := filepath.Join(portableDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(portablePath, canonical, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", portablePath, err)
	}
}

func copyPortableBundledDiskBackedExport(t *testing.T, projectDir, sourceDir, portableDir string) {
	t.Helper()

	copyPortableBundledExportFile(t, filepath.Join(sourceDir, "docs", "README.md"), filepath.Join(portableDir, "docs", "README.md"))
	copyPortableBundledExportFile(t, filepath.Join(sourceDir, "inputs", "task", "default", "starter.md"), filepath.Join(portableDir, "inputs", "task", "default", "starter.md"))
	copyPortableBundledExportFile(t, filepath.Join(sourceDir, "scripts", "execute-story.ps1"), filepath.Join(portableDir, "scripts", "execute-story.ps1"))
}

func assertPortableBundledReplacementPaths(t *testing.T, replacements []interfaces.PortableBundledFileReplacement, want []string) {
	t.Helper()

	if len(replacements) != len(want) {
		t.Fatalf("portable bundled replacements = %#v, want %#v", replacements, want)
	}
	for i := range replacements {
		if replacements[i].TargetPath != want[i] {
			t.Fatalf("portable bundled replacement[%d] = %q, want %q", i, replacements[i].TargetPath, want[i])
		}
	}
}

func copyPortableBundledExportFile(t *testing.T, sourcePath, targetPath string) {
	t.Helper()

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(targetPath), err)
	}
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", targetPath, err)
	}
}

func mustCreatePortableBundledDirLinkExternal(t *testing.T, targetPath, linkPath string) {
	t.Helper()

	if err := os.Symlink(targetPath, linkPath); err == nil {
		return
	} else if runtime.GOOS != "windows" {
		t.Fatalf("Symlink(%s -> %s): %v", linkPath, targetPath, err)
	}

	t.Skip("Windows symlink privilege unavailable; junctions do not exercise symlink resolution semantics")
}
