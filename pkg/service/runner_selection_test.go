package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestLoadWorkersFromConfig_RejectsUnknownConfiguredRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: codex
---
You are a helpful assistant.
`)
	workstationDir := filepath.Join(dir, "workstations", "review")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(`---
type: MODEL_WORKSTATION
worker: worker-a
runner: mystery-runner
---
Review.
`), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	_, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, logging.NoopLogger{}, false, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), `unknown runner "mystery-runner"`) {
		t.Fatalf("loadWorkersFromConfig error = %v, want unknown runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableGeminiFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), interfaces.RunnerIDGemini, cfg, logging.NoopLogger{}, false, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available gemini runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableKiroFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), interfaces.RunnerIDKiro, cfg, logging.NoopLogger{}, false, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available kiro runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableCursorFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), interfaces.RunnerIDCursorCLI, cfg, logging.NoopLogger{}, false, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available cursor runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableOpenCodeFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), interfaces.RunnerIDOpenCode, cfg, logging.NoopLogger{}, false, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available opencode runner", err)
	}
}

func TestLoadWorkersFromConfig_RejectsUnknownFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	_, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), "mystery-runner", cfg, logging.NoopLogger{}, false, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), `unknown runner "mystery-runner"`) {
		t.Fatalf("loadWorkersFromConfig error = %v, want unknown runner", err)
	}
}

func TestValidateConfiguredWorkstationRunners_AcceptsLegacyBuiltInRunnerDefault(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			WorkerTypeName: "worker-a",
		}},
	}
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &interfaces.WorkerConfig{
		Name:          "worker-a",
		ModelProvider: interfaces.RunnerIDCodex,
	})

	if err := validateConfiguredWorkstationRunners(cfg, "", runtimeCfg, runnerSelectionPreflight{skipCommandAvailability: true}); err != nil {
		t.Fatalf("validateConfiguredWorkstationRunners: %v", err)
	}
}

func TestValidateConfiguredWorkstationRunners_SkipsDefaultFallbackValidation(t *testing.T) {
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

	if err := validateConfiguredWorkstationRunners(cfg, "", runtimeCfg, runnerSelectionPreflight{}); err != nil {
		t.Fatalf("validateConfiguredWorkstationRunners: %v", err)
	}
}

func TestEffectiveFactoryRunnerID_PrefersExplicitOverrideThenConfig(t *testing.T) {
	cfg := &interfaces.FactoryConfig{Runner: interfaces.RunnerIDGemini}

	if got := effectiveFactoryRunnerID("  cursor-cli  ", cfg); got != interfaces.RunnerIDCursorCLI {
		t.Fatalf("effectiveFactoryRunnerID override = %q, want %q", got, interfaces.RunnerIDCursorCLI)
	}
	if got := effectiveFactoryRunnerID("", cfg); got != interfaces.RunnerIDGemini {
		t.Fatalf("effectiveFactoryRunnerID config = %q, want %q", got, interfaces.RunnerIDGemini)
	}
	if got := effectiveFactoryRunnerID("", nil); got != "" {
		t.Fatalf("effectiveFactoryRunnerID nil config = %q, want empty", got)
	}
}

func configFixtureWithWorkerAndWorkstation(workerName, workstationName string, worker *interfaces.WorkerConfig) configRuntimeFixture {
	return configRuntimeFixture{
		workers: map[string]*interfaces.WorkerConfig{
			workerName: worker,
		},
		workstations: map[string]*interfaces.FactoryWorkstationConfig{
			workstationName: {Name: workstationName, WorkerTypeName: workerName},
		},
	}
}

type configRuntimeFixture struct {
	workers      map[string]*interfaces.WorkerConfig
	workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (f configRuntimeFixture) Worker(name string) (*interfaces.WorkerConfig, bool) {
	worker, ok := f.workers[name]
	return worker, ok
}

func (f configRuntimeFixture) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.workstations[name]
	return workstation, ok
}

func (configRuntimeFixture) FactoryDir() string     { return "" }
func (configRuntimeFixture) RuntimeBaseDir() string { return "" }

var _ interfaces.RuntimeConfigLookup = configRuntimeFixture{}
