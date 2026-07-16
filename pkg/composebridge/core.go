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
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

// Collaborators groups explicit composition collaborators.
type Collaborators struct {
	Sessions         *factorysessions.Registry
	LocalModels      LocalModelDomain
	RuntimeBuild     *runtimebuild.Service
	WorkersScheduler *workersservice.Service
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
	if err := validateCoreCollaborators(collaborators); err != nil {
		return nil, err
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
	durableExecution, persistence, runtimeBuild, err := composeSessionRuntimeDependencies(cfg, root, clock, collaborators.RuntimeBuild)
	if err != nil {
		return nil, err
	}
	collaborators.RuntimeBuild = runtimeBuild

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
		persistence,
	), nil
}

func validateCoreCollaborators(collaborators Collaborators) error {
	switch {
	case collaborators.Sessions == nil:
		return fmt.Errorf("compose runtime host core: Factory Session registry is required")
	case collaborators.RuntimeBuild == nil:
		return fmt.Errorf("compose runtime host core: runtime build service is required")
	case collaborators.WorkersScheduler == nil:
		return fmt.Errorf("compose runtime host core: worker sidecar owner is required")
	default:
		return nil
	}
}

func composeSessionRuntimeDependencies(
	cfg *runtimehost.Config,
	root Root,
	clock factory.Clock,
	build *runtimebuild.Service,
) (factorysessionexecution.Service, runtimepersist.Store, *runtimebuild.Service, error) {
	execution, persistence, err := composeDurableExecution(cfg, root, clock)
	if err != nil {
		return nil, nil, nil, err
	}
	configured, err := composePetriRecordingRuntimeBuild(build, execution)
	if err != nil {
		return nil, nil, nil, err
	}
	return execution, persistence, configured, nil
}

func composePetriRecordingRuntimeBuild(
	build *runtimebuild.Service,
	execution factorysessionexecution.Service,
) (*runtimebuild.Service, error) {
	recorder, ok := execution.(interface {
		RecordPetriTokenMutations(string, []interfaces.TokenMutationRecord) error
	})
	if !ok {
		return nil, fmt.Errorf("compose runtime host core: durable execution owner does not record Petri mutations")
	}
	configured, err := build.WithPetriMutationRecorder(recorder.RecordPetriTokenMutations)
	if err != nil {
		return nil, fmt.Errorf("compose runtime host core: %w", err)
	}
	return configured, nil
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
) (factorysessionexecution.Service, runtimepersist.Store, error) {
	projectRoot := durableProjectRoot(cfg.ExecutionBaseDir, cfg.Dir, root.FactoryRootDir)
	persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(
		cfg.DurableSessionPersistencePolicy,
		projectRoot,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("compose durable session persistence: %w", err)
	}
	configPath := strings.TrimSpace(cfg.SystemConfigPath)
	if configPath == "" {
		homeDir := strings.TrimSpace(cfg.SystemConfigHomeDir)
		if homeDir == "" {
			homeDir, err = os.UserHomeDir()
			if err != nil {
				return nil, nil, fmt.Errorf("resolve operator config home: %w", err)
			}
		}
		configPath = defaultpaths.OperatorConfigPath(homeDir)
	}
	operatorConfig, err := operatorconfig.LoadFileConfig(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("compose durable session worker presets: %w", err)
	}
	workerPresetIDs := make(map[string]struct{}, len(operatorConfig.WorkerPresets))
	workerPresets := make(map[string]workflowruntime.WorkerPreset, len(operatorConfig.WorkerPresets))
	for _, preset := range operatorConfig.WorkerPresets {
		workerPresetIDs[preset.ID] = struct{}{}
		workerPresets[preset.ID] = workflowruntime.WorkerPreset{ModelProvider: preset.ModelProvider, Model: preset.Model, ReasoningEffort: preset.ReasoningEffort}
	}
	service, err := factorysessionexecution.NewExecutionService(
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
	if err != nil {
		return nil, nil, err
	}
	return service, persistence.Store(), nil
}
