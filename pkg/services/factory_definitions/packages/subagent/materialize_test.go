package subagent

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

func TestMaterializePackagedSubagentFactory_WritesEditableSplitLayout(t *testing.T) {
	factoryDir := materializePackagedSubagentFactory(t, t.TempDir())
	assertMaterializedSplitLayout(t, factoryDir)
	wantWorkerPrompt := authoredSubagentPrompt(t, "prompts/worker.md")
	wantWorkstationPrompt := authoredSubagentPrompt(t, "prompts/run-subagent.md")
	assertMaterializedPrompt(t,
		filepath.Join(factoryDir, interfaces.WorkersDir, PackagedWorkerName, interfaces.FactoryAgentsFileName),
		wantWorkerPrompt,
	)
	assertMaterializedPrompt(t,
		filepath.Join(factoryDir, interfaces.WorkstationsDir, PackagedRunWorkstationName, "prompts", "run-subagent.md"),
		wantWorkstationPrompt,
	)

	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	if loaded.FactoryConfig().Name != PackagedFactoryName {
		t.Fatalf("factory name = %q, want %s", loaded.FactoryConfig().Name, PackagedFactoryName)
	}
	if loaded.FactoryConfig().Project != PackagedFactoryProject {
		t.Fatalf("project = %q, want %s", loaded.FactoryConfig().Project, PackagedFactoryProject)
	}
	worker, ok := loaded.Worker(PackagedWorkerName)
	if !ok {
		t.Fatal("expected materialized subagent-worker")
	}
	if worker.Body != wantWorkerPrompt {
		t.Fatal("reloaded worker body does not exactly match authored prompt")
	}
	workstation, ok := loaded.Workstation(PackagedRunWorkstationName)
	if !ok {
		t.Fatal("expected materialized run-subagent workstation")
	}
	if workstation.Body != wantWorkstationPrompt {
		t.Fatal("reloaded workstation body does not exactly match authored prompt")
	}
	if workstation.PromptTemplate != wantWorkstationPrompt {
		t.Fatal("reloaded workstation prompt template does not exactly match authored prompt")
	}
}

func authoredSubagentPrompt(t *testing.T, promptFile string) string {
	t.Helper()
	path := "factories/subagent/" + promptFile
	content, err := fs.ReadFile(packagedfactories.Source(), path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(content)
}

func assertMaterializedPrompt(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("materialized prompt %s does not exactly match authored content", path)
	}
}

func TestResolveNamedFactoryAcrossRoots_ResolvesInstalledSubagentFromGlobalRoot(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	installedDir, err := factorydefinitioncomposition.PersistNamedFactory(globalRoot, PackagedFactoryName, BuiltInFactoryJSON, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	resolution, err := factorydefinitioncomposition.NamedFactoryCatalog().ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(builtin subagent): %v", err)
	}
	if resolution.Name != PackagedFactoryName {
		t.Fatalf("resolution name = %q, want %s", resolution.Name, PackagedFactoryName)
	}
	if resolution.Source != interfaces.NamedFactoryResolutionSourceGlobal {
		t.Fatalf("resolution source = %q, want global", resolution.Source)
	}

	wantDir := filepath.Join(globalRoot, "@you", "subagent")
	if wantDir != installedDir {
		t.Fatalf("installed dir = %q, want %q", installedDir, wantDir)
	}
	if resolution.FactoryDir != wantDir {
		t.Fatalf("factory dir = %q, want hierarchical layout %q", resolution.FactoryDir, wantDir)
	}
	assertMaterializedSplitLayout(t, wantDir)

	loaded, err := factorydefinitioncomposition.LoadDirectory(
		resolution.FactoryDir,
		nil,
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(materialized builtin subagent): %v", err)
	}
	if loaded.FactoryConfig().Project != PackagedFactoryProject {
		t.Fatalf("materialized builtin project = %q, want %s", loaded.FactoryConfig().Project, PackagedFactoryProject)
	}
}

func materializePackagedSubagentFactory(t *testing.T, globalRoot string) string {
	t.Helper()
	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(globalRoot, PackagedFactoryName, BuiltInFactoryJSON, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	return factoryDir
}

func assertMaterializedSplitLayout(t *testing.T, factoryDir string) {
	t.Helper()
	for _, dirName := range []string{interfaces.WorkersDir, interfaces.WorkstationsDir} {
		info, err := os.Stat(filepath.Join(factoryDir, dirName))
		if err != nil {
			t.Fatalf("stat %s: %v", dirName, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dirName)
		}
	}
	for _, path := range []string{
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		filepath.Join(factoryDir, interfaces.WorkersDir, PackagedWorkerName, interfaces.FactoryAgentsFileName),
		filepath.Join(factoryDir, interfaces.WorkstationsDir, PackagedRunWorkstationName, interfaces.FactoryAgentsFileName),
		filepath.Join(factoryDir, interfaces.WorkstationsDir, PackagedRunWorkstationName, "prompts", "run-subagent.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected materialized path %s: %v", path, err)
		}
	}
}
