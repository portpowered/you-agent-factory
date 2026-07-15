package splitreplacetests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestReplaceFactoryLayoutAtDir_WritesSplitLayoutAndRestores(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	updated := splitReplaceTestPayload(t, "alpha-v2")
	restore, err := factoryconfig.ReplaceFactoryLayoutAtDir(
		targetDir,
		updated,
		factoryconfig.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
	if err != nil {
		t.Fatalf("ReplaceFactoryLayoutAtDir: %v", err)
	}
	if restore == nil {
		t.Fatal("expected restore callback")
	}

	assertSplitLayoutPersisted(t, targetDir, "alpha-v2")

	restore()
	assertMonolithicFactoryJSON(t, targetDir, initial)
}

func TestReplaceFactoryLayoutAtDir_ValidationFailureLeavesFactoryUnchanged(t *testing.T) {
	targetDir := t.TempDir()
	initial := []byte(`{
		"name": "alpha",
		"id": "alpha",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-alpha","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	_, err := factoryconfig.ReplaceFactoryLayoutAtDirWithAfterStageHook(
		targetDir,
		splitReplaceTestPayload(t, "broken"),
		factoryconfig.DefaultFactoryLayoutReplaceOptions(targetDir),
		func(stagingDir string) error {
			path := filepath.Join(stagingDir, interfaces.WorkstationsDir, "execute-broken", interfaces.FactoryAgentsFileName)
			return os.WriteFile(path, []byte("---\ntype: [\n"), 0o644)
		},
	)
	if err == nil {
		t.Fatal("expected validation failure before commit")
	}
	if got := err.Error(); !strings.Contains(got, `validate factory`) || !strings.Contains(got, "AGENTS.md missing closing frontmatter delimiter") {
		t.Fatalf("expected load-time validation error, got %v", err)
	}

	got, readErr := os.ReadFile(filepath.Join(targetDir, interfaces.FactoryConfigFile))
	if readErr != nil {
		t.Fatalf("ReadFile(factory.json): %v", readErr)
	}
	if string(got) != string(initial) {
		t.Fatalf("factory.json after failed replace = %q, want unchanged payload", got)
	}
}

func TestReplaceFactorySplitLayout_WritesSplitLayoutAndRestores(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	updated := splitReplaceTestPayload(t, "alpha-v2")
	result, err := factoryconfig.ReplaceFactorySplitLayout(targetDir, updated)
	if err != nil {
		t.Fatalf("ReplaceFactorySplitLayout: %v", err)
	}

	assertSplitLayoutPersisted(t, targetDir, "alpha-v2")

	result.Restore()
	assertMonolithicFactoryJSON(t, targetDir, initial)
}

func TestReplaceFactorySplitLayout_ValidationFailureLeavesFactoryUnchanged(t *testing.T) {
	targetDir := t.TempDir()
	initial := []byte(`{
		"name": "alpha",
		"id": "alpha",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-alpha","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	_, err := factoryconfig.ReplaceFactorySplitLayoutWithAfterStageHook(
		targetDir,
		splitReplaceTestPayload(t, "broken"),
		func(stagingDir string) error {
			path := filepath.Join(stagingDir, interfaces.WorkstationsDir, "execute-broken", interfaces.FactoryAgentsFileName)
			return os.WriteFile(path, []byte("---\ntype: [\n"), 0o644)
		},
	)
	if err == nil {
		t.Fatal("expected validation failure before commit")
	}
	if got := err.Error(); !strings.Contains(got, `validate factory`) || !strings.Contains(got, "AGENTS.md missing closing frontmatter delimiter") {
		t.Fatalf("expected load-time validation error, got %v", err)
	}

	got, readErr := os.ReadFile(filepath.Join(targetDir, interfaces.FactoryConfigFile))
	if readErr != nil {
		t.Fatalf("ReadFile(factory.json): %v", readErr)
	}
	if string(got) != string(initial) {
		t.Fatalf("factory.json after failed replace = %q, want unchanged payload", got)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, interfaces.WorkersDir)); !os.IsNotExist(statErr) {
		t.Fatalf("expected no workers directory after failed replace, got stat err=%v", statErr)
	}
}

func TestReplaceFactorySplitLayout_PrunesRemovedWorkerAndOverwritesAgents(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
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

	updated := splitReplaceTestPayload(t, "alpha-v2")
	result, err := factoryconfig.ReplaceFactorySplitLayout(targetDir, updated)
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

func TestReplaceFactoryLayoutAtDir_PrunesRemovedWorkerDirectory(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	removedWorkerDir := filepath.Join(targetDir, interfaces.WorkersDir, "removed")
	if err := os.MkdirAll(removedWorkerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workers/removed): %v", err)
	}
	if err := os.WriteFile(filepath.Join(removedWorkerDir, interfaces.FactoryAgentsFileName), []byte("orphan worker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(workers/removed/AGENTS.md): %v", err)
	}

	restore, err := factoryconfig.ReplaceFactoryLayoutAtDir(
		targetDir,
		splitReplaceTestPayload(t, "alpha-v2"),
		factoryconfig.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
	if err != nil {
		t.Fatalf("ReplaceFactoryLayoutAtDir: %v", err)
	}
	if restore == nil {
		t.Fatal("expected restore callback")
	}

	if _, err := os.Stat(removedWorkerDir); !os.IsNotExist(err) {
		t.Fatalf("expected workers/removed pruned after persist-from-save, stat err=%v", err)
	}
}

func TestReplaceFactoryLayoutAtDir_PrunesRemovedWorkstationDirectory(t *testing.T) {
	targetDir := t.TempDir()
	initial := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	removedWorkstationDir := filepath.Join(targetDir, interfaces.WorkstationsDir, "removed")
	if err := os.MkdirAll(removedWorkstationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workstations/removed): %v", err)
	}
	if err := os.WriteFile(filepath.Join(removedWorkstationDir, interfaces.FactoryAgentsFileName), []byte("orphan workstation\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(workstations/removed/AGENTS.md): %v", err)
	}

	restore, err := factoryconfig.ReplaceFactoryLayoutAtDir(
		targetDir,
		splitReplaceTestPayload(t, "alpha-v2"),
		factoryconfig.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
	if err != nil {
		t.Fatalf("ReplaceFactoryLayoutAtDir: %v", err)
	}
	if restore == nil {
		t.Fatal("expected restore callback")
	}

	if _, err := os.Stat(removedWorkstationDir); !os.IsNotExist(err) {
		t.Fatalf("expected workstations/removed pruned after persist-from-save, stat err=%v", err)
	}
}

func TestReplaceFactorySplitLayout_OverwritesExistingAgentsOnSave(t *testing.T) {
	targetDir := t.TempDir()
	payload := splitReplaceTestPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	result, err := factoryconfig.ReplaceFactorySplitLayout(targetDir, payload)
	if err != nil {
		t.Fatalf("ReplaceFactorySplitLayout initial: %v", err)
	}
	if result != nil && result.DiscardBackup != nil {
		result.DiscardBackup()
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

	result, err = factoryconfig.ReplaceFactorySplitLayout(targetDir, payload)
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

	loaded, err := factoryconfig.LoadRuntimeConfig(factoryDir, nil)
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

func splitReplaceTestPayload(t *testing.T, project string) []byte {
	t.Helper()
	return []byte(`{
		"name": "` + project + `",
		"id": "` + project + `",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-` + project + `","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
}
