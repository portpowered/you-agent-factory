package compose_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestInjectCLIRunner_ComposesDashboardEnabledRunsOutsideFactoryService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	runner, err := compose.InjectCLIRunner(context.Background(), &service.FactoryServiceConfig{
		Dir:                     dir,
		SimpleDashboardRenderer: func(service.SimpleDashboardRenderInput) {},
	})
	if err != nil {
		t.Fatalf("InjectCLIRunner: %v", err)
	}
	if _, legacy := runner.(*service.FactoryService); legacy {
		t.Fatal("dashboard-enabled runner used legacy FactoryService composition")
	}
}

func TestInjectCLIRunner_ConstructsDashboardSuppressedGraphWithoutStartingIt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	runner, err := compose.InjectCLIRunner(context.Background(), &service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("InjectCLIRunner: %v", err)
	}
	application, ok := runner.(*initializer.Application)
	if !ok || application.Graph() == nil {
		t.Fatalf("dashboard-suppressed runner type = %T, want graph-backed initializer application", runner)
	}
}

func TestInjectCLIRunner_DashboardSuppressedQuietBatchPreservesWorkFileAndBatchMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	workFile := filepath.Join(dir, "initial-work.json")
	if err := os.WriteFile(workFile, []byte(`{"requestId":"quiet-batch","type":"FACTORY_REQUEST_BATCH","works":[{"name":"quiet-work","workTypeName":"task","payload":"quiet batch"}]}`), 0o600); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{
		Dir:                                     dir,
		WorkFile:                                workFile,
		RuntimeMode:                             interfaces.RuntimeModeBatch,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	}

	runner, err := compose.InjectCLIRunner(ctx, cfg)
	if err != nil {
		t.Fatalf("InjectCLIRunner: %v", err)
	}
	if application, ok := runner.(*initializer.Application); !ok || application.Graph() == nil {
		t.Fatalf("runner type = %T, want graph-backed initializer application", runner)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- runner.Run(ctx) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("quiet batch Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quiet batch did not consume its WorkFile and terminate in batch mode")
	}
}

func TestInjectFactoryService_RejectsMissingFactoryDir(t *testing.T) {
	t.Parallel()

	_, err := compose.InjectFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir: filepath.Join(t.TempDir(), "missing-factory"),
	})
	if err == nil {
		t.Fatal("expected error for missing factory dir")
	}
}

func TestInjectFactoryService_MatchesBuildFactoryServiceCollaborators(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{"name":"wire-compose","workTypes":[]}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{Dir: dir}

	wireBuilt, err := compose.InjectFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("InjectFactoryService: %v", err)
	}
	directBuilt, err := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if directBuilt.ComposeCollaboratorSnapshot() != wireBuilt.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: direct=%+v wire=%+v", directBuilt.ComposeCollaboratorSnapshot(), wireBuilt.ComposeCollaboratorSnapshot())
	}
	if wireBuilt.DurableExecutionService() == nil {
		t.Fatal("wire composition did not inject durable execution")
	}
	firstDurable := wireBuilt.DurableExecutionService()
	secondDurable := wireBuilt.DurableExecutionService()
	if firstDurable != secondDurable {
		t.Fatal("wire composition returned more than one durable execution collaborator")
	}
}

func TestInjectFactoryService_DurableExecutionUsesExecutionBaseDir(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, factoryfixtures.MinimalFactoryConfig())
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o700); err != nil {
		t.Fatalf("create workflow dir: %v", err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "orchestrators", "javascript", "runtime", "testdata", "simple-final.workflow.js"))
	if err != nil {
		t.Fatalf("read workflow fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "simple-final.js"), fixture, 0o600); err != nil {
		t.Fatalf("write workflow fixture: %v", err)
	}

	svc, err := compose.InjectFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:              factoryDir,
		ExecutionBaseDir: projectRoot,
	})
	if err != nil {
		t.Fatalf("InjectFactoryService: %v", err)
	}
	workflowName := "simple-final"
	if _, err := svc.StartDurableFactorySessionSync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-compose-execution-root",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		},
	}); err != nil {
		t.Fatalf("start workflow from execution base dir: %v", err)
	}
}

func TestInjectFactoryService_BuildsMinimalFactory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{"name":"wire-bootstrap","workTypes":[]}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	svc, err := compose.InjectFactoryService(context.Background(), &service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("InjectFactoryService: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil FactoryService")
	}
}

func TestInjectFactoryService_ResolvesBackendScopeBeforeSessionIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{"name":"wire-backend-scope","workTypes":[]}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	homeDir := t.TempDir()

	_, err := compose.InjectFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:                                     dir,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("InjectFactoryService: %v", err)
	}

	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", configPath, err)
	}
	var persisted struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !strings.HasPrefix(persisted.BackendScopeID, "local-") {
		t.Fatalf("persisted backendScopeID = %q, want local-<uuid>", persisted.BackendScopeID)
	}
}

func TestInjectFactoryService_MatchesBuildFactoryServiceWithResolvedOperatorDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{
  "name": "wire-operator-defaults",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "executor",
    "type": "MODEL_WORKER",
    "body": "You are the executor."
  }],
  "workstations": [{
    "name": "execute-task",
    "worker": "executor",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "type": "MODEL_WORKSTATION",
    "body": "Implement {{ .WorkID }}."
  }]
}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{
		Dir: dir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModel:               "gpt-5-codex",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
		},
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	}

	wireBuilt, err := compose.InjectFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("InjectFactoryService: %v", err)
	}
	directBuilt, err := service.BuildFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if directBuilt.ComposeCollaboratorSnapshot() != wireBuilt.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: direct=%+v wire=%+v",
			directBuilt.ComposeCollaboratorSnapshot(), wireBuilt.ComposeCollaboratorSnapshot())
	}

	assertComposeOperatorDefaultedExecutorWorker(t, directBuilt)
	assertComposeOperatorDefaultedExecutorWorker(t, wireBuilt)
}

func TestInjectFactoryService_IgnoresOperatorDefaultEnvironmentWhenConfigSupplied(t *testing.T) {
	t.Setenv(operatorconfig.EnvDefaultWorkerModelProvider, "claude")
	t.Setenv(operatorconfig.EnvDefaultWorkerModel, "claude-sonnet-4-20250514")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{
  "name": "wire-env-ignore",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "executor",
    "type": "MODEL_WORKER",
    "body": "You are the executor."
  }],
  "workstations": [{
    "name": "execute-task",
    "worker": "executor",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "type": "MODEL_WORKSTATION",
    "body": "Implement {{ .WorkID }}."
  }]
}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{
		Dir: dir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModel:               "gpt-5-codex",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
		},
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	}

	wireBuilt, err := compose.InjectFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("InjectFactoryService: %v", err)
	}
	directBuilt, err := service.BuildFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if directBuilt.ComposeCollaboratorSnapshot() != wireBuilt.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: direct=%+v wire=%+v",
			directBuilt.ComposeCollaboratorSnapshot(), wireBuilt.ComposeCollaboratorSnapshot())
	}

	assertComposeOperatorDefaultedExecutorWorker(t, wireBuilt)
}

func assertComposeOperatorDefaultedExecutorWorker(t *testing.T, svc *service.FactoryService) {
	t.Helper()
	worker, ok := svc.StartupWorkerConfig("executor")
	if !ok || worker == nil {
		t.Fatal("expected executor worker")
	}
	if worker.ModelProvider != string(interfaces.ModelProviderCodex) {
		t.Fatalf("modelProvider = %q, want %q", worker.ModelProvider, interfaces.ModelProviderCodex)
	}
	if worker.Model != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", worker.Model)
	}
}
