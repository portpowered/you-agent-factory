package compose_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

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
