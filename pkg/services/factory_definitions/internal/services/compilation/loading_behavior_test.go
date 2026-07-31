package compilation_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/canonical"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	factorydefinitiontestcomposition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

func TestCompilationOwner_LoadAndCompilePreserveEffectiveSourceFacts(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	writeAuthoredFactory(t, factoryDir)

	composition := testComposition()
	loader := composition.Loader()

	compilation, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loader.LoadSourceFromCanonicalJSON,
		LoadFromFactoryDir: loader.LoadSourceFromFactoryDir,
		EncodeFactory:      compilationcanonical.EncodeFactoryPort(),
	})
	if err != nil {
		t.Fatalf("compilationwire.NewService: %v", err)
	}

	loaded, err := loader.LoadSourceFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadSourceFromFactoryDir: %v", err)
	}
	worker, ok := loaded.Worker("executor")
	if !ok || worker.Command != "go" {
		t.Fatalf("loaded worker = %#v, want executor with command go", worker)
	}
	workstation, ok := loaded.Workstation("execute-story")
	if !ok || workstation.Type != factoryroot.WorkstationTypeModel {
		t.Fatalf("loaded workstation = %#v, want model workstation facts", workstation)
	}

	compiled, err := compilation.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{FactoryDir: factoryDir},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource: %v", err)
	}
	if compiled.Effective.FactoryDir != factoryDir {
		t.Fatalf("FactoryDir = %q, want %q", compiled.Effective.FactoryDir, factoryDir)
	}
	if compiled.Effective.RuntimeBaseDir != factoryDir {
		t.Fatalf("RuntimeBaseDir = %q, want %q", compiled.Effective.RuntimeBaseDir, factoryDir)
	}
	if compiled.Effective.ContentIdentity == "" {
		t.Fatal("ContentIdentity is empty")
	}
}

func testComposition() factorydefinitiontestcomposition.Composition {
	mapper := factorymapping.NewFactoryConfigMapper()
	fileSystem := platformfilesystem.Local{}
	return factorydefinitiontestcomposition.New(
		factorydefinitiontestcomposition.Representation{
			DecodeFactory:     factorymapping.ExpandFactoryConfigForRuntimeLoad,
			DecodeAuthored:    mapper.Expand,
			EncodeFactory:     factorymapping.MarshalCanonicalFactoryConfig,
			NormalizeAuthored: authoredmapping.AuthoredFactoryConfigForExpandedLayout,
			ParseWorker:       authoredmapping.ParseWorkerConfig,
			ParseWorkstation:  authoredmapping.ParseWorkstationConfig,
			ParseBody:         authoredmapping.ParseAgentsBody,
			RenderWorker:      authoredmapping.RenderWorkerAgentsMarkdown,
			RenderWorkstation: authoredmapping.RenderWorkstationAgentsMarkdown,
			RenderBody:        authoredmapping.RenderAgentsBody,
			SafeLayoutSegment: authoredmapping.SafeFactoryLayoutSegment,
			SafePromptPath:    authoredmapping.SafePromptFilePath,
		},
		fileSystem,
		directoryreplace.Local{},
		factorydefinitiontestcomposition.Effects{
			Loading:             fileSystem,
			AuthoredReader:      fileSystem,
			AuthoredWriter:      fileSystem,
			Persistence:         fileSystem,
			NamedPaths:          fileSystem,
			NamedFactoryCatalog: fileSystem,
			InboxSentinels:      inboxgitkeep.NewLocal(fileSystem),
		},
	)
}

func writeAuthoredFactory(t *testing.T, factoryDir string) {
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
