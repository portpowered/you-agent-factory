package wire_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
	internalportableconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestWireCompileLoadEquivalence_SuccessThroughDefinitionsRoot(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	writeWireCompileEquivalenceAuthoredFactory(t, factoryDir)

	root, loader := newWireRootWithCompileLoader(t)
	canonical, err := loader.FlattenFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	exerciseWireRootCompileSuccess(t, root, factoryDir, canonical)
}

func TestWireCompileLoadEquivalence_TypedFailuresThroughDefinitionsRoot(t *testing.T) {
	t.Parallel()

	root, _ := newWireRootWithCompileLoader(t)
	exerciseWireRootCompileTypedFailures(t, root)
}

func TestWireCompileLoadEquivalence_RuntimeConfigMergeThroughDefinitionsRoot(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	writeWireCompileEquivalenceRuntimeMergeFactory(t, factoryDir)

	root, loader := newWireRootWithCompileLoader(t)
	loaded, err := loader.LoadSourceFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadSourceFromFactoryDir: %v", err)
	}
	workstation, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected execute-story workstation from merged load")
	}
	if len(workstation.StopWords) < 2 {
		t.Fatalf("merged workstation stopWords = %v, want canonical and runtime values", workstation.StopWords)
	}
	if workstation.StopWords[0] != "CANONICAL" || workstation.StopWords[1] != "RUNTIME" {
		t.Fatalf("merged workstation stopWords = %v, want [CANONICAL RUNTIME]", workstation.StopWords)
	}

	compiled, err := root.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{FactoryDir: factoryDir},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource: %v", err)
	}

	facts := wireEffectiveWorkerAndWorkstationFacts(t, compiled.Effective.ContentIdentity)
	if facts.workerCommand != "go" {
		t.Fatalf("merged worker command = %q, want go from runtime definitions", facts.workerCommand)
	}
	if facts.workstationWorker != "executor" {
		t.Fatalf("merged workstation worker = %q, want executor", facts.workstationWorker)
	}
}

func TestValidatedAuthoredFactoryDefinitionLoader_SelectsExplicitSupportedSources(t *testing.T) {
	t.Parallel()

	loader := newCompileLoadLoader(t, platformfilesystem.Local{})
	service := newValidatedAuthoredFactoryDefinitionLoader(t, loader)
	tests := []struct {
		name       string
		rootName   string
		format     factorydefinitions.AuthoredFactoryFormat
		directory  bool
		runtimeDir string
	}{
		{name: "explicit JSON", rootName: "factory.json", format: factorydefinitions.AuthoredFactoryFormatJSON},
		{name: "explicit YAML", rootName: "factory.yaml", format: factorydefinitions.AuthoredFactoryFormatYAML},
		{name: "explicit YML", rootName: "factory.yml", format: factorydefinitions.AuthoredFactoryFormatYAML},
		{name: "direct directory", rootName: "factory.yaml", format: factorydefinitions.AuthoredFactoryFormatYAML, directory: true, runtimeDir: "runtime-base"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, test.rootName)
			writeWireValidatedFactory(t, path, false)

			request := factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{
				SourcePath: path,
			}
			if test.directory {
				request.SourcePath = directory
				request.ExecutionBaseDir = test.runtimeDir
			}
			result, err := service.LoadValidatedAuthoredFactoryDefinition(t.Context(), request)
			if err != nil {
				t.Fatalf("LoadValidatedAuthoredFactoryDefinition: %v", err)
			}
			assertWireValidatedSourceResult(t, result, path, test.format, directory, test.runtimeDir)
		})
	}
}

func TestValidatedAuthoredFactoryDefinitionLoader_PreservesCurrentSplitBundledAndDetachedFacts(t *testing.T) {
	t.Parallel()

	loader := newCompileLoadLoader(t, platformfilesystem.Local{})
	service := newValidatedAuthoredFactoryDefinitionLoader(t, loader)

	t.Run("current directory", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "factory.json")
		writeWireValidatedFactory(t, path, false)

		result, err := service.LoadValidatedAuthoredFactoryDefinition(
			t.Context(),
			factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{Directory: directory},
		)
		if err != nil {
			t.Fatalf("LoadValidatedAuthoredFactoryDefinition: %v", err)
		}
		assertWireValidatedSourceResult(
			t,
			result,
			path,
			factorydefinitions.AuthoredFactoryFormatJSON,
			directory,
			"",
		)
	})

	t.Run("split layout remains effective and detached", func(t *testing.T) {
		directory := t.TempDir()
		writeWireCompileEquivalenceAuthoredFactory(t, directory)

		first, err := service.LoadValidatedAuthoredFactoryDefinition(
			t.Context(),
			factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{SourcePath: directory},
		)
		if err != nil {
			t.Fatalf("LoadValidatedAuthoredFactoryDefinition(first): %v", err)
		}
		if len(first.Definition.Workers) != 1 || first.Definition.Workers[0].Command != "go" ||
			len(first.Definition.Workstations) != 1 || first.Definition.Workstations[0].PromptTemplate != "Implement {{ .WorkID }}." {
			t.Fatalf("split effective definition = %#v", first.Definition)
		}
		first.Definition.Workers[0].Command = "caller mutation"

		later, err := service.LoadValidatedAuthoredFactoryDefinition(
			t.Context(),
			factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{SourcePath: directory},
		)
		if err != nil {
			t.Fatalf("LoadValidatedAuthoredFactoryDefinition(later): %v", err)
		}
		if later.Definition.Workers[0].Command != "go" {
			t.Fatalf("later effective definition retained caller mutation: %#v", later.Definition)
		}
	})

	t.Run("bundled content remains materialized", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "factory.json")
		writeWireValidatedFactory(t, path, true)
		if err := os.MkdirAll(filepath.Join(directory, "docs"), 0o755); err != nil {
			t.Fatalf("MkdirAll(docs): %v", err)
		}
		if err := os.WriteFile(filepath.Join(directory, "docs", "guide.md"), []byte("old\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(existing bundled doc): %v", err)
		}

		result, err := service.LoadValidatedAuthoredFactoryDefinition(
			t.Context(),
			factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{SourcePath: path},
		)
		if err != nil {
			t.Fatalf("LoadValidatedAuthoredFactoryDefinition: %v", err)
		}
		if len(result.BundledFileReplacements) != 1 ||
			result.BundledFileReplacements[0].TargetPath != "factory/docs/guide.md" {
			t.Fatalf("bundled replacements = %#v", result.BundledFileReplacements)
		}
		result.Definition.ResourceManifest.BundledFiles[0].Content.Inline = "caller mutation"
		result.BundledFileReplacements[0].TargetPath = "caller.md"
		later, err := service.LoadValidatedAuthoredFactoryDefinition(
			t.Context(),
			factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{SourcePath: path},
		)
		if err != nil {
			t.Fatalf("LoadValidatedAuthoredFactoryDefinition(later): %v", err)
		}
		if later.Definition.ResourceManifest.BundledFiles[0].Content.Inline != "new bundled content\n" {
			t.Fatalf("later bundled definition retained caller mutation: %#v", later.Definition.ResourceManifest)
		}
		content, err := os.ReadFile(filepath.Join(directory, "docs", "guide.md"))
		if err != nil {
			t.Fatalf("ReadFile(materialized bundled doc): %v", err)
		}
		if string(content) != "new bundled content\n" {
			t.Fatalf("materialized bundled doc = %q", content)
		}
	})
}

func newValidatedAuthoredFactoryDefinitionLoader(
	t *testing.T,
	loader *factorydefinitionswire.Loader,
) factorydefinitions.ValidatedAuthoredFactoryDefinitionLoader {
	t.Helper()

	service, err := factorydefinitionswire.NewValidatedAuthoredFactoryDefinitionLoader(
		loader.LoadRuntimeSource,
		loader.LoadSourceFromFactoryDir,
		factoryvalidation.New(nil),
	)
	if err != nil {
		t.Fatalf("NewValidatedAuthoredFactoryDefinitionLoader: %v", err)
	}
	return service
}

func writeWireValidatedFactory(t *testing.T, path string, bundled bool) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(factoryfixtures.CrossPathValidAlphaFactoryJSON), &payload); err != nil {
		t.Fatalf("Unmarshal valid Factory fixture: %v", err)
	}
	if bundled {
		payload["supportingFiles"] = map[string]any{
			"bundledFiles": []map[string]any{{
				"type":       "DOC",
				"targetPath": "factory/docs/guide.md",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "new bundled content\n",
				},
			}},
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal valid Factory fixture: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func assertWireValidatedSourceResult(
	t *testing.T,
	result factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult,
	wantPath string,
	wantFormat factorydefinitions.AuthoredFactoryFormat,
	wantFactoryDir string,
	runtimeDirOverride string,
) {
	t.Helper()

	if result.Source.Path != wantPath || result.Source.Format != wantFormat {
		t.Fatalf("selected source = %#v, want path=%q format=%q", result.Source, wantPath, wantFormat)
	}
	if result.FactoryDir != wantFactoryDir {
		t.Fatalf("FactoryDir = %q, want %q", result.FactoryDir, wantFactoryDir)
	}
	wantRuntimeDir := wantFactoryDir
	if runtimeDirOverride != "" {
		wantRuntimeDir = runtimeDirOverride
	}
	if result.RuntimeBaseDir != wantRuntimeDir {
		t.Fatalf("RuntimeBaseDir = %q, want %q", result.RuntimeBaseDir, wantRuntimeDir)
	}
	if result.Definition == nil || result.Definition.Name != "alpha" || result.Validation.HasBlockingTargets() {
		t.Fatalf("validated effective definition = %#v, validation=%#v", result.Definition, result.Validation)
	}
}

func newWireRootWithCompileLoader(t *testing.T) (factorydefinitions.Service, *factorydefinitionswire.Loader) {
	t.Helper()

	fileSystem := platformfilesystem.Local{}
	loader := newCompileLoadLoader(t, fileSystem)
	ports := validConstructionPorts(t)
	ports.loader = loader

	service, err := factorydefinitionswire.NewService(
		ports.sessionHost,
		ports.activationGateway,
		ports.validator,
		ports.persistence,
		ports.loader,
		ports.applySupportedFiles,
		ports.applyStarterWork,
		ports.namedPaths,
		ports.namedFactoryCatalogFileSystem,
		ports.clock,
		ports.versionFileSystem,
		ports.listEffective,
		ports.packagedCatalog,
		ports.packagedInstaller,
		ports.requiredToolChecker,
		ports.orchestratorValidator,
		ports.portableFileSystem,
		ports.directoryReplacementStore,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service, loader
}

func newCompileLoadLoader(
	t *testing.T,
	fileSystem platformfilesystem.Local,
) *factorydefinitionswire.Loader {
	t.Helper()

	applySupportedFiles, err := internalportableconfig.NewPortableBundledFilesApplier(fileSystem)
	if err != nil {
		t.Fatalf("construct bundled-files applier: %v", err)
	}
	applyStarterWork, err := internalportableconfig.NewFactoryStarterWorkApplier(fileSystem)
	if err != nil {
		t.Fatalf("construct starter-Work applier: %v", err)
	}
	materializeFiles := func(
		targetDir string,
		config *factorydefinitions.FactoryConfig,
	) ([]factorydefinitions.PortableBundledFileReplacement, error) {
		return internalportableconfig.MaterializeFiles(fileSystem, targetDir, config)
	}
	namedPaths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("construct named-path resolver: %v", err)
	}
	sourceResolver, err := internalportableconfig.NewSupportedSourceResolver(fileSystem)
	if err != nil {
		t.Fatalf("construct portable source resolver: %v", err)
	}
	return factorydefinitionswire.NewLoader(
		applySupportedFiles,
		applyStarterWork,
		materializeFiles,
		fileSystem,
		namedPaths,
		fileSystem,
		sourceResolver,
		fileSystem,
		stubRequiredToolChecker{},
	)
}

func exerciseWireRootCompileSuccess(
	t *testing.T,
	root factorydefinitions.Service,
	factoryDir string,
	canonical []byte,
) {
	t.Helper()
	ctx := context.Background()

	fromDirectory, err := root.CompileEffectiveFactorySource(
		ctx,
		factorydefinitions.CompileEffectiveFactorySourceRequest{FactoryDir: factoryDir},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource(directory): %v", err)
	}

	fromCanonical, err := root.CompileEffectiveFactorySource(
		ctx,
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical:  canonical,
			FactoryDir: factoryDir,
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource(canonical): %v", err)
	}

	assertWireEquivalentEffectiveOutcomes(t, fromDirectory, fromCanonical)

	facts := wireEffectiveWorkerAndWorkstationFacts(t, fromDirectory.Effective.ContentIdentity)
	if facts.workerName != "executor" || facts.workerCommand != "go" {
		t.Fatalf("worker facts = %#v, want executor with command go", facts)
	}
	if facts.workstationName != "execute-story" || facts.workstationWorker != "executor" {
		t.Fatalf("workstation facts = %#v, want execute-story bound to executor", facts)
	}
	if fromDirectory.Effective.FactoryDir != factoryDir ||
		fromDirectory.Effective.RuntimeBaseDir != factoryDir {
		t.Fatalf(
			"directory effective = %#v, want factory directory identity %q",
			fromDirectory.Effective,
			factoryDir,
		)
	}
}

func exerciseWireRootCompileTypedFailures(t *testing.T, root factorydefinitions.Service) {
	t.Helper()
	ctx := context.Background()

	_, invalidErr := root.CompileEffectiveFactorySource(
		ctx,
		factorydefinitions.CompileEffectiveFactorySourceRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatalf(
			"CompileEffectiveFactorySource invalid-source error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidAuthoredFactorySource,
		)
	}

	_, unresolvedErr := root.CompileEffectiveFactorySource(
		ctx,
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical: []byte(`{"worker":"$unresolved"}`),
		},
	)
	if !errors.Is(unresolvedErr, factorydefinitions.ErrUnresolvedDefinitionReference) {
		t.Fatalf(
			"CompileEffectiveFactorySource unresolved error = %v, want %v",
			unresolvedErr,
			factorydefinitions.ErrUnresolvedDefinitionReference,
		)
	}
	if errors.Is(unresolvedErr, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatal("unresolved definition reference must not also match ErrInvalidAuthoredFactorySource")
	}
}

type wireEffectiveMergedFacts struct {
	workerName        string
	workerCommand     string
	workstationName   string
	workstationType   string
	workstationWorker string
}

func wireEffectiveWorkerAndWorkstationFacts(t *testing.T, contentIdentity string) wireEffectiveMergedFacts {
	t.Helper()

	var cfg factorydefinitions.FactoryConfig
	if err := json.Unmarshal([]byte(contentIdentity), &cfg); err != nil {
		t.Fatalf("decode ContentIdentity: %v", err)
	}
	if len(cfg.Workers) != 1 || len(cfg.Workstations) != 1 {
		t.Fatalf("effective config = %#v, want one worker and one workstation", cfg)
	}
	return wireEffectiveMergedFacts{
		workerName:        cfg.Workers[0].Name,
		workerCommand:     cfg.Workers[0].Command,
		workstationName:   cfg.Workstations[0].Name,
		workstationType:   cfg.Workstations[0].Type,
		workstationWorker: cfg.Workstations[0].WorkerTypeName,
	}
}

func assertWireEquivalentEffectiveOutcomes(
	t *testing.T,
	first factorydefinitions.CompileEffectiveFactorySourceResult,
	second factorydefinitions.CompileEffectiveFactorySourceResult,
) {
	t.Helper()

	if first.Effective.ContentIdentity == "" || second.Effective.ContentIdentity == "" {
		t.Fatal("ContentIdentity must not be empty")
	}
	if first.Effective.ContentIdentity != second.Effective.ContentIdentity {
		t.Fatalf(
			"equivalent inputs produced different ContentIdentity: %q vs %q",
			first.Effective.ContentIdentity,
			second.Effective.ContentIdentity,
		)
	}
	if first.Effective.FactoryDir != second.Effective.FactoryDir {
		t.Fatalf(
			"equivalent inputs produced different FactoryDir: %q vs %q",
			first.Effective.FactoryDir,
			second.Effective.FactoryDir,
		)
	}
	if first.Effective.RuntimeBaseDir != second.Effective.RuntimeBaseDir {
		t.Fatalf(
			"equivalent inputs produced different RuntimeBaseDir: %q vs %q",
			first.Effective.RuntimeBaseDir,
			second.Effective.RuntimeBaseDir,
		)
	}
}

func writeWireCompileEquivalenceAuthoredFactory(t *testing.T, factoryDir string) {
	t.Helper()

	factoryJSON := `{
  "name": "factory",
  "workTypes": [{
    "name": "story",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"}
    ]
  }],
  "resources": [],
  "workers": [{"name": "executor"}],
  "workstations": [{
    "name": "execute-story",
    "worker": "executor",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}]
  }]
}`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(factoryJSON), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	workerDir := filepath.Join(factoryDir, "workers", "executor")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}
	workerBody := `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
---
Run tests.
`
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(workerBody), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
	workstationDir := filepath.Join(factoryDir, "workstations", "execute-story")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("mkdir workstation dir: %v", err)
	}
	workstationBody := `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
---
Implement {{ .WorkID }}.
`
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(workstationBody), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workstationDir, "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}
}

func writeWireCompileEquivalenceRuntimeMergeFactory(t *testing.T, factoryDir string) {
	t.Helper()

	factoryJSON := `{
  "name": "factory",
  "workTypes": [{
    "name": "story",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"}
    ]
  }],
  "resources": [],
  "workers": [{"name": "executor"}],
  "workstations": [{
    "name": "execute-story",
    "worker": "executor",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}],
    "stopWords": ["CANONICAL"]
  }]
}`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(factoryJSON), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	workerDir := filepath.Join(factoryDir, "workers", "executor")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}
	workerBody := `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
---
Run tests.
`
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(workerBody), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
	workstationDir := filepath.Join(factoryDir, "workstations", "execute-story")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("mkdir workstation dir: %v", err)
	}
	workstationBody := `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
stopWords: ["RUNTIME"]
---
Implement {{ .WorkID }}.
`
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(workstationBody), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workstationDir, "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}
}
