package factoryvalidationtests

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/testutil/testdeps"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"go.uber.org/zap"
)

type commandRunnerProbe struct{}

func (*commandRunnerProbe) Run(context.Context, workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	return workerprocess.CommandResult{}, nil
}

func TestConfigWithWorkerApplicationPreservesDistinctCommandRunnerOverrides(t *testing.T) {
	t.Parallel()

	providerRunner := &commandRunnerProbe{}
	scriptRunner := &commandRunnerProbe{}
	cfg, err := service.ConfigWithWorkerApplication(&service.FactoryServiceConfig{
		ProviderCommandRunnerOverride: workers.CommandRunner(providerRunner),
		CommandRunnerOverride:         workers.CommandRunner(scriptRunner),
	})
	if err != nil {
		t.Fatalf("ConfigWithWorkerApplication: %v", err)
	}
	if cfg.WorkerApplication.ProviderCommandRunner != providerRunner {
		t.Fatal("provider command runner override was not preserved")
	}
	if cfg.WorkerApplication.ScriptCommandRunner != scriptRunner {
		t.Fatal("script command runner override was not preserved")
	}
}

func TestConfigWithWorkerApplicationRejectsNilConfig(t *testing.T) {
	t.Parallel()

	if _, err := service.ConfigWithWorkerApplication(nil); err == nil {
		t.Fatal("ConfigWithWorkerApplication(nil) succeeded")
	}
}

func TestConfigWithWorkerApplicationKeepsPreconstructedApplicationWithoutOverrides(t *testing.T) {
	t.Parallel()

	components, err := workerapplication.New(zap.NewNop(), workerapplication.Edges{})
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}
	configured, err := service.ConfigWithWorkerApplication(&service.FactoryServiceConfig{WorkerApplication: components})
	if err != nil {
		t.Fatalf("ConfigWithWorkerApplication: %v", err)
	}
	if configured.WorkerApplication.Provider != components.Provider || configured.WorkerApplication.Script != components.Script {
		t.Fatal("ConfigWithWorkerApplication unexpectedly replaced the supplied worker factories")
	}
}

func TestConfigWithWorkerApplicationAppliesOverrideToPreconstructedApplication(t *testing.T) {
	t.Parallel()

	baseProviderRunner := &commandRunnerProbe{}
	baseScriptRunner := &commandRunnerProbe{}
	components, err := workerapplication.New(zap.NewNop(), workerapplication.Edges{
		ProviderCommandRunner: baseProviderRunner,
		ScriptCommandRunner:   baseScriptRunner,
	})
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}
	overrideProviderRunner := &commandRunnerProbe{}
	configured, err := service.ConfigWithWorkerApplication(&service.FactoryServiceConfig{
		WorkerApplication:             components,
		ProviderCommandRunnerOverride: overrideProviderRunner,
	})
	if err != nil {
		t.Fatalf("ConfigWithWorkerApplication: %v", err)
	}
	if configured.WorkerApplication.ProviderCommandRunner != overrideProviderRunner {
		t.Fatal("preconstructed worker application did not receive provider command runner override")
	}
	if configured.WorkerApplication.ScriptCommandRunner != baseScriptRunner {
		t.Fatal("preconstructed worker application unexpectedly replaced script command runner")
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

	composeCfg, err := service.ConfigWithWorkerApplication(&service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("ConfigWithWorkerApplication: %v", err)
	}
	root, err := service.ResolveFactoryServiceRoot(composeCfg)
	if err != nil {
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	load, err := service.LoadFactoryConfigForCompose(composeCfg, root)
	if err != nil {
		t.Fatalf("LoadFactoryConfigForCompose: %v", err)
	}
	clock := service.ServiceClockForCompose(composeCfg, load)
	collaborators, err := service.NewFactoryServiceCollaborators(
		composeCfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
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
		service.NewHostedWorkersConfig(composeCfg, root.BaseLogger, clock),
	)
	if err != nil {
		t.Fatalf("ComposeFactoryService: %v", err)
	}
	modelDeps, err := service.ModelServiceDependencies(shell)
	if err != nil {
		t.Fatalf("ModelServiceDependencies: %v", err)
	}
	models, err := modelsservice.NewService(modelDeps)
	if err != nil {
		t.Fatalf("modelsservice.NewService: %v", err)
	}
	composed := service.AttachModelServiceCollaborator(shell, models)
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
	cfg, err := service.ConfigWithWorkerApplication(&service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("ConfigWithWorkerApplication: %v", err)
	}

	built, err := service.BuildFactoryCore(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryCore: %v", err)
	}

	composeCfg, err := service.ConfigWithWorkerApplication(&service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("ConfigWithWorkerApplication: %v", err)
	}
	root, err := service.ResolveFactoryServiceRoot(composeCfg)
	if err != nil {
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	load, err := service.LoadFactoryConfigForCompose(composeCfg, root)
	if err != nil {
		t.Fatalf("LoadFactoryConfigForCompose: %v", err)
	}
	clock := service.ServiceClockForCompose(composeCfg, load)
	collaborators, err := service.NewFactoryServiceCollaborators(
		composeCfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
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
		service.NewHostedWorkersConfig(composeCfg, root.BaseLogger, clock),
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

	composeCfg, err := service.ConfigWithWorkerApplication(cfg)
	if err != nil {
		t.Fatalf("ConfigWithWorkerApplication: %v", err)
	}
	root, err := service.ResolveFactoryServiceRoot(composeCfg)
	if err != nil {
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	load, err := service.LoadFactoryConfigForCompose(composeCfg, root)
	if err != nil {
		t.Fatalf("LoadFactoryConfigForCompose: %v", err)
	}
	clock := service.ServiceClockForCompose(composeCfg, load)
	collaborators, err := service.NewFactoryServiceCollaborators(
		composeCfg,
		clock,
		root.BaseLogger,
		service.NewFactorySessionsRegistry(),
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
		service.NewHostedWorkersConfig(composeCfg, root.BaseLogger, clock),
	)
	if err != nil {
		t.Fatalf("ComposeFactoryService: %v", err)
	}
	modelDeps, err := service.ModelServiceDependencies(shell)
	if err != nil {
		t.Fatalf("ModelServiceDependencies: %v", err)
	}
	models, err := modelsservice.NewService(modelDeps)
	if err != nil {
		t.Fatalf("modelsservice.NewService: %v", err)
	}
	composed := service.AttachModelServiceCollaborator(shell, models)
	composed = service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Service: composed},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Service: composed}, composeCfg),
	)

	if built.ComposeCollaboratorSnapshot() != composed.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: built=%+v composed=%+v", built.ComposeCollaboratorSnapshot(), composed.ComposeCollaboratorSnapshot())
	}
}
