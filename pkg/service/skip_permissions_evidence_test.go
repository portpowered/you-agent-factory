package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestS14AbsentOverrideUsesPersistedFalseOnly(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
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
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	executeProviderBackedWorker(t, opts, "agent-a", runner)
	assertProviderArgsDoNotContain(t, runner.Requests(), "--dangerously-skip-permissions")
}

func TestS14InvocationSkipPermissionsDoesNotMutateWorkerAgentsFrontmatter(t *testing.T) {
	dir := t.TempDir()

	agentsPath := filepath.Join(dir, "workers", "agent-a", "AGENTS.md")
	agentsContent := `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`
	writeWorkerAgentsMDWithContent(t, dir, "agent-a", agentsContent)
	writeWorkstationAgentsMD(t, dir, "review")

	before, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md before load: %v", err)
	}

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
	opts, err := loadWorkersFromConfig(
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
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	executeProviderBackedWorker(t, opts, "agent-a", runner)
	assertProviderArgsContain(t, runner.Requests(), "--dangerously-skip-permissions")

	after, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md after load: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("AGENTS.md changed after invocation override:\nbefore=%q\nafter=%q", before, after)
	}
	if !strings.Contains(string(after), "skipPermissions: false") {
		t.Fatalf("AGENTS.md = %q, want persisted skipPermissions:false unchanged", string(after))
	}
	if strings.Contains(string(after), "skipPermissions: true") {
		t.Fatalf("AGENTS.md = %q, want skipPermissions not persisted as true", string(after))
	}
}

func TestS14UnsupportedProviderFailsBeforeDispatchEvidence(t *testing.T) {
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
		t.Fatal("expected unsupported provider to fail before dispatch when --skip-permissions is set")
	}
	if !strings.Contains(err.Error(), "skip-permissions") {
		t.Fatalf("error = %q, want skip-permissions failure", err.Error())
	}
	if len(runner.Requests()) != 0 {
		t.Fatalf("provider command count = %d, want 0 before dispatch", len(runner.Requests()))
	}
}
