package subagent

import (
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestMaterializePackagedSubagentFactory_WritesEditableSplitLayout(t *testing.T) {
	factoryDir := materializePackagedSubagentFactory(t, t.TempDir())
	assertMaterializedSplitLayout(t, factoryDir)

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
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
	if worker.Body == "" {
		t.Fatal("expected worker body loaded from split-layout workers/ directory")
	}
	workstation, ok := loaded.Workstation(PackagedRunWorkstationName)
	if !ok {
		t.Fatal("expected materialized run-subagent workstation")
	}
	if workstation.Body == "" {
		t.Fatal("expected workstation body loaded from split-layout workstations/ directory")
	}
}

func TestResolveNamedFactoryAcrossRoots_ResolvesInstalledSubagentFromGlobalRoot(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	installedDir, err := factoryconfig.PersistNamedFactory(globalRoot, PackagedFactoryName, BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(builtin subagent): %v", err)
	}
	if resolution.Name != PackagedFactoryName {
		t.Fatalf("resolution name = %q, want %s", resolution.Name, PackagedFactoryName)
	}
	if resolution.Source != factoryconfig.NamedFactoryResolutionSourceGlobal {
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

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(resolution.FactoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(materialized builtin subagent): %v", err)
	}
	if loaded.FactoryConfig().Project != PackagedFactoryProject {
		t.Fatalf("materialized builtin project = %q, want %s", loaded.FactoryConfig().Project, PackagedFactoryProject)
	}
}

func materializePackagedSubagentFactory(t *testing.T, globalRoot string) string {
	t.Helper()
	factoryDir, err := factoryconfig.PersistNamedFactory(globalRoot, PackagedFactoryName, BuiltInFactoryJSON)
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
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected materialized path %s: %v", path, err)
		}
	}
}
