package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReplaceFactorySplitLayout_WritesSplitLayoutAndRestores(t *testing.T) {
	targetDir := t.TempDir()
	initial := rollbackNamedFactoryPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	updated := rollbackNamedFactoryPayload(t, "alpha-v2")
	restore, err := config.ReplaceFactorySplitLayout(targetDir, updated)
	if err != nil {
		t.Fatalf("ReplaceFactorySplitLayout: %v", err)
	}

	assertSplitLayoutPersisted(t, targetDir, "alpha-v2")

	restore()
	assertMonolithicFactoryJSON(t, targetDir, initial)
}

func assertSplitLayoutPersisted(t *testing.T, factoryDir, project string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the executor.") {
		t.Fatalf("factory.json should omit inlined worker body, got %s", factoryJSON)
	}

	agentsPath := filepath.Join(factoryDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName)
	agents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(worker AGENTS.md): %v", err)
	}
	if !strings.Contains(string(agents), "You are the executor.") {
		t.Fatalf("worker AGENTS.md = %q, want executor body", agents)
	}

	workstationPath := filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-"+project, interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected workstation AGENTS.md at %s: %v", workstationPath, err)
	}

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.FactoryConfig().Project != project {
		t.Fatalf("project = %q, want %q", loaded.FactoryConfig().Project, project)
	}
}

func assertMonolithicFactoryJSON(t *testing.T, factoryDir string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("factory.json = %q, want %q", got, want)
	}
}

func rollbackNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return []byte(`{
		"name": "` + project + `",
		"id": "` + project + `",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-` + project + `","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
}
