package goal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
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

func TestMaterializePackagedGoalFactory_DerivesRolePromptsFromCanonicalSource(t *testing.T) {
	factoryDir := materializePackagedGoalFactory(t, t.TempDir())

	for _, source := range PackagedGoalRolePromptSources {
		authoredPrompt := authoredGoalPrompt(t, source)

		materializedPrompt, err := loadPackagedGoalRolePrompt(factoryDir, source)
		if err != nil {
			t.Fatalf("role %q materialized prompt load: %v", source.Role, err)
		}
		if materializedPrompt != authoredPrompt {
			t.Fatalf("role %q materialized prompt does not match canonical authored source", source.Role)
		}
	}

	assertPersistedPackagedGoalFactoryJSONOmitsInlinePromptBodies(t, factoryDir)
}

func authoredGoalPrompt(t *testing.T, source PackagedGoalRolePromptSource) string {
	t.Helper()
	path := filepath.Join("..", "definitions", "goal", filepath.FromSlash(source.PromptFile))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read authored prompt for role %q: %v", source.Role, err)
	}
	return string(content)
}

func TestMaterializePackagedGoalFactory_DeterministicFreshMaterialization(t *testing.T) {
	firstDir := materializePackagedGoalFactory(t, t.TempDir())
	secondDir := materializePackagedGoalFactory(t, t.TempDir())

	for _, source := range PackagedGoalRolePromptSources {
		firstPrompt, err := os.ReadFile(packagedGoalMaterializedPromptPath(firstDir, source))
		if err != nil {
			t.Fatalf("read first materialized prompt for role %q: %v", source.Role, err)
		}
		secondPrompt, err := os.ReadFile(packagedGoalMaterializedPromptPath(secondDir, source))
		if err != nil {
			t.Fatalf("read second materialized prompt for role %q: %v", source.Role, err)
		}
		if string(firstPrompt) != string(secondPrompt) {
			t.Fatalf("role %q prompt bytes differ across two fresh materializations", source.Role)
		}
	}
}

func materializePackagedGoalFactory(t *testing.T, globalRoot string) string {
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
		filepath.Join(factoryDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName),
		filepath.Join(factoryDir, interfaces.WorkstationsDir, PackagedExecuteWorkstationName, interfaces.FactoryAgentsFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected materialized path %s: %v", path, err)
		}
	}
	for _, source := range PackagedGoalRolePromptSources {
		path := packagedGoalMaterializedPromptPath(factoryDir, source)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected materialized role prompt path %s: %v", path, err)
		}
	}
}

func assertPersistedPackagedGoalFactoryJSONOmitsInlinePromptBodies(t *testing.T, factoryDir string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(factoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}

	workers, ok := payload["workers"].([]any)
	if !ok {
		t.Fatal("persisted factory.json workers must be an array")
	}
	for _, entry := range workers {
		worker, ok := entry.(map[string]any)
		if !ok {
			t.Fatal("persisted worker entry must be an object")
		}
		if body, hasBody := worker["body"]; hasBody && strings.TrimSpace(body.(string)) != "" {
			t.Fatalf("persisted worker %q must not inline prompt body in factory.json", worker["name"])
		}
	}

	workstations, ok := payload["workstations"].([]any)
	if !ok {
		t.Fatal("persisted factory.json workstations must be an array")
	}
	for _, entry := range workstations {
		workstation, ok := entry.(map[string]any)
		if !ok {
			t.Fatal("persisted workstation entry must be an object")
		}
		if body, hasBody := workstation["body"]; hasBody && strings.TrimSpace(body.(string)) != "" {
			t.Fatalf("persisted workstation %q must not inline prompt body in factory.json", workstation["name"])
		}
		if promptTemplate, hasPromptTemplate := workstation["promptTemplate"]; hasPromptTemplate && strings.TrimSpace(promptTemplate.(string)) != "" {
			t.Fatalf("persisted workstation %q must not inline promptTemplate in factory.json", workstation["name"])
		}
	}
}
