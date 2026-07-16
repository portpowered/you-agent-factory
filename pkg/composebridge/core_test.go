package composebridge_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/composebridge"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	"go.uber.org/zap"
)

func composedRuntimeConfig(t *testing.T, cfg *runtimehost.Config) *runtimehost.Config {
	t.Helper()
	components, err := workerapplication.New(cfg.Logger, workerapplication.Edges{})
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}
	cfg.WorkerApplication = components
	return cfg
}

func TestCompatibilityFacadesShareFakeDurableExecution(t *testing.T) {
	t.Parallel()

	execution, err := factorysessionexecution.NewExecutionService(
		factorysessionexecution.ExecutionProviderFake,
		factorysessionexecution.ServiceConfig{FakeOptions: []factorysessionexecution.FakeServiceOption{
			factorysessionexecution.WithFakeScenarios(factorysessionexecution.BuiltinInterruptedRecoverableScenario()),
		}},
	)
	if err != nil {
		t.Fatalf("compose fake execution: %v", err)
	}
	core := runtimehost.NewCore(&runtimehost.Config{}, "", zap.NewNop(), nil, nil, nil,
		runtimehost.LocalModelDomain{}, hostedworkers.Config{}, nil, nil, zap.NewNop(), nil, execution, nil)
	host := runtimehost.NewHostFromCore(core)
	svc := service.NewFactoryServiceFromRuntimeHostCore(core)
	if host.DurableExecutionService() != execution || svc.DurableExecutionService() != execution {
		t.Fatal("compatibility facades did not receive the same core-owned execution collaborator")
	}

	workflowName := "recoverable-audit"
	started, err := host.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-interrupted-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		},
	})
	if err != nil {
		t.Fatalf("runtimehost start: %v", err)
	}
	read, err := svc.GetDurableFactorySession(context.Background(), started.SessionId)
	if err != nil {
		t.Fatalf("FactoryService read of runtimehost start: %v", err)
	}
	if read.SessionId != started.SessionId {
		t.Fatalf("FactoryService session id = %q, want %q", read.SessionId, started.SessionId)
	}
}

func TestBuildCore_RejectsRecordAndReplayTogether(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := composedRuntimeConfig(t, &runtimehost.Config{
		Dir:        t.TempDir(),
		RecordPath: "recording.json",
		ReplayPath: "recording.json",
		Logger:     zap.NewNop(),
	})

	core, err := buildCoreForTest(ctx, cfg)
	if core != nil {
		t.Fatal("expected BuildCore to return nil core for conflicting record/replay paths")
	}
	if err == nil {
		t.Fatal("expected BuildCore to fail for conflicting record/replay paths")
	}
}

func TestBuildCore_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := composedRuntimeConfig(t, &runtimehost.Config{
		Dir:    t.TempDir(),
		Logger: zap.NewNop(),
	})

	core, err := buildCoreForTest(ctx, cfg)
	if core != nil {
		t.Fatal("expected BuildCore to return nil core without factory.json")
	}
	if err == nil {
		t.Fatal("expected BuildCore to fail without factory.json")
	}
}

func TestBuildCore_ComposesCoreForValidFactoryConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	cfg := composedRuntimeConfig(t, &runtimehost.Config{
		Dir:                                     dir,
		SystemConfigPath:                        filepath.Join(t.TempDir(), "operator-config.json"),
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})

	core, err := buildCoreForTest(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildCore: %v", err)
	}
	if core == nil {
		t.Fatal("expected composed core")
	}
	if core.Sessions() == nil || core.RuntimeBuild() == nil || core.WorkersScheduler() == nil {
		t.Fatal("expected session, runtime build, and workers scheduler collaborators on composed core")
	}
	if core.DurableExecution() == nil {
		t.Fatal("expected durable execution collaborator on composed core")
	}
	host := runtimehost.NewHostFromCore(core)
	if host.DurableExecutionService() != core.DurableExecution() {
		t.Fatal("runtime host did not receive the core-owned durable execution collaborator")
	}
	if composebridge.NewModelServiceFromCore(core) == nil {
		t.Fatal("expected model service from composed core")
	}
	if composebridge.NewFactoryDefinitionServiceFromCore(core) == nil {
		t.Fatal("expected factory definition service from composed core")
	}
	if err := composebridge.CloseRuntimeBundleSinks(nil, nil); err != nil {
		t.Fatalf("CloseRuntimeBundleSinks(nil): %v", err)
	}
}

func TestBuildCore_ComposesExplicitlyDisabledPersistence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	core, err := buildCoreForTest(context.Background(), &runtimehost.Config{
		Dir:                                     dir,
		SystemConfigPath:                        filepath.Join(t.TempDir(), "operator-config.json"),
		Logger:                                  zap.NewNop(),
		DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildCore: %v", err)
	}
	if core.DurableExecution() == nil {
		t.Fatal("disabled persistence did not compose in-memory durable execution")
	}
	if _, err := os.Stat(filepath.Join(dir, ".you-agent-factory", "durable-sessions")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled persistence path stat error = %v, want not-exist", err)
	}
}

func TestBuildCore_RejectsUnavailablePersistenceLocation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	blockedRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedRoot, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocked root: %v", err)
	}
	core, err := buildCoreForTest(context.Background(), &runtimehost.Config{
		Dir:                                     dir,
		ExecutionBaseDir:                        blockedRoot,
		SystemConfigPath:                        filepath.Join(t.TempDir(), "operator-config.json"),
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if core != nil {
		t.Fatal("BuildCore returned a core for unavailable persistence")
	}
	var validation *factorysessionexecution.ValidationError
	if !errors.As(err, &validation) || validation.Field != "persistence" {
		t.Fatalf("BuildCore error = %#v, want wrapped persistence ValidationError", err)
	}
}

func buildCoreForTest(ctx context.Context, cfg *runtimehost.Config) (*runtimehost.Core, error) {
	if !cfg.WorkerApplication.Valid() {
		components, err := workerapplication.New(cfg.Logger, workerapplication.Edges{})
		if err != nil {
			return nil, err
		}
		cfg.WorkerApplication = components
	}
	root, err := service.ResolveFactoryServiceRoot(service.FactoryServiceConfigFromRuntimeHost(cfg))
	if err != nil {
		return nil, err
	}
	if err := service.EnsureBackendScopeForCompose(cfg, root.BaseLogger); err != nil {
		return nil, err
	}
	load, err := service.LoadFactoryConfigForStartup(cfg, root)
	if err != nil {
		return nil, err
	}
	clock := composebridge.ClockForCompose(cfg, load)
	sessions := factorysessions.NewRegistry()
	localModels, err := composebridge.NewLocalModelDomain(cfg)
	if err != nil {
		return nil, err
	}
	runtimeBuild, err := composebridge.NewRuntimeBuildService(cfg, clock, root.BaseLogger, &localModels, sessions)
	if err != nil {
		return nil, err
	}
	projectRoot := strings.TrimSpace(cfg.ExecutionBaseDir)
	if projectRoot == "" {
		projectRoot = strings.TrimSpace(cfg.Dir)
	}
	if projectRoot == "" {
		projectRoot = strings.TrimSpace(root.FactoryRootDir)
	}
	persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(cfg.DurableSessionPersistencePolicy, projectRoot)
	if err != nil {
		return nil, err
	}
	durableExecution, err := factorysessionexecution.NewExecutionService(
		factorysessionexecution.ExecutionProviderJavaScriptRuntime,
		factorysessionexecution.ServiceConfig{
			ProjectRoot: projectRoot, Provider: cfg.ProviderOverride,
			ProviderExecutor: providerexecution.NewExecutor(cfg.ProviderOverride),
			Persistence:      persistence, Clock: clock,
		},
	)
	if err != nil {
		return nil, err
	}
	recorder := durableExecution.(interface {
		RecordPetriTokenMutations(string, []interfaces.TokenMutationRecord) error
	})
	runtimeBuild, err = runtimeBuild.WithPetriMutationRecorder(recorder.RecordPetriTokenMutations)
	if err != nil {
		return nil, err
	}
	hostedWorkers := cfg.WorkerApplication.Hosted
	return composebridge.ComposeCore(ctx, cfg, root, composebridge.Collaborators{
		Sessions: sessions, LocalModels: localModels, RuntimeBuild: runtimeBuild,
		WorkersScheduler: composebridge.NewWorkersScheduler(cfg, clock, root.BaseLogger, hostedWorkers),
		DurableExecution: durableExecution, Persistence: persistence,
	}, load, clock, hostedWorkers)
}
