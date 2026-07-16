package factoryvalidationtests

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/testutil/testdeps"
)

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
	composeCfg := &service.FactoryServiceConfig{Dir: dir}
	hostedWorkers := service.NewHostedWorkersConfig(composeCfg, root.BaseLogger, clock)
	collaborators := service.NewFactoryServiceCollaborators(
		composeCfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
		hostedWorkers,
	)
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
	composed := service.AttachModelServiceCollaborator(shell, service.ProvideModelServiceCollaborator(shell, composeCfg))
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
	composeCfg := &service.FactoryServiceConfig{Dir: dir}
	hostedWorkers := service.NewHostedWorkersConfig(composeCfg, root.BaseLogger, clock)
	collaborators := service.NewFactoryServiceCollaborators(
		composeCfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
		hostedWorkers,
	)
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
	hostedWorkers := service.NewHostedWorkersConfig(cfg, root.BaseLogger, clock)
	collaborators := service.NewFactoryServiceCollaborators(
		cfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
		hostedWorkers,
	)
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
	composed := service.AttachModelServiceCollaborator(shell, service.ProvideModelServiceCollaborator(shell, cfg))
	composed = service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Service: composed},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Service: composed}, cfg),
	)

	if built.ComposeCollaboratorSnapshot() != composed.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: built=%+v composed=%+v", built.ComposeCollaboratorSnapshot(), composed.ComposeCollaboratorSnapshot())
	}
}
