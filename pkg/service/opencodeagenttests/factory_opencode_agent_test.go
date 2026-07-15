package opencodeagenttests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/service"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestBuildFactoryService_RejectsOpenCodeAgentOnNonOpenCodeRunner(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
openCodeAgent: reviewer
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "process")

	_, err := service.BuildFactoryService(context.Background(), testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{
		Dir:                                     dir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}))
	if err == nil || !strings.Contains(err.Error(), "openCodeAgent") || !strings.Contains(err.Error(), "reviewer") {
		t.Fatalf("BuildFactoryService error = %v, want openCodeAgent configuration error", err)
	}
}

func TestBuildFactoryService_AllowsOpenCodeAgentWithOpenCodeFactoryRunner(t *testing.T) {
	dir := t.TempDir()
	cfg := factoryfixtures.MinimalFactoryConfig()
	cfg["runner"] = workerexecution.RunnerIDOpenCode
	factoryfixtures.WriteFactoryJSON(t, dir, cfg)
	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
openCodeAgent: reviewer
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "process")

	_, err := service.BuildFactoryService(context.Background(), testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{
		Dir:                                     dir,
		RunnerID:                                workerexecution.RunnerIDOpenCode,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}))
	if err != nil {
		t.Fatalf("BuildFactoryService error = %v, want openCodeAgent with opencode runner", err)
	}
}

func TestBuildFactoryService_AllowsUnsetOpenCodeAgentOnNonOpenCodeRunner(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "process")

	_, err := service.BuildFactoryService(context.Background(), testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{
		Dir:                                     dir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}))
	if err != nil {
		t.Fatalf("BuildFactoryService error = %v, want unchanged non-opencode runner behavior", err)
	}
}

func writeWorkerAgentsMDWithContent(t *testing.T, factoryDir, workerName, content string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}
