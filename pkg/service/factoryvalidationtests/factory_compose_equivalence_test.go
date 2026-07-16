package factoryvalidationtests

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/testutil/testdeps"
)

func TestBuildFactoryCoreRejectsMissingWorkerApplicationBeforeLoading(t *testing.T) {
	t.Parallel()

	_, err := service.BuildFactoryCore(t.Context(), &service.FactoryServiceConfig{Dir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "worker application is required") {
		t.Fatalf("BuildFactoryCore error = %v, want missing worker application", err)
	}
}

func TestFactoryServiceComposeCollaboratorsMatchBuildFactoryService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := t.Context()
	cfg := testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{Dir: dir})

	built, err := service.BuildFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	root, err := service.ResolveFactoryServiceRoot(&service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	load, err := service.LoadFactoryConfigForCompose(&service.FactoryServiceConfig{Dir: dir}, root)
	if err != nil {
		t.Fatalf("LoadFactoryConfigForCompose: %v", err)
	}
	clock := service.ServiceClockForCompose(&service.FactoryServiceConfig{Dir: dir}, load)
	composeCfg, err := service.ConfigWithWorkerApplication(&service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("construct compose worker application: %v", err)
	}
	hostedWorkers := composeCfg.WorkerApplication.Hosted
	collaborators, err := service.NewFactoryServiceCollaborators(
		composeCfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
		hostedWorkers,
	)
	if err != nil {
		t.Fatalf("NewFactoryServiceCollaborators: %v", err)
	}
	shell, err := service.ComposeFactoryService(
		ctx,
		composeCfg,
		root,
		collaborators,
		load,
		clock,
		hostedWorkers,
	)
	if err != nil {
		t.Fatalf("ComposeFactoryService: %v", err)
	}
	modelDeps, err := service.ModelServiceDependencies(shell)
	if err != nil {
		t.Fatalf("ModelServiceDependencies: %v", err)
	}
	modelService, err := modelsservice.NewService(modelDeps)
	if err != nil {
		t.Fatalf("modelsservice.NewService: %v", err)
	}
	composed := service.AttachModelServiceCollaborator(shell, modelService)
	composed = service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Service: composed},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Service: composed}, composeCfg),
	)

	if built.ComposeCollaboratorSnapshot() != composed.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: built=%+v composed=%+v", built.ComposeCollaboratorSnapshot(), composed.ComposeCollaboratorSnapshot())
	}
}

func TestFactoryCoreComposeCollaboratorsMatchBuildFactoryCore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := t.Context()
	cfg := &service.FactoryServiceConfig{Dir: dir}
	cfg, err := service.ConfigWithWorkerApplication(cfg)
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}

	built, err := service.BuildFactoryCore(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryCore: %v", err)
	}

	root, err := service.ResolveFactoryServiceRoot(&service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	load, err := service.LoadFactoryConfigForCompose(&service.FactoryServiceConfig{Dir: dir}, root)
	if err != nil {
		t.Fatalf("LoadFactoryConfigForCompose: %v", err)
	}
	clock := service.ServiceClockForCompose(&service.FactoryServiceConfig{Dir: dir}, load)
	composeCfg, err := service.ConfigWithWorkerApplication(&service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("construct compose worker application: %v", err)
	}
	hostedWorkers := composeCfg.WorkerApplication.Hosted
	collaborators, err := service.NewFactoryServiceCollaborators(
		composeCfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
		hostedWorkers,
	)
	if err != nil {
		t.Fatalf("NewFactoryServiceCollaborators: %v", err)
	}
	composed, err := service.ComposeFactoryCore(
		ctx,
		composeCfg,
		root,
		collaborators,
		load,
		clock,
		hostedWorkers,
	)
	if err != nil {
		t.Fatalf("ComposeFactoryCore: %v", err)
	}

	if built.ComposeCollaboratorSnapshot() != composed.ComposeCollaboratorSnapshot() {
		t.Fatalf("core compose snapshot mismatch: built=%+v composed=%+v", built.ComposeCollaboratorSnapshot(), composed.ComposeCollaboratorSnapshot())
	}
	if built.Sessions() == nil || built.RuntimeBuild() == nil || built.WorkersScheduler() == nil || built.StartupBundle() == nil {
		t.Fatalf("BuildFactoryCore omitted required collaborators: snapshot=%+v", built.ComposeCollaboratorSnapshot())
	}
}

func TestFactoryServiceComposeCollaboratorsMatchBuildFactoryServiceWithOperatorDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := t.Context()
	cfg := testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{
		Dir: dir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModel:               "gpt-5-codex",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
		},
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	cfg, err := service.ConfigWithWorkerApplication(cfg)
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}

	built, err := service.BuildFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	root, err := service.ResolveFactoryServiceRoot(cfg)
	if err != nil {
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	load, err := service.LoadFactoryConfigForCompose(cfg, root)
	if err != nil {
		t.Fatalf("LoadFactoryConfigForCompose: %v", err)
	}
	clock := service.ServiceClockForCompose(cfg, load)
	hostedWorkers := cfg.WorkerApplication.Hosted
	collaborators, err := service.NewFactoryServiceCollaborators(
		cfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
		hostedWorkers,
	)
	if err != nil {
		t.Fatalf("NewFactoryServiceCollaborators: %v", err)
	}
	shell, err := service.ComposeFactoryService(
		ctx,
		cfg,
		root,
		collaborators,
		load,
		clock,
		hostedWorkers,
	)
	if err != nil {
		t.Fatalf("ComposeFactoryService: %v", err)
	}
	modelDeps, err := service.ModelServiceDependencies(shell)
	if err != nil {
		t.Fatalf("ModelServiceDependencies: %v", err)
	}
	modelService, err := modelsservice.NewService(modelDeps)
	if err != nil {
		t.Fatalf("modelsservice.NewService: %v", err)
	}
	composed := service.AttachModelServiceCollaborator(shell, modelService)
	composed = service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Service: composed},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Service: composed}, cfg),
	)

	if built.ComposeCollaboratorSnapshot() != composed.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: built=%+v composed=%+v", built.ComposeCollaboratorSnapshot(), composed.ComposeCollaboratorSnapshot())
	}
}
