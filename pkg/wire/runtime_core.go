package wire

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	replay "github.com/portpowered/infinite-you/pkg/factory/replay"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

// runtimeCoreCollaborators is the explicit set of stateful collaborators built
// once for a production application graph.
type runtimeCoreCollaborators struct {
	sessions         *factorysessions.Registry
	localModels      service.LocalModelDomain
	runtimeBuild     *runtimebuild.Service
	workersScheduler *workersservice.Service
}

func buildRuntimeCore(ctx context.Context, cfg *runtimehost.Config) (*runtimehost.Core, error) {
	if err := runtimehost.ValidateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("compose runtime config is required")
	}
	serviceCfg, err := service.ConfigWithWorkerApplication(runtimeConfigAsServiceConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("compose runtime worker application: %w", err)
	}
	cfg = serviceConfigAsRuntimeConfig(serviceCfg)
	root, err := service.ResolveFactoryServiceRoot(serviceCfg)
	if err != nil {
		return nil, err
	}
	if err := ensureRuntimeBackendScope(cfg, root.BaseLogger); err != nil {
		return nil, err
	}
	load, err := service.LoadFactoryConfigForCompose(serviceCfg, root)
	if err != nil {
		return nil, err
	}
	clock := service.ServiceClockForCompose(serviceCfg, load)
	hostedWorkers := cfg.WorkerApplication.Hosted
	collaborators, err := newRuntimeCoreCollaborators(serviceCfg, clock, root.BaseLogger, hostedWorkers)
	if err != nil {
		return nil, err
	}
	core, err := composeRuntimeCore(ctx, cfg, root, collaborators, load, clock, hostedWorkers)
	if err != nil {
		return nil, err
	}
	host := runtimehost.NewHostFromCore(core)
	models, err := composeModelService(core, host, cfg)
	if err != nil {
		return nil, fmt.Errorf("compose runtime model service: %w", err)
	}
	return runtimehost.AttachModelService(core, models), nil
}

func newRuntimeCoreCollaborators(
	cfg *service.FactoryServiceConfig,
	clock factory.Clock,
	logger *zap.Logger,
	hostedWorkers hostedworkers.Config,
) (runtimeCoreCollaborators, error) {
	sessions := factorysessions.NewRegistry()
	localModels, err := modelhost.NewLocalDomain(service.LocalModelDomainDependencies(cfg))
	if err != nil {
		return runtimeCoreCollaborators{}, err
	}
	runtimeBuild, err := service.NewRuntimeBuildServiceWithObservers(
		cfg,
		clock,
		logger,
		&localModels,
		runtimehost.NewInferenceProgressPublisherFactory(sessions, logger),
		runtimehost.NewSessionDispatchCompletionObserverFactory(sessions),
	)
	if err != nil {
		return runtimeCoreCollaborators{}, err
	}
	return runtimeCoreCollaborators{
		sessions:         sessions,
		localModels:      localModels,
		runtimeBuild:     runtimeBuild,
		workersScheduler: provideWorkersSchedulerService(cfg, clock, logger, hostedWorkers),
	}, nil
}

func composeRuntimeCore(
	ctx context.Context,
	cfg *runtimehost.Config,
	root service.FactoryServiceRoot,
	collaborators runtimeCoreCollaborators,
	load service.FactoryConfigLoadResult,
	clock factory.Clock,
	hostedWorkers hostedworkers.Config,
) (*runtimehost.Core, error) {
	if collaborators.workersScheduler == nil {
		return nil, fmt.Errorf("compose runtime core: worker sidecar owner is required")
	}
	coreBuilt := false
	var runtimeBundle *factoryservice.Bundle
	defer func() {
		if !coreBuilt && runtimeBundle != nil {
			_ = factoryservice.CloseBundleSinks(runtimeBundle.LogSink, runtimeBundle.MetricsSink)
		}
	}()
	if cfg.ReplayPath == "" {
		resolvedDir, err := factoryconfig.ResolveCurrentFactoryDir(cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("resolve factory dir: %w", err)
		}
		resolvedDir, err = factorysessions.AbsolutizeFactoryDirectory(resolvedDir)
		if err != nil {
			return nil, fmt.Errorf("resolve factory dir: %w", err)
		}
		cfg.Dir = resolvedDir
	}
	durableExecution, err := composeRuntimeDurableExecution(cfg, root, clock)
	if err != nil {
		return nil, err
	}
	recorder, ok := durableExecution.(interface {
		RecordPetriTokenMutations(string, []interfaces.TokenMutationRecord) error
	})
	if !ok {
		return nil, fmt.Errorf("compose runtime core: durable execution owner does not record Petri mutations")
	}
	collaborators.runtimeBuild, err = collaborators.runtimeBuild.WithPetriMutationRecorder(recorder.RecordPetriTokenMutations)
	if err != nil {
		return nil, fmt.Errorf("compose runtime core: %w", err)
	}
	persistenceOwner, ok := durableExecution.(interface{ PersistenceStore() runtimepersist.Store })
	if !ok {
		return nil, fmt.Errorf("compose runtime core: durable execution owner does not expose persistence")
	}

	runtimeBundle, defaultSessionSpec, err := buildDefaultRuntimeBundle(
		ctx, cfg, root, collaborators.runtimeBuild, load,
	)
	if err != nil {
		return nil, err
	}
	defaultSession := factorysessions.NewLiveSession(
		factorysessions.DefaultSessionID,
		runtimeBundle.Dir,
		runtimeBundle.FolderPath,
		runtimeBundle.RuntimeCfg.RuntimeBaseDir(),
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		runtimehost.NewLiveSessionState(runtimeBundle, &defaultSessionSpec),
		true,
		filepath.Base(runtimeBundle.FolderPath),
	)
	factorysessions.BindResponseEventCompletion(defaultSession, runtimeBundle.EventHistory.AddEventTypeRecorder)
	collaborators.sessions.Upsert(defaultSession, true)

	modelAssets := collaborators.localModels.Assets
	if cfg.ModelAssets != nil {
		modelAssets = cfg.ModelAssets
	}
	coreBuilt = true
	return runtimehost.NewCore(
		cfg,
		root.FactoryRootDir,
		root.BaseLogger,
		collaborators.sessions,
		collaborators.runtimeBuild,
		collaborators.workersScheduler,
		collaborators.localModels,
		hostedWorkers,
		clock,
		runtimeBundle,
		runtimeBundle.Logger,
		modelAssets,
		durableExecution,
		persistenceOwner.PersistenceStore(),
	), nil
}

func buildDefaultRuntimeBundle(
	ctx context.Context,
	cfg *runtimehost.Config,
	root service.FactoryServiceRoot,
	builder *runtimebuild.Service,
	load service.FactoryConfigLoadResult,
) (*factoryservice.Bundle, runtimebuild.SessionBuildSpec, error) {
	replaySideEffects, replayFactoryOpts, err := runtimeReplayFactoryModeOptions(load.ReplayArtifact)
	if err != nil {
		return nil, runtimebuild.SessionBuildSpec{}, err
	}
	spec, err := builder.BuildSpec(ctx, runtimebuild.SessionSpecInput{
		Dir:                                    cfg.Dir,
		FolderPath:                             root.FactoryRootDir,
		SessionID:                              factorysessions.DefaultSessionID,
		ExecutionBaseDir:                       cfg.ExecutionBaseDir,
		LoadedFactoryCfg:                       load.LoadedFactoryCfg,
		RuntimeInstanceID:                      cfg.RuntimeInstanceID,
		SideEffects:                            replaySideEffects,
		AdditionalFactoryOpts:                  replayFactoryOpts,
		PreserveCompatibilityDefaultRecordPath: true,
	})
	if err != nil {
		return nil, runtimebuild.SessionBuildSpec{}, err
	}
	bundle, err := builder.Build(ctx, spec)
	if err != nil {
		return nil, runtimebuild.SessionBuildSpec{}, err
	}
	runtimeBundle, ok := bundle.(*factoryservice.Bundle)
	if !ok || runtimeBundle == nil {
		return nil, runtimebuild.SessionBuildSpec{}, fmt.Errorf("default runtime bundle is required")
	}
	return runtimeBundle, spec, nil
}

func composeRuntimeDurableExecution(
	cfg *runtimehost.Config,
	root service.FactoryServiceRoot,
	clock factory.Clock,
) (factorysessionexecution.Service, error) {
	projectRoot := firstNonEmpty(cfg.ExecutionBaseDir, cfg.Dir, root.FactoryRootDir)
	persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(cfg.DurableSessionPersistencePolicy, projectRoot)
	if err != nil {
		return nil, fmt.Errorf("compose durable session persistence: %w", err)
	}
	configPath, err := runtimeOperatorConfigPath(cfg)
	if err != nil {
		return nil, err
	}
	operatorConfig, err := operatorconfig.LoadFileConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("compose durable session worker presets: %w", err)
	}
	workerPresetIDs := make(map[string]struct{}, len(operatorConfig.WorkerPresets))
	workerPresets := make(map[string]workflowruntime.WorkerPreset, len(operatorConfig.WorkerPresets))
	for _, preset := range operatorConfig.WorkerPresets {
		workerPresetIDs[preset.ID] = struct{}{}
		workerPresets[preset.ID] = workflowruntime.WorkerPreset{
			ModelProvider:   preset.ModelProvider,
			Model:           preset.Model,
			ReasoningEffort: preset.ReasoningEffort,
		}
	}
	return factorysessionexecution.NewExecutionService(
		factorysessionexecution.ExecutionProviderJavaScriptRuntime,
		factorysessionexecution.ServiceConfig{
			ProjectRoot:      projectRoot,
			Provider:         cfg.ProviderOverride,
			ProviderExecutor: providerexecution.NewExecutor(cfg.ProviderOverride),
			Persistence:      persistence,
			Clock:            clock,
			WorkerPresetIDs:  workerPresetIDs,
			WorkerSettings: workflowruntime.WorkerSettingsConfig{
				Presets:              workerPresets,
				DefaultModelProvider: operatorConfig.Defaults.WorkerModelProvider,
				DefaultModel:         operatorConfig.Defaults.WorkerModel,
			},
		},
	)
}

func runtimeReplayFactoryModeOptions(
	replayArtifact *interfaces.ReplayArtifact,
) (*replay.SideEffects, []factory.FactoryOption, error) {
	if replayArtifact == nil {
		return nil, nil, nil
	}
	sideEffects, err := replay.NewSideEffects(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay side effects: %w", err)
	}
	submissionHook, err := replay.NewSubmissionHook(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay submission hook: %w", err)
	}
	workStateChangeHook, err := replay.NewWorkStateChangeHook(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay work state change hook: %w", err)
	}
	deliveryPlan, err := replay.NewCompletionDeliveryPlan(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay completion delivery plan: %w", err)
	}
	return sideEffects, []factory.FactoryOption{
		factory.WithSubmissionHook(submissionHook),
		factory.WithSubmissionHook(workStateChangeHook),
		factory.WithCompletionDeliveryPlanner(deliveryPlan),
	}, nil
}

func ensureRuntimeBackendScope(cfg *runtimehost.Config, logger *zap.Logger) error {
	if cfg == nil {
		return fmt.Errorf("runtime config is required to resolve backend scope")
	}
	if strings.TrimSpace(cfg.ReplayPath) != "" || strings.TrimSpace(cfg.BackendScopeID) != "" {
		return nil
	}
	configPath, err := runtimeOperatorConfigPath(cfg)
	if err != nil {
		return err
	}
	resolved, err := systemconfig.EnsureLocalBackendScope(configPath)
	if err != nil {
		return err
	}
	cfg.BackendScopeID = resolved.BackendScopeID
	if logger != nil {
		logger.Info("resolved backend scope for local backend", zap.String("diagnostics", resolved.DiagnosticsLine()))
	}
	return nil
}

func runtimeOperatorConfigPath(cfg *runtimehost.Config) (string, error) {
	if cfg != nil && strings.TrimSpace(cfg.SystemConfigPath) != "" {
		return strings.TrimSpace(cfg.SystemConfigPath), nil
	}
	homeDir := ""
	if cfg != nil {
		homeDir = strings.TrimSpace(cfg.SystemConfigHomeDir)
	}
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve operator config home: %w", err)
		}
	}
	return defaultpaths.OperatorConfigPath(homeDir), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// These two migration mappings live at the composition root. The structs are
// kept layout-compatible until the remaining runtimehost facade is deleted.
func runtimeConfigAsServiceConfig(cfg *runtimehost.Config) *service.FactoryServiceConfig {
	if cfg == nil {
		return nil
	}
	return (*service.FactoryServiceConfig)(unsafe.Pointer(cfg))
}

func serviceConfigAsRuntimeConfig(cfg *service.FactoryServiceConfig) *runtimehost.Config {
	if cfg == nil {
		return nil
	}
	return (*runtimehost.Config)(unsafe.Pointer(cfg))
}
