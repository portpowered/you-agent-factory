package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReplaceFactorySplitLayout_PrunesRemovedWorkerAndOverwritesAgents(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitLayoutRefreshPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	oldWorkerDir := filepath.Join(targetDir, interfaces.WorkersDir, "old-name")
	if err := os.MkdirAll(oldWorkerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(old worker): %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldWorkerDir, interfaces.FactoryAgentsFileName), []byte("stale worker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(old worker AGENTS.md): %v", err)
	}

	updated := splitLayoutRefreshPayload(t, "alpha-v2")
	result, err := ReplaceFactorySplitLayout(targetDir, updated)
	if err != nil {
		t.Fatalf("ReplaceFactorySplitLayout: %v", err)
	}
	if result != nil && result.DiscardBackup != nil {
		result.DiscardBackup()
	}

	if _, err := os.Stat(oldWorkerDir); !os.IsNotExist(err) {
		t.Fatalf("expected removed worker directory %s, stat err=%v", oldWorkerDir, err)
	}

	agentsPath := filepath.Join(targetDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName)
	agents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(executor AGENTS.md): %v", err)
	}
	if !strings.Contains(string(agents), "You are the executor.") {
		t.Fatalf("executor AGENTS.md = %q, want submitted worker body", agents)
	}

	workstationPath := filepath.Join(targetDir, interfaces.WorkstationsDir, "execute-alpha-v2", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected workstation AGENTS.md at %s: %v", workstationPath, err)
	}
}

func TestWriteNamedFactoryLayout_OverwritesExistingAgentsOnSave(t *testing.T) {
	targetDir := t.TempDir()
	payload := splitLayoutRefreshPayload(t, "alpha")
	factoryCfg, canonical, err := normalizeNamedFactoryPayload("alpha", payload)
	if err != nil {
		t.Fatalf("normalizeNamedFactoryPayload: %v", err)
	}
	sourcePath := filepath.Join(targetDir, interfaces.FactoryConfigFile)

	if _, err := writeNamedFactoryLayout(targetDir, factoryCfg, canonical, sourcePath); err != nil {
		t.Fatalf("writeNamedFactoryLayout initial: %v", err)
	}

	executorAgents := filepath.Join(targetDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName)
	if err := os.WriteFile(executorAgents, []byte("stale executor body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale executor AGENTS.md): %v", err)
	}
	oldWorkerDir := filepath.Join(targetDir, interfaces.WorkersDir, "old-name")
	if err := os.MkdirAll(oldWorkerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(old worker): %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldWorkerDir, interfaces.FactoryAgentsFileName), []byte("stale worker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(old worker AGENTS.md): %v", err)
	}

	if _, err := writeNamedFactoryLayout(targetDir, factoryCfg, canonical, sourcePath); err != nil {
		t.Fatalf("writeNamedFactoryLayout refresh: %v", err)
	}

	if _, err := os.Stat(oldWorkerDir); !os.IsNotExist(err) {
		t.Fatalf("expected pruned worker directory %s, stat err=%v", oldWorkerDir, err)
	}

	got, err := os.ReadFile(executorAgents)
	if err != nil {
		t.Fatalf("ReadFile(executor AGENTS.md): %v", err)
	}
	if strings.Contains(string(got), "stale executor body") {
		t.Fatalf("executor AGENTS.md = %q, want overwrite from submitted config", got)
	}
	if !strings.Contains(string(got), "You are the executor.") {
		t.Fatalf("executor AGENTS.md = %q, want submitted worker body", got)
	}
}

func splitLayoutRefreshPayload(t *testing.T, project string) []byte {
	t.Helper()
	return []byte(`{
		"name": "` + project + `",
		"id": "` + project + `",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-` + project + `","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
}
