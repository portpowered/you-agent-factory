package splitreplacetests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
)

func TestReplaceFactoryLayoutAtDir_WritesSplitLayoutAndRestores(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	updated := splitReplaceTestPayload(t, "alpha-v2")
	result, err := factorydefinitioncomposition.ReplaceFactoryLayout(
		targetDir,
		updated,
		factoryvalidation.New(nil),
	)
	if err != nil {
		t.Fatalf("ReplaceFactoryLayout: %v", err)
	}
	if result == nil || result.Restore == nil {
		t.Fatal("expected restore callback")
	}

	assertSplitLayoutPersisted(t, targetDir, "alpha-v2")

	result.Restore()
	assertMonolithicFactoryJSON(t, targetDir, initial)
}

func TestReplaceFactorySplitLayout_WritesSplitLayoutAndRestores(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	updated := splitReplaceTestPayload(t, "alpha-v2")
	result, err := factorydefinitioncomposition.ReplaceFactoryLayout(targetDir, updated, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("ReplaceFactorySplitLayout: %v", err)
	}

	assertSplitLayoutPersisted(t, targetDir, "alpha-v2")

	result.Restore()
	assertMonolithicFactoryJSON(t, targetDir, initial)
}

func TestReplaceFactorySplitLayout_PrunesRemovedWorkerAndOverwritesAgents(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	oldWorkerDir := filepath.Join(targetDir, factorydefinitions.WorkersDir, "old-name")
	if err := os.MkdirAll(oldWorkerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(old worker): %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldWorkerDir, factorydefinitions.FactoryAgentsFileName), []byte("stale worker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(old worker AGENTS.md): %v", err)
	}

	updated := splitReplaceTestPayload(t, "alpha-v2")
	result, err := factorydefinitioncomposition.ReplaceFactoryLayout(targetDir, updated, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("ReplaceFactorySplitLayout: %v", err)
	}
	if result != nil && result.DiscardBackup != nil {
		result.DiscardBackup()
	}

	if _, err := os.Stat(oldWorkerDir); !os.IsNotExist(err) {
		t.Fatalf("expected removed worker directory %s, stat err=%v", oldWorkerDir, err)
	}

	agentsPath := filepath.Join(targetDir, factorydefinitions.WorkersDir, "executor", factorydefinitions.FactoryAgentsFileName)
	agents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(executor AGENTS.md): %v", err)
	}
	if !strings.Contains(string(agents), "You are the executor.") {
		t.Fatalf("executor AGENTS.md = %q, want submitted worker body", agents)
	}

	workstationPath := filepath.Join(targetDir, factorydefinitions.WorkstationsDir, "execute-alpha-v2", factorydefinitions.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected workstation AGENTS.md at %s: %v", workstationPath, err)
	}
}

func TestReplaceFactoryLayoutAtDir_PrunesRemovedWorkerDirectory(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	removedWorkerDir := filepath.Join(targetDir, factorydefinitions.WorkersDir, "removed")
	if err := os.MkdirAll(removedWorkerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workers/removed): %v", err)
	}
	if err := os.WriteFile(filepath.Join(removedWorkerDir, factorydefinitions.FactoryAgentsFileName), []byte("orphan worker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(workers/removed/AGENTS.md): %v", err)
	}

	result, err := factorydefinitioncomposition.ReplaceFactoryLayout(
		targetDir,
		splitReplaceTestPayload(t, "alpha-v2"),
		factoryvalidation.New(nil),
	)
	if err != nil {
		t.Fatalf("ReplaceFactoryLayout: %v", err)
	}
	if result == nil || result.Restore == nil {
		t.Fatal("expected restore callback")
	}

	if _, err := os.Stat(removedWorkerDir); !os.IsNotExist(err) {
		t.Fatalf("expected workers/removed pruned after persist-from-save, stat err=%v", err)
	}
}

func TestReplaceFactoryLayoutAtDir_PrunesRemovedWorkstationDirectory(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	removedWorkstationDir := filepath.Join(targetDir, factorydefinitions.WorkstationsDir, "removed")
	if err := os.MkdirAll(removedWorkstationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workstations/removed): %v", err)
	}
	if err := os.WriteFile(filepath.Join(removedWorkstationDir, factorydefinitions.FactoryAgentsFileName), []byte("orphan workstation\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(workstations/removed/AGENTS.md): %v", err)
	}

	result, err := factorydefinitioncomposition.ReplaceFactoryLayout(
		targetDir,
		splitReplaceTestPayload(t, "alpha-v2"),
		factoryvalidation.New(nil),
	)
	if err != nil {
		t.Fatalf("ReplaceFactoryLayout: %v", err)
	}
	if result == nil || result.Restore == nil {
		t.Fatal("expected restore callback")
	}

	if _, err := os.Stat(removedWorkstationDir); !os.IsNotExist(err) {
		t.Fatalf("expected workstations/removed pruned after persist-from-save, stat err=%v", err)
	}
}

func TestReplaceFactorySplitLayout_OverwritesExistingAgentsOnSave(t *testing.T) {
	targetDir := t.TempDir()
	payload := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	result, err := factorydefinitioncomposition.ReplaceFactoryLayout(targetDir, payload, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("ReplaceFactorySplitLayout initial: %v", err)
	}
	if result != nil && result.DiscardBackup != nil {
		result.DiscardBackup()
	}

	executorAgents := filepath.Join(targetDir, factorydefinitions.WorkersDir, "executor", factorydefinitions.FactoryAgentsFileName)
	if err := os.WriteFile(executorAgents, []byte("stale executor body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale executor AGENTS.md): %v", err)
	}
	oldWorkerDir := filepath.Join(targetDir, factorydefinitions.WorkersDir, "old-name")
	if err := os.MkdirAll(oldWorkerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(old worker): %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldWorkerDir, factorydefinitions.FactoryAgentsFileName), []byte("stale worker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(old worker AGENTS.md): %v", err)
	}

	result, err = factorydefinitioncomposition.ReplaceFactoryLayout(targetDir, payload, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("ReplaceFactorySplitLayout refresh: %v", err)
	}
	if result != nil && result.DiscardBackup != nil {
		result.DiscardBackup()
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

func assertSplitLayoutPersisted(t *testing.T, factoryDir, project string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the executor.") {
		t.Fatalf("factory.json should omit inlined worker body, got %s", factoryJSON)
	}

	agentsPath := filepath.Join(factoryDir, factorydefinitions.WorkersDir, "executor", factorydefinitions.FactoryAgentsFileName)
	agents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(worker AGENTS.md): %v", err)
	}
	if !strings.Contains(string(agents), "You are the executor.") {
		t.Fatalf("worker AGENTS.md = %q, want executor body", agents)
	}

	workstationPath := filepath.Join(factoryDir, factorydefinitions.WorkstationsDir, "execute-"+project, factorydefinitions.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected workstation AGENTS.md at %s: %v", workstationPath, err)
	}

	loaded, err := factorydefinitioncomposition.LoadedFactoryLoader(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.FactoryConfig().Project != project {
		t.Fatalf("project = %q, want %q", loaded.FactoryConfig().Project, project)
	}
}

func assertMonolithicFactoryJSON(t *testing.T, factoryDir string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("factory.json = %q, want %q", got, want)
	}
}

func splitReplaceTestPayload(t *testing.T, project string) []byte {
	t.Helper()
	return []byte(`{
		"name": "` + project + `",
		"id": "` + project + `",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-` + project + `","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
}
