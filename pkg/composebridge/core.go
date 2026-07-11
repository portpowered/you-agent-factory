package composebridge

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
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

// NewCollaborators builds composition collaborators for startup.
func NewCollaborators(
	cfg *runtimehost.Config,
	clock factory.Clock,
	baseLogger *zap.Logger,
	sessions *factorysessions.Registry,
) Collaborators {
	startupLocalModels := NewLocalModelDomain(cfg)
	hostedWorkers := HostedWorkers(cfg, baseLogger, clock)
	return Collaborators{
		Sessions:         sessions,
		LocalModels:      startupLocalModels,
		RuntimeBuild:     NewRuntimeBuildService(cfg, clock, baseLogger, &startupLocalModels, sessions),
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
	if err := EnsureBackendScope(cfg, root.BaseLogger); err != nil {
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
	factorysessions.EnsureRuntimeFactorySessionID(defaultSession)
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
		factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
			ProjectRoot:     durableProjectRoot(cfg.ExecutionBaseDir, cfg.Dir, root.FactoryRootDir),
			Provider:        cfg.ProviderOverride,
			PersistSessions: true,
			Clock:           clock,
		}),
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

// BuildCore constructs the normalized runtime graph without attaching a transport host.
func BuildCore(ctx context.Context, cfg *runtimehost.Config) (*runtimehost.Core, error) {
	if err := runtimehost.ValidateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	root, err := ResolveRoot(cfg)
	if err != nil {
		return nil, err
	}
	if err := EnsureBackendScope(cfg, root.BaseLogger); err != nil {
		return nil, err
	}
	load, err := LoadConfig(cfg, root)
	if err != nil {
		return nil, err
	}
	clock := ClockForCompose(cfg, load)
	collaborators := NewCollaborators(cfg, clock, root.BaseLogger, NewSessionsRegistry())
	return ComposeCore(
		ctx,
		cfg,
		root,
		collaborators,
		load,
		clock,
		HostedWorkers(cfg, root.BaseLogger, clock),
	)
}
