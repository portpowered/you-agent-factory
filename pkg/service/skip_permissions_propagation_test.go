package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverridePropagatesToAgentProviderCommand(t *testing.T) {
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
}

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverrideDoesNotApplyToModelWorker(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "model-a", `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "model-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"model-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "model-a")),
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

	executeProviderBackedWorker(t, opts, "model-a", runner)
	assertProviderArgsDoNotContain(t, runner.Requests(), "--dangerously-skip-permissions")
}

func TestLoadWorkersFromConfig_PersistedSkipPermissionsTrueWithoutInvocationOverride(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: true
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
	assertProviderArgsContain(t, runner.Requests(), "--dangerously-skip-permissions")
}

func executeProviderBackedWorker(
	t *testing.T,
	opts []factory.FactoryOption,
	workerName string,
	runner *providerCommandRunnerRecorder,
) {
	t.Helper()

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors[workerName]
	if !ok {
		t.Fatalf("expected %q executor to be registered", workerName)
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	if _, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "dispatch-skip-permissions",
		TransitionID:    "transition-skip-permissions",
		WorkerType:      workerName,
		WorkstationName: "review",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-skip-permissions",
			Color: interfaces.TokenColor{
				WorkID:  "work-skip-permissions",
				Payload: []byte("helpful input"),
			},
		}),
	}); err != nil {
		t.Fatalf("execute worker: %v", err)
	}
	if len(runner.Requests()) != 1 {
		t.Fatalf("provider command count = %d, want 1", len(runner.Requests()))
	}
}

func assertProviderArgsContain(t *testing.T, requests []workers.CommandRequest, want string) {
	t.Helper()
	if len(requests) == 0 {
		t.Fatal("expected provider command requests")
	}
	joined := strings.Join(requests[0].Args, " ")
	if !strings.Contains(joined, want) {
		t.Fatalf("provider args = %q, want substring %q", joined, want)
	}
}

func assertProviderArgsDoNotContain(t *testing.T, requests []workers.CommandRequest, unwanted string) {
	t.Helper()
	if len(requests) == 0 {
		t.Fatal("expected provider command requests")
	}
	joined := strings.Join(requests[0].Args, " ")
	if strings.Contains(joined, unwanted) {
		t.Fatalf("provider args = %q, want to omit %q", joined, unwanted)
	}
}
