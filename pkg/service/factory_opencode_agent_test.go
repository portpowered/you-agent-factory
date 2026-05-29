package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestRunnerSelectionValidation_RejectsOpenCodeAgentOnNonOpenCodeRunner(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			WorkerTypeName: "worker-a",
		}},
	}
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &interfaces.WorkerConfig{
		Name:          "worker-a",
		ModelProvider: "claude",
		OpenCodeAgent: "reviewer",
	})

	err := validateConfiguredWorkstationRunners(cfg, "", runtimeCfg, runnerSelectionPreflight{skipCommandAvailability: true})
	if err == nil || !strings.Contains(err.Error(), "openCodeAgent") || !strings.Contains(err.Error(), "reviewer") || !strings.Contains(err.Error(), interfaces.RunnerIDCodex) {
		t.Fatalf("validateConfiguredWorkstationRunners error = %v, want openCodeAgent mismatch with codex runner", err)
	}
}

func TestRunnerSelectionValidation_AllowsOpenCodeAgentWithOpenCodeRunner(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Runner: interfaces.RunnerIDOpenCode,
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			WorkerTypeName: "worker-a",
		}},
	}
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &interfaces.WorkerConfig{
		Name:          "worker-a",
		ModelProvider: "claude",
		OpenCodeAgent: "reviewer",
	})

	if err := validateConfiguredWorkstationRunners(cfg, interfaces.RunnerIDOpenCode, runtimeCfg, runnerSelectionPreflight{skipCommandAvailability: true}); err != nil {
		t.Fatalf("validateConfiguredWorkstationRunners error = %v, want openCodeAgent with opencode runner", err)
	}
}

func TestRunnerSelectionValidation_AllowsUnsetOpenCodeAgentOnNonOpenCodeRunner(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			WorkerTypeName: "worker-a",
		}},
	}
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &interfaces.WorkerConfig{
		Name:          "worker-a",
		ModelProvider: "claude",
	})

	if err := validateConfiguredWorkstationRunners(cfg, "", runtimeCfg, runnerSelectionPreflight{skipCommandAvailability: true}); err != nil {
		t.Fatalf("validateConfiguredWorkstationRunners error = %v, want unchanged non-opencode runner behavior", err)
	}
}

func TestLoadWorkersFromConfig_RejectsOpenCodeAgentOnNonOpenCodeRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
openCodeAgent: reviewer
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review", WorkerTypeName: "worker-a"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	_, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, logging.NoopLogger{}, true, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "openCodeAgent") || !strings.Contains(err.Error(), "reviewer") {
		t.Fatalf("loadWorkersFromConfig error = %v, want openCodeAgent configuration error", err)
	}
}
