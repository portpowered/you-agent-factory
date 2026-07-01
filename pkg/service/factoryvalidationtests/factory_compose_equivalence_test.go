package factoryvalidationtests

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestFactoryServiceComposeCollaboratorsMatchBuildFactoryService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := t.Context()
	cfg := &service.FactoryServiceConfig{Dir: dir}

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
	collaborators := service.NewFactoryServiceCollaborators(
		&service.FactoryServiceConfig{Dir: dir},
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
	)
	composeCfg := &service.FactoryServiceConfig{Dir: dir}
	shell, err := service.ComposeFactoryService(
		ctx,
		composeCfg,
		root,
		collaborators,
		load,
		clock,
		service.NewHostedWorkersConfig(composeCfg, root.BaseLogger, clock),
	)
	if err != nil {
		t.Fatalf("ComposeFactoryService: %v", err)
	}
	composed := service.AttachModelServiceCollaborator(shell, service.ProvideModelServiceCollaborator(shell, composeCfg))
	composed = service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Host: composed},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Host: composed}, composeCfg),
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
	collaborators := service.NewFactoryServiceCollaborators(
		&service.FactoryServiceConfig{Dir: dir},
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
	)
	composed, err := service.ComposeFactoryCore(
		ctx,
		&service.FactoryServiceConfig{Dir: dir},
		root,
		collaborators,
		load,
		clock,
		service.NewHostedWorkersConfig(&service.FactoryServiceConfig{Dir: dir}, root.BaseLogger, clock),
	)
	if err != nil {
		t.Fatalf("ComposeFactoryCore: %v", err)
	}

	if built.ComposeCollaboratorSnapshot() != composed.ComposeCollaboratorSnapshot() {
		t.Fatalf("core compose snapshot mismatch: built=%+v composed=%+v", built.ComposeCollaboratorSnapshot(), composed.ComposeCollaboratorSnapshot())
	}
	if built.Sessions() == nil || built.RuntimeBuild() == nil || built.StartupBundle() == nil {
		t.Fatalf("BuildFactoryCore omitted required collaborators: snapshot=%+v", built.ComposeCollaboratorSnapshot())
	}
}

func TestFactoryServiceComposeCollaboratorsMatchBuildFactoryServiceWithOperatorDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := t.Context()
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
	collaborators := service.NewFactoryServiceCollaborators(
		cfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
	)
	shell, err := service.ComposeFactoryService(
		ctx,
		cfg,
		root,
		collaborators,
		load,
		clock,
		service.NewHostedWorkersConfig(cfg, root.BaseLogger, clock),
	)
	if err != nil {
		t.Fatalf("ComposeFactoryService: %v", err)
	}
	composed := service.AttachModelServiceCollaborator(shell, service.ProvideModelServiceCollaborator(shell, cfg))
	composed = service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Host: composed},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Host: composed}, cfg),
	)

	if built.ComposeCollaboratorSnapshot() != composed.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: built=%+v composed=%+v", built.ComposeCollaboratorSnapshot(), composed.ComposeCollaboratorSnapshot())
	}
}
