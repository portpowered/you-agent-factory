package opencodeagenttests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

func TestBuildFactoryService_RejectsOpenCodeAgentOnNonOpenCodeRunner(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
openCodeAgent: reviewer
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "process")

	_, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:                                     dir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err == nil || !strings.Contains(err.Error(), "openCodeAgent") || !strings.Contains(err.Error(), "reviewer") {
		t.Fatalf("BuildFactoryService error = %v, want openCodeAgent configuration error", err)
	}
}

func TestBuildFactoryService_AllowsOpenCodeAgentWithOpenCodeFactoryRunner(t *testing.T) {
	dir := t.TempDir()
	cfg := minimalFactoryConfig()
	cfg["runner"] = interfaces.RunnerIDOpenCode
	writeFactoryJSON(t, dir, cfg)
	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
openCodeAgent: reviewer
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "process")

	_, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:                                     dir,
		RunnerID:                                interfaces.RunnerIDOpenCode,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService error = %v, want openCodeAgent with opencode runner", err)
	}
}

func TestBuildFactoryService_AllowsUnsetOpenCodeAgentOnNonOpenCodeRunner(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "process")

	_, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:                                     dir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService error = %v, want unchanged non-opencode runner behavior", err)
	}
}

func minimalFactoryConfig() map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func writeFactoryJSON(t *testing.T, dir string, cfg map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
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
