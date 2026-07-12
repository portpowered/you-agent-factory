package goal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestMaterializePackagedGoalFactory_WritesEditableSplitLayout(t *testing.T) {
	factoryDir := materializePackagedGoalFactory(t, t.TempDir())
	assertMaterializedSplitLayout(t, factoryDir)

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	if loaded.FactoryConfig().Project != PackagedFactoryProject {
		t.Fatalf("project = %q, want %s", loaded.FactoryConfig().Project, PackagedFactoryProject)
	}
	if _, ok := loaded.Worker("goal-executor"); !ok {
		t.Fatal("expected materialized goal-executor worker")
	}
	workstation, ok := loaded.Workstation(PackagedExecuteWorkstationName)
	if !ok {
		t.Fatal("expected materialized execute-goal workstation")
	}
	if strings.TrimSpace(workstation.PromptTemplate) == "" {
		t.Fatal("expected execute-goal workstation prompt loaded from split-layout prompts/ directory")
	}
}

func materializePackagedGoalFactory(t *testing.T, globalRoot string) string {
	t.Helper()
	factoryDir, err := factoryconfig.PersistNamedFactory(globalRoot, PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
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
		filepath.Join(factoryDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName),
		filepath.Join(factoryDir, interfaces.WorkstationsDir, PackagedExecuteWorkstationName, interfaces.FactoryAgentsFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected materialized path %s: %v", path, err)
		}
	}
}
