package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverrideFailsForUnsupportedAgentProvider(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: custom-model
modelProvider: acme
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "agent-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	_, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err == nil {
		t.Fatal("expected loadWorkersFromConfig to fail for unsupported provider with --skip-permissions")
	}
	if !strings.Contains(err.Error(), "skip-permissions") {
		t.Fatalf("error = %q, want skip-permissions failure", err.Error())
	}
	if !strings.Contains(err.Error(), "acme") {
		t.Fatalf("error = %q, want unsupported provider detail", err.Error())
	}
	if len(runner.Requests()) != 0 {
		t.Fatalf("provider command count = %d, want 0 before dispatch", len(runner.Requests()))
	}
}

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverrideFailsForLocalManagedAgentWorker(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: OMNIVOICE_Q4_K_M
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers: []interfaces.WorkerConfig{{
			Name:          "agent-a",
			ModelLocality: interfaces.ModelLocalityLocal,
		}},
	},
		map[string]*interfaces.WorkerConfig{
			"agent-a": func() *interfaces.WorkerConfig {
				worker := mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a"))
				worker.ModelLocality = interfaces.ModelLocalityLocal
				return worker
			}(),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	_, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err == nil {
		t.Fatal("expected loadWorkersFromConfig to fail for local managed agent worker with --skip-permissions")
	}
	if !strings.Contains(err.Error(), "local managed model workers cannot honor CLI skip-permissions") {
		t.Fatalf("error = %q, want local managed model failure detail", err.Error())
	}
	if len(runner.Requests()) != 0 {
		t.Fatalf("provider command count = %d, want 0 before dispatch", len(runner.Requests()))
	}
}

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverrideFailsWhenFactoryHasUnsupportedAgentPeer(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-supported", `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkerAgentsMDWithContent(t, dir, "agent-unsupported", `---
type: AGENT_WORKER
model: custom-model
modelProvider: acme
stopToken: COMPLETE
skipPermissions: false
---
You are another agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers: []interfaces.WorkerConfig{
			{Name: "agent-supported"},
			{Name: "agent-unsupported"},
		},
	},
		map[string]*interfaces.WorkerConfig{
			"agent-supported":   mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-supported")),
			"agent-unsupported": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-unsupported")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	_, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err == nil {
		t.Fatal("expected mixed factory to fail when --skip-permissions is set and any agent worker is unsupported")
	}
	if !strings.Contains(err.Error(), "agent-unsupported") {
		t.Fatalf("error = %q, want unsupported worker name", err.Error())
	}
	if len(runner.Requests()) != 0 {
		t.Fatalf("provider command count = %d, want 0 before dispatch", len(runner.Requests()))
	}
}

func TestLoadWorkersFromConfig_UnsupportedAgentProviderWithoutInvocationOverrideDoesNotFail(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: custom-model
modelProvider: acme
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "agent-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	opts, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		nil,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig without override: %v", err)
	}

	executeProviderBackedWorker(t, opts, "agent-a", runner)
	assertProviderArgsDoNotContain(t, runner.Requests(), "--dangerously-skip-permissions")
}
