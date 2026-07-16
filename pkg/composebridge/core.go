package composebridge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

// Collaborators groups explicit composition collaborators.
type Collaborators struct {
	Sessions         *factorysessions.Registry
	LocalModels      LocalModelDomain
	RuntimeBuild     *runtimebuild.Service
	WorkersScheduler *workersservice.Service
}

// NewCollaboratorsWithLocalModels assembles the non-model startup collaborators
// around an already-validated model domain supplied by pkg/wire.
func NewCollaboratorsWithLocalModels(
	cfg *runtimehost.Config,
	clock factory.Clock,
	baseLogger *zap.Logger,
	sessions *factorysessions.Registry,
	localModels LocalModelDomain,
	hostedWorkers hostedworkers.Config,
) Collaborators {
	return Collaborators{
		Sessions:         sessions,
		LocalModels:      localModels,
		RuntimeBuild:     NewRuntimeBuildService(cfg, clock, baseLogger, &localModels, sessions),
		WorkersScheduler: NewWorkersScheduler(cfg, clock, baseLogger, hostedWorkers),
	}
}

// ComposeCore constructs a runtimehost.Core using explicit composition collaborators.
func ComposeCore(
	ctx context.Context,
	cfg *runtimehost.Config,
	root Root,
	collaborators Collaborators,
	load ConfigLoad,
	clock factory.Clock,
	hostedWorkers hostedworkers.Config,
) (*runtimehost.Core, error) {
	if collaborators.WorkersScheduler == nil {
		return nil, fmt.Errorf("compose runtime host core: worker sidecar owner is required")
	}
	if err := runtimehost.ValidateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	if err := ensurebackendScope(cfg, root.BaseLogger); err != nil {
		return nil, err
	}
	coreBuilt := false
	var runtimeBundle *factoryservice.Bundle
	defer func() {
		if !coreBuilt && runtimeBundle != nil {
			_ = CloseRuntimeBundleSinks(runtimeBundle.LogSink, runtimeBundle.MetricsSink)
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
	durableExecution, err := composeDurableExecution(cfg, root, clock)
	if err != nil {
		return nil, err
	}

	replaySideEffects, replayFactoryOpts, err := ReplayFactoryModeOptions(load.ReplayArtifact)
	if err != nil {
		return nil, err
	}
	defaultSessionID := factorysessions.DefaultSessionID
	defaultSessionSpec, err := collaborators.RuntimeBuild.BuildSpec(ctx, runtimebuild.SessionSpecInput{
		Dir:                                    cfg.Dir,
		FolderPath:                             root.FactoryRootDir,
		SessionID:                              defaultSessionID,
		ExecutionBaseDir:                       cfg.ExecutionBaseDir,
		LoadedFactoryCfg:                       load.LoadedFactoryCfg,
		RuntimeInstanceID:                      cfg.RuntimeInstanceID,
		SideEffects:                            replaySideEffects,
		AdditionalFactoryOpts:                  replayFactoryOpts,
		PreserveCompatibilityDefaultRecordPath: true,
	})
	if err != nil {
		return nil, err
	}
	runtimeBundleAny, err := collaborators.RuntimeBuild.Build(ctx, defaultSessionSpec)
	if err != nil {
		return nil, err
	}
	runtimeBundle = AsRuntimeBundle(runtimeBundleAny)
	if runtimeBundle == nil {
		return nil, fmt.Errorf("default runtime bundle is required")
	}
	defaultSession := factorysessions.NewLiveSession(
		defaultSessionID,
		runtimeBundle.Dir,
		runtimeBundle.FolderPath,
		runtimeBundle.RuntimeCfg.RuntimeBaseDir(),
		runtimehost.FactorySessionTargetRef{Kind: runtimehost.FactorySessionTargetKindDefault},
		NewStartupLiveSessionHandle(runtimeBundle, &defaultSessionSpec),
		true,
		filepath.Base(runtimeBundle.FolderPath),
	)
	factorysessions.BindResponseEventCompletion(defaultSession, runtimeBundle.EventHistory.AddGeneratedRecorder)
	collaborators.Sessions.Upsert(defaultSession, true)

	coreBuilt = true
	return runtimehost.NewCore(
		cfg,
		root.FactoryRootDir,
		root.BaseLogger,
		collaborators.Sessions,
		collaborators.RuntimeBuild,
		collaborators.WorkersScheduler,
		collaborators.LocalModels,
		hostedWorkers,
		clock,
		runtimeBundle,
		runtimeBundle.Logger,
		WireModelAssetPuller(cfg, collaborators.LocalModels),
		durableExecution,
	), nil
}

func durableProjectRoot(executionBaseDir, configuredDir, factoryRootDir string) string {
	for _, candidate := range []string{executionBaseDir, configuredDir, factoryRootDir} {
		if root := strings.TrimSpace(candidate); root != "" {
			return root
		}
	}
	return ""
}

func composeDurableExecution(
	cfg *runtimehost.Config,
	root Root,
	clock factory.Clock,
) (factorysessionexecution.Service, error) {
	projectRoot := durableProjectRoot(cfg.ExecutionBaseDir, cfg.Dir, root.FactoryRootDir)
	persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(
		cfg.DurableSessionPersistencePolicy,
		projectRoot,
	)
	if err != nil {
		return nil, fmt.Errorf("compose durable session persistence: %w", err)
	}
	configPath := strings.TrimSpace(cfg.SystemConfigPath)
	if configPath == "" {
		homeDir := strings.TrimSpace(cfg.SystemConfigHomeDir)
		if homeDir == "" {
			homeDir, err = os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("resolve operator config home: %w", err)
			}
		}
		configPath = defaultpaths.OperatorConfigPath(homeDir)
	}
	operatorConfig, err := operatorconfig.LoadFileConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("compose durable session worker presets: %w", err)
	}
	workerPresetIDs := make(map[string]struct{}, len(operatorConfig.WorkerPresets))
	workerPresets := make(map[string]workflowruntime.WorkerPreset, len(operatorConfig.WorkerPresets))
	for _, preset := range operatorConfig.WorkerPresets {
		workerPresetIDs[preset.ID] = struct{}{}
		workerPresets[preset.ID] = workflowruntime.WorkerPreset{ModelProvider: preset.ModelProvider, Model: preset.Model, ReasoningEffort: preset.ReasoningEffort}
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
			WorkerSettings:   workflowruntime.WorkerSettingsConfig{Presets: workerPresets, DefaultModelProvider: operatorConfig.Defaults.WorkerModelProvider, DefaultModel: operatorConfig.Defaults.WorkerModel},
		},
	)
}

// BuildCore constructs the normalized runtime graph without attaching a transport host.
func BuildCore(ctx context.Context, cfg *runtimehost.Config) (*runtimehost.Core, error) {
	return buildCore(ctx, cfg, nil)
}

// BuildCoreWithLocalModels constructs the normalized runtime graph with the
// validated model collaborators supplied by the application composition root.
func BuildCoreWithLocalModels(
	ctx context.Context,
	cfg *runtimehost.Config,
	localModels LocalModelDomain,
) (*runtimehost.Core, error) {
	return buildCore(ctx, cfg, &localModels)
}

func buildCore(
	ctx context.Context,
	cfg *runtimehost.Config,
	localModels *LocalModelDomain,
) (*runtimehost.Core, error) {
	if err := runtimehost.ValidateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	if !cfg.WorkerApplication.Valid() {
		return nil, fmt.Errorf("compose runtime worker application is required")
	}
	root, err := resolveRoot(cfg)
	if err != nil {
		return nil, err
	}
	if err := ensurebackendScope(cfg, root.BaseLogger); err != nil {
		return nil, err
	}
	load, err := loadConfig(cfg, root)
	if err != nil {
		return nil, err
	}
	clock := ClockForCompose(cfg, load)
	sessions := NewSessionsRegistry()
	hostedWorkers := cfg.WorkerApplication.Hosted
	var collaborators Collaborators
	if localModels == nil {
		startupLocalModels, modelErr := modelhost.NewLocalDomain(LocalModelDomainDependencies(cfg))
		if modelErr != nil {
			return nil, modelErr
		}
		collaborators = NewCollaboratorsWithLocalModels(
			cfg, clock, root.BaseLogger, sessions, startupLocalModels, hostedWorkers,
		)
	} else {
		collaborators = NewCollaboratorsWithLocalModels(
			cfg, clock, root.BaseLogger, sessions, *localModels, hostedWorkers,
		)
	}
	core, err := ComposeCore(
		ctx,
		cfg,
		root,
		collaborators,
		load,
		clock,
		hostedWorkers,
	)
	if err != nil {
		return nil, err
	}
	models, modelErr := composeModelService(core, cfg)
	if modelErr != nil {
		return nil, modelErr
	}
	runtimehost.AttachModelService(core, models)
	return core, nil
}

// composeModelService adapts the already-built runtime core to the canonical
// model-package constructor. Construction remains inert: it neither starts a
// runtime nor selects application lifecycle or process-mode policy.
func composeModelService(core *runtimehost.Core, cfg *runtimehost.Config) (apisurface.ModelAPI, error) {
	if cfg != nil && cfg.ModelAPI != nil {
		return cfg.ModelAPI, nil
	}
	if core == nil || core.Clock() == nil {
		return nil, fmt.Errorf("construct model service: runtime core and clock are required")
	}
	host := runtimehost.NewHostFromCore(core)
	var metrics modelsservice.PullMetricsRecorder
	if cfg != nil && cfg.ModelPullMetricsRecorder != nil {
		metrics = modelPullMetricsAdapter{inner: cfg.ModelPullMetricsRecorder}
	}
	models, err := modelsservice.NewService(modelsservice.Dependencies{
		RuntimeConfig:           host.CurrentModelRuntimeConfig,
		ModelHost:               core.ModelHost(),
		ModelAssetPuller:        core.ModelAssetPuller(),
		Logger:                  core.Logger(),
		Clock:                   core.Clock().Now,
		ModelPullMetrics:        metrics,
		ModelInvocationExecutor: host.BuildModelInvocationExecutor,
		FactoryRunnerID:         cfg.RunnerID,
	})
	if err != nil {
		return nil, fmt.Errorf("construct model service: %w", err)
	}
	return models, nil
}

type modelPullMetricsAdapter struct {
	inner runtimehost.ModelPullMetricsRecorder
}

func (a modelPullMetricsAdapter) RecordModelPullMetric(metric modelsservice.PullMetric) {
	a.inner.RecordModelPullMetric(runtimehost.InvocationMetric{Name: metric.Name, Labels: metric.Labels})
}
