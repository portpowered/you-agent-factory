package factory_transformation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func assertFunctionalSplitLayoutAtRoot(t *testing.T, rootDir, project string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the planner.") {
		t.Fatalf("factory.json should omit inlined planner body after split save, got %s", factoryJSON)
	}

	agentsPath := filepath.Join(rootDir, interfaces.WorkersDir, "planner", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected planner AGENTS.md at %s: %v", agentsPath, err)
	}

	workstationPath := filepath.Join(rootDir, interfaces.WorkstationsDir, "plan-task", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected plan-task AGENTS.md at %s: %v", workstationPath, err)
	}

	loaded, err := config.LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.FactoryConfig().Project != project {
		t.Fatalf("project = %q, want %q", loaded.FactoryConfig().Project, project)
	}
}
