package runtimehost

import (
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

// ComposeCollaboratorSnapshot records whether collaborators were initialized on a
// built Core or Host. Tests compare snapshots across wire and direct build paths.
type ComposeCollaboratorSnapshot struct {
	SessionsInitialized         bool
	RuntimeBuildInitialized     bool
	WorkersSchedulerInitialized bool
	LocalModelsInitialized      bool
	ModelAssetsInitialized      bool
	ModelServiceInitialized     bool
	FactorySaveInitialized      bool
	DefinitionsInitialized      bool
	HostedWorkersLoggerReady    bool
	BundleModelResources        bool
	BundleLocalModels           bool
}

// LocalModelDomain wires pkg/localmodels runtime dependencies constructed at
// build time and copied onto each factory runtime bundle.
type LocalModelDomain = factoryservice.LocalModelDomain

// Core owns the normalized runtime graph assembled before transport facades or
// runtime loops begin.
type Core struct {
	cfg              *Config
	factoryRootDir   string
	baseLogger       *zap.Logger
	sessions         *factorysessions.Registry
	runtimeBuild     *runtimebuild.Service
	workersScheduler *workersservice.Service
	localModels      LocalModelDomain
	hostedWorkers    hostedworkers.Config
	clock            factory.Clock
	startupBundle    *factoryRuntimeBundle
	logger           *zap.Logger
	modelAssets      modelAssetPuller
	durableExecution factorysessionexecution.Service
}

// ServiceConfig returns the normalized service config used to compose the core.
func (core *Core) ServiceConfig() *Config {
	if core == nil {
		return nil
	}
	return core.cfg
}

// FactoryRootDir returns the canonical factory root selected at build time.
func (core *Core) FactoryRootDir() string {
	if core == nil {
		return ""
	}
	return core.factoryRootDir
}

// BaseLogger returns the base logger used for runtime graph assembly.
func (core *Core) BaseLogger() *zap.Logger {
	if core == nil {
		return nil
	}
	return core.baseLogger
}

// Logger returns the startup runtime logger.
func (core *Core) Logger() *zap.Logger {
	if core == nil {
		return nil
	}
	return core.logger
}

// Clock returns the normalized service clock.
func (core *Core) Clock() factory.Clock {
	if core == nil {
		return nil
	}
	return core.clock
}

// Sessions returns the live session registry collaborator owned by the core.
func (core *Core) Sessions() *factorysessions.Registry {
	if core == nil {
		return nil
	}
	return core.sessions
}

// RuntimeBuild returns the runtime-build collaborator owned by the core.
func (core *Core) RuntimeBuild() *runtimebuild.Service {
	if core == nil {
		return nil
	}
	return core.runtimeBuild
}

// WorkersScheduler returns the workers scheduling collaborator owned by the core.
func (core *Core) WorkersScheduler() *workersservice.Service {
	if core == nil {
		return nil
	}
	return core.workersScheduler
}

// ModelHost returns the process-wide model host collaborator.
func (core *Core) ModelHost() modelhost.Host {
	if core == nil {
		return nil
	}
	return core.localModels.Host
}

// LocalModels returns the startup local-model collaborator group.
func (core *Core) LocalModels() LocalModelDomain {
	if core == nil {
		return LocalModelDomain{}
	}
	return core.localModels
}

// HostedWorkers returns the hosted-worker config built for this runtime graph.
func (core *Core) HostedWorkers() hostedworkers.Config {
	if core == nil {
		return hostedworkers.Config{}
	}
	return core.hostedWorkers
}

// StartupBundle returns the pre-run runtime bundle built during composition.
func (core *Core) StartupBundle() *factoryRuntimeBundle {
	if core == nil {
		return nil
	}
	return core.startupBundle
}

// ModelAssetPuller returns the explicit model asset collaborator used by direct
// model operations.
func (core *Core) ModelAssetPuller() localmodels.AssetPuller {
	if core == nil {
		return nil
	}
	return core.modelAssets
}

// DurableExecution returns the single durable execution collaborator owned by
// this composed application graph.
func (core *Core) DurableExecution() factorysessionexecution.Service {
	if core == nil {
		return nil
	}
	return core.durableExecution
}

// ComposeCollaboratorSnapshot reports initialized core collaborators for
// equivalence tests.
func (core *Core) ComposeCollaboratorSnapshot() ComposeCollaboratorSnapshot {
	if core == nil {
		return ComposeCollaboratorSnapshot{}
	}
	bundle := core.StartupBundle()
	snapshot := ComposeCollaboratorSnapshot{
		SessionsInitialized:         core.Sessions() != nil,
		RuntimeBuildInitialized:     core.RuntimeBuild() != nil,
		WorkersSchedulerInitialized: core.WorkersScheduler() != nil,
		LocalModelsInitialized:      core.LocalModels().Manager != nil,
		ModelAssetsInitialized:      core.ModelAssetPuller() != nil,
		DefinitionsInitialized:      true,
		HostedWorkersLoggerReady:    core.HostedWorkers().Logger != nil,
	}
	if bundle != nil {
		snapshot.BundleModelResources = bundle.ModelResources != nil
		snapshot.BundleLocalModels = bundle.LocalModels != nil
	}
	return snapshot
}

// NewCore constructs a composed runtime graph core from explicit collaborators.
func NewCore(
	cfg *Config,
	factoryRootDir string,
	baseLogger *zap.Logger,
	sessions *factorysessions.Registry,
	runtimeBuild *runtimebuild.Service,
	workersScheduler *workersservice.Service,
	localModels LocalModelDomain,
	hostedWorkers hostedworkers.Config,
	clock factory.Clock,
	startupBundle *factoryRuntimeBundle,
	logger *zap.Logger,
	modelAssets modelAssetPuller,
	durableExecution factorysessionexecution.Service,
) *Core {
	return &Core{
		cfg:              cfg,
		factoryRootDir:   factoryRootDir,
		baseLogger:       baseLogger,
		sessions:         sessions,
		runtimeBuild:     runtimeBuild,
		workersScheduler: workersScheduler,
		localModels:      localModels,
		hostedWorkers:    hostedWorkers,
		clock:            clock,
		startupBundle:    startupBundle,
		logger:           logger,
		modelAssets:      modelAssets,
		durableExecution: durableExecution,
	}
}
