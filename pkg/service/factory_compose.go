package service

import (
	"context"
	"fmt"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"go.uber.org/zap"
)

// FactoryServiceRoot holds the absolutized factory directory and base logger
// after FactoryServiceConfig normalization during service construction.
type FactoryServiceRoot struct {
	FactoryRootDir string
	BaseLogger     *zap.Logger
}

// ResolveFactoryServiceRoot absolutizes cfg.Dir, assigns cfg.Logger, and mints
// cfg.RuntimeInstanceID when empty.
func ResolveFactoryServiceRoot(cfg *FactoryServiceConfig) (FactoryServiceRoot, error) {
	factoryRootDir, baseLogger, err := resolveFactoryServiceRoot(cfg)
	if err != nil {
		return FactoryServiceRoot{}, err
	}
	return FactoryServiceRoot{
		FactoryRootDir: factoryRootDir,
		BaseLogger:     baseLogger,
	}, nil
}

// NewFactorySessionsRegistry constructs the live session registry collaborator.
func NewFactorySessionsRegistry() *factorysessions.Registry {
	return factorysessions.NewRegistry()
}

// LocalModelDomain wires pkg/localmodels runtime dependencies constructed at
// service build time and copied onto each factoryRuntimeBundle.
type LocalModelDomain = localModelDomain

// NewLocalModelDomain constructs the local-model collaborator group for a build.
func NewLocalModelDomain(cfg *FactoryServiceConfig) LocalModelDomain {
	return newRuntimeLocalModelDependencies(cfg)
}

// FactoryServiceCollaborators groups explicit S6 composition collaborators.
type FactoryServiceCollaborators struct {
	Sessions     *factorysessions.Registry
	LocalModels  LocalModelDomain
	RuntimeBuild *runtimebuild.Service
}

// NewFactoryServiceCollaborators builds S6 collaborators using the provided
// session registry and freshly constructed local-model dependencies.
func NewFactoryServiceCollaborators(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	sessions *factorysessions.Registry,
) FactoryServiceCollaborators {
	startupLocalModels := newRuntimeLocalModelDependencies(cfg)
	return FactoryServiceCollaborators{
		Sessions:     sessions,
		LocalModels:  startupLocalModels,
		RuntimeBuild: newRuntimeBuildService(cfg, clock, baseLogger, &startupLocalModels),
	}
}

// NewFactoryServiceCollaboratorsFromParts assembles collaborators from explicit
// wire-provided parts.
func NewFactoryServiceCollaboratorsFromParts(
	sessions *factorysessions.Registry,
	localModels LocalModelDomain,
	runtimeBuild *runtimebuild.Service,
) FactoryServiceCollaborators {
	return FactoryServiceCollaborators{
		Sessions:     sessions,
		LocalModels:  localModels,
		RuntimeBuild: runtimeBuild,
	}
}

// NewRuntimeBuildService constructs the runtimebuild collaborator for wire.
func NewRuntimeBuildService(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels *LocalModelDomain,
) *runtimebuild.Service {
	return newRuntimeBuildService(cfg, clock, baseLogger, localModels)
}

// FactoryConfigLoadResult carries factory config load outputs needed before
// runtime bundle construction.
type FactoryConfigLoadResult struct {
	LoadedFactoryCfg *factoryconfig.LoadedFactoryConfig
	ReplayArtifact   *interfaces.ReplayArtifact
	SessionLogger    *zap.Logger
}

// LoadFactoryConfigForCompose loads factory.json and replay metadata for wire
// composition after FactoryServiceRoot resolution.
func LoadFactoryConfigForCompose(
	cfg *FactoryServiceConfig,
	root FactoryServiceRoot,
) (FactoryConfigLoadResult, error) {
	logger := runtimebuild.NewSessionLogger(
		root.BaseLogger,
		defaultFactorySessionID,
		root.FactoryRootDir,
		cfg.Dir,
	)
	loadedFactoryCfg, replayArtifact, err := loadFactoryConfigForService(cfg, logger)
	if err != nil {
		return FactoryConfigLoadResult{}, err
	}
	return FactoryConfigLoadResult{
		LoadedFactoryCfg: loadedFactoryCfg,
		ReplayArtifact:   replayArtifact,
		SessionLogger:    logger,
	}, nil
}

// ServiceClockForCompose selects the factory clock for the loaded replay artifact.
func ServiceClockForCompose(cfg *FactoryServiceConfig, load FactoryConfigLoadResult) factory.Clock {
	return serviceClockForMode(cfg.Clock, load.ReplayArtifact)
}

// NewHostedWorkersConfig builds the hosted-workers collaborator from service config.
func NewHostedWorkersConfig(
	cfg *FactoryServiceConfig,
	logger *zap.Logger,
	clock factory.Clock,
) hostedworkers.Config {
	return buildHostedWorkersConfig(cfg, logger, clock)
}

// NewFactorySaveCollaborator wires the factorysave collaborator for a built service.
func NewFactorySaveCollaborator(fs *FactoryService, cfg *FactoryServiceConfig) factorySaveSaver {
	return wireFactorySaveCollaborator(fs, cfg)
}

// ComposeCollaboratorSnapshot records whether S6 collaborators were initialized
// on a built FactoryService. Tests compare snapshots across wire and direct build paths.
type ComposeCollaboratorSnapshot struct {
	SessionsInitialized      bool
	RuntimeBuildInitialized  bool
	ModelAssetsInitialized   bool
	FactorySaveInitialized   bool
	HostedWorkersLoggerReady bool
	BundleModelResources     bool
	BundleLocalModels        bool
}

// ComposeCollaboratorSnapshot reports initialized S6 collaborators for equivalence tests.
func (fs *FactoryService) ComposeCollaboratorSnapshot() ComposeCollaboratorSnapshot {
	if fs == nil {
		return ComposeCollaboratorSnapshot{}
	}
	bundle := fs.currentRuntimeBundle()
	snapshot := ComposeCollaboratorSnapshot{
		SessionsInitialized:      fs.sessions != nil,
		RuntimeBuildInitialized:  fs.runtimeBuild != nil,
		ModelAssetsInitialized:   fs.modelAssets != nil,
		FactorySaveInitialized:   fs.factorySave != nil,
		HostedWorkersLoggerReady: fs.hostedWorkers.Logger != nil,
	}
	if bundle != nil {
		snapshot.BundleModelResources = bundle.modelResources != nil
		snapshot.BundleLocalModels = bundle.localModels != nil
	}
	return snapshot
}

// ComposeFactoryService constructs *FactoryService using explicit S6 collaborators.
func ComposeFactoryService(
	ctx context.Context,
	cfg *FactoryServiceConfig,
	root FactoryServiceRoot,
	collaborators FactoryServiceCollaborators,
	load FactoryConfigLoadResult,
	clock factory.Clock,
) (*FactoryService, error) {
	if err := validateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	serviceBuilt := false
	var runtimeBundle *factoryRuntimeBundle
	defer func() {
		if !serviceBuilt && runtimeBundle != nil && runtimeBundle.logSink != nil {
			_ = runtimeBundle.logSink.Close()
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

	replaySideEffects, replayFactoryOpts, err := replayFactoryModeOptions(load.ReplayArtifact)
	if err != nil {
		return nil, err
	}
	runtimeBundleAny, err := collaborators.RuntimeBuild.BuildFromLoadedConfig(ctx, runtimebuild.BuildInput{
		Dir:                   cfg.Dir,
		FolderPath:            root.FactoryRootDir,
		SessionID:             defaultFactorySessionID,
		LoadedFactoryCfg:      load.LoadedFactoryCfg,
		BaseLogger:            root.BaseLogger,
		RuntimeInstanceID:     cfg.RuntimeInstanceID,
		Clock:                 clock,
		RecordPath:            runtimebuild.SessionScopedRecordPath(cfg.RecordPath, defaultFactorySessionID),
		WorkflowID:            cfg.WorkflowID,
		ProviderOverride:      providerOverrideForMode(cfg, replaySideEffects),
		ProviderCommandRunner: providerCommandRunnerForMode(cfg, load.LoadedFactoryCfg),
		CommandRunnerOverride: commandRunnerOverrideForMode(cfg, load.LoadedFactoryCfg, replaySideEffects),
		AdditionalFactoryOpts: replayFactoryOpts,
	})
	if err != nil {
		return nil, err
	}
	runtimeBundle = asRuntimeBundle(runtimeBundleAny)

	serviceBuilt = true
	fs := &FactoryService{
		factoryRootDir: root.FactoryRootDir,
		sessions:       collaborators.Sessions,
		hostedWorkers:  buildHostedWorkersConfig(cfg, runtimeBundle.logger, clock),
		startupBundle:  runtimeBundle,
		cfg:            cfg,
		modelAssets:    wireModelAssetPuller(cfg, collaborators.LocalModels.assets),
		baseLogger:     root.BaseLogger,
		logger:         runtimeBundle.logger,
		clock:          clock,
		runtimeBuild:   collaborators.RuntimeBuild,
	}
	fs.factorySave = wireFactorySaveCollaborator(fs, cfg)
	return fs, nil
}
