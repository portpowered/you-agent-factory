package initializer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestInitialize_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &initializer.Config{Dir: t.TempDir()}

	services, errInit := initializer.Initialize(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, cfg)

	if services != nil {
		t.Fatal("expected Initialize to return nil services without factory.json")
	}
	if errInit == nil {
		t.Fatal("expected Initialize to fail without factory.json")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("Initialize error = %q, want %q", errInit, errService)
	}
}

func TestInitialize_RejectsInvalidFactoryConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{
  "name": "invalid",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [],
  "workstations": [{
    "name": "orphan",
    "worker": "does-not-exist",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "type": "MODEL_WORKSTATION"
  }]
}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	cfg := &initializer.Config{Dir: dir}

	services, errInit := initializer.Initialize(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, cfg)

	if services != nil {
		t.Fatal("expected Initialize to return nil services for invalid workstation worker reference")
	}
	if errInit == nil {
		t.Fatal("expected Initialize to fail for invalid workstation worker reference")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail for invalid workstation worker reference")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("Initialize error = %q, want %q", errInit, errService)
	}
}

func TestInitialize_ComposesDomainServicesWithoutFactoryService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	services, err := initializer.Initialize(ctx, &initializer.Config{Dir: dir})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if services.Sessions == nil {
		t.Fatal("expected session registry")
	}
	if services.Sessions.Get(factorysessions.DefaultSessionID) == nil {
		t.Fatal("expected default session in registry")
	}
	if services.FactoryDefinition == nil {
		t.Fatal("expected factory definition service")
	}
	if services.Models == nil {
		t.Fatal("expected model service")
	}
	if services.Workers.Logger == nil {
		t.Fatal("expected hosted workers config with logger")
	}
	if services.RuntimeHost == nil {
		t.Fatal("expected runtime host service")
	}

	current, err := services.FactoryDefinition.GetCurrentNamedFactory(ctx)
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory: %v", err)
	}
	if current.Name == "" {
		t.Fatal("expected current factory name")
	}

	models, err := services.Models.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if models.Results == nil {
		t.Fatal("expected model list payload")
	}
}

func TestInitialize_MatchesBuildFactoryServiceOperatorDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{
  "name": "initializer-operator-defaults",
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
	cfg := &initializer.Config{
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

	services, err := initializer.Initialize(ctx, cfg)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	svc, err := service.BuildFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	worker, ok := services.StartupWorkerConfig("executor")
	if !ok || worker == nil {
		t.Fatal("expected executor worker from initializer startup runtime")
	}
	if worker.ModelProvider != string(interfaces.ModelProviderCodex) {
		t.Fatalf("initializer modelProvider = %q, want %q", worker.ModelProvider, interfaces.ModelProviderCodex)
	}

	serviceWorker, ok := svc.StartupWorkerConfig("executor")
	if !ok || serviceWorker == nil {
		t.Fatal("expected executor worker from service startup runtime")
	}
	if worker.ModelProvider != serviceWorker.ModelProvider || worker.Model != serviceWorker.Model {
		t.Fatalf("initializer worker %+v != service worker %+v", worker, serviceWorker)
	}
}

func TestInitialize_GetCurrentFactoryForSession_DefaultSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	services, err := initializer.Initialize(ctx, &initializer.Config{Dir: dir})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	factory, err := services.FactoryDefinition.GetCurrentFactoryForSession(ctx, factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession: %v", err)
	}
	if factory.Name == "" {
		t.Fatal("expected default session factory name")
	}
}
