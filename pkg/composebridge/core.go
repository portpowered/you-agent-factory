package composebridge

import (
	"context"
	"fmt"
	"path/filepath"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

// Collaborators groups explicit composition collaborators.
type Collaborators struct {
	Sessions         *factorysessions.Registry
	LocalModels      LocalModelDomain
	RuntimeBuild     *runtimebuild.Service
	WorkersScheduler *workersservice.Service
	DurableExecution factorysessionexecution.Service
	Persistence      factorysessionexecution.PersistenceChoice
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
		collaborators.DurableExecution,
		collaborators.Persistence.Store(),
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
	case collaborators.DurableExecution == nil:
		return fmt.Errorf("compose runtime host core: durable execution service is required")
	default:
		if err := collaborators.Persistence.Validate(); err != nil {
			return fmt.Errorf("compose runtime host core: %w", err)
		}
		return nil
	}
}
