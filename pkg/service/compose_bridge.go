// Package service exposes compose-bridge helpers for pkg/initializer startup
// composition. Runtime bundle construction internals remain here until a
// dedicated runtime-bundle package split; transports must not call these.
package service

import (
	"unsafe"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"go.uber.org/zap"
)

// FactoryServiceConfigFromRuntimeHost maps initializer compose config onto the
// service composition config. The structs are kept layout-compatible during the
// runtimehost migration.
func FactoryServiceConfigFromRuntimeHost(cfg *runtimehost.Config) *FactoryServiceConfig {
	if cfg == nil {
		return nil
	}
	return (*FactoryServiceConfig)(unsafe.Pointer(cfg))
}

// EnsureBackendScopeForCompose resolves backend scope before core composition.
func EnsureBackendScopeForCompose(cfg *runtimehost.Config, logger *zap.Logger) error {
	return ensureServiceBackendScope(FactoryServiceConfigFromRuntimeHost(cfg), logger)
}

// NewRuntimeBuildServiceForCompose constructs the runtimebuild collaborator for
// initializer-owned core composition.
func NewRuntimeBuildServiceForCompose(
	cfg *runtimehost.Config,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels *LocalModelDomain,
	sessions *factorysessions.Registry,
) *runtimebuild.Service {
	return newRuntimeBuildService(
		FactoryServiceConfigFromRuntimeHost(cfg),
		clock,
		baseLogger,
		localModels,
		inferenceProgressPublisherFactory(
			runtimehost.NewInferenceProgressPublisherFactory(sessions, baseLogger),
		),
		dispatchCompletionObserverFactory(
			runtimehost.NewSessionDispatchCompletionObserverFactory(sessions),
		),
	)
}

// LoadFactoryConfigForStartup loads factory.json and replay metadata for compose.
func LoadFactoryConfigForStartup(
	cfg *runtimehost.Config,
	root FactoryServiceRoot,
) (FactoryConfigLoadResult, error) {
	return LoadFactoryConfigForCompose(FactoryServiceConfigFromRuntimeHost(cfg), root)
}

// ClockForCompose selects the factory clock for the loaded replay artifact.
func ClockForCompose(cfg *runtimehost.Config, load FactoryConfigLoadResult) factory.Clock {
	return ServiceClockForCompose(FactoryServiceConfigFromRuntimeHost(cfg), load)
}

// HostedWorkersForCompose builds the hosted-workers collaborator from config.
func HostedWorkersForCompose(
	cfg *runtimehost.Config,
	logger *zap.Logger,
	clock factory.Clock,
) hostedworkers.Config {
	return NewHostedWorkersConfig(FactoryServiceConfigFromRuntimeHost(cfg), logger, clock)
}

// CloseRuntimeBundleSinksForCompose closes startup bundle sinks when compose fails.
func CloseRuntimeBundleSinksForCompose(logSink *logging.RuntimeLogSink, metricsSink *logging.RuntimeMetricsSink) error {
	return closeRuntimeBundleSinks(logSink, metricsSink)
}

// ReplayFactoryModeOptionsForCompose builds replay side-effect options for compose.
func ReplayFactoryModeOptionsForCompose(
	replayArtifact *interfaces.ReplayArtifact,
) (*replay.SideEffects, []factory.FactoryOption, error) {
	return replayFactoryModeOptions(replayArtifact)
}

// AsRuntimeBundleForCompose converts a runtime-build product into the startup bundle.
func AsRuntimeBundleForCompose(bundle any) *factoryRuntimeBundle {
	return asRuntimeBundle(bundle)
}

// NewStartupLiveSessionHandle constructs the default session handle during startup.
func NewStartupLiveSessionHandle(bundle *factoryRuntimeBundle, spec *runtimebuild.SessionBuildSpec) any {
	state := &liveSessionState{bundle: bundle, spec: spec}
	if bundle != nil {
		state.handle = &liveRuntimeHandle{Bundle: bundle}
	}
	return state
}

// WireModelAssetPullerForCompose selects the model asset puller for compose.
func WireModelAssetPullerForCompose(cfg *runtimehost.Config, production modelAssetPuller) modelAssetPuller {
	return wireModelAssetPuller(FactoryServiceConfigFromRuntimeHost(cfg), production)
}

// ModelService is the compose-facing model API collaborator.
type ModelService = apisurface.ModelAPI

// NewModelServiceFromCore constructs a model service from a composed runtime core.
func NewModelServiceFromCore(core *runtimehost.Core) ModelService {
	if core == nil {
		return wireModelServiceCollaborator(nil, nil)
	}
	shell := FactoryServiceShell{Service: NewFactoryServiceFromCore(adaptRuntimeHostCore(core))}
	return ProvideModelServiceCollaborator(shell, FactoryServiceConfigFromRuntimeHost(core.ServiceConfig()))
}

// NewFactoryDefinitionServiceFromCore constructs a factory definition service from a core.
func NewFactoryDefinitionServiceFromCore(core *runtimehost.Core) FactoryDefinitionService {
	if core == nil {
		return factoryDefinitionHost{}
	}
	return factoryDefinitionHost{FactoryService: NewFactoryServiceFromCore(adaptRuntimeHostCore(core))}
}

// StartupWorkerConfigFromCore returns the named worker from the composed startup runtime.
func StartupWorkerConfigFromCore(core *runtimehost.Core, name string) (*interfaces.WorkerConfig, bool) {
	if core == nil {
		return nil, false
	}
	runtimeCfg := coreStartupRuntimeConfigFromRuntimeHost(core)
	if runtimeCfg == nil {
		return nil, false
	}
	return runtimeCfg.Worker(name)
}

func adaptRuntimeHostCore(core *runtimehost.Core) *FactoryCore {
	if core == nil {
		return nil
	}
	return &FactoryCore{
		cfg:           FactoryServiceConfigFromRuntimeHost(core.ServiceConfig()),
		root:          FactoryServiceRoot{FactoryRootDir: core.FactoryRootDir(), BaseLogger: core.BaseLogger()},
		collaborators: factoryServiceCollaboratorsFromRuntimeHost(core),
		hostedWorkers: core.HostedWorkers(),
		clock:         core.Clock(),
		startupBundle: asRuntimeBundle(core.StartupBundle()),
		logger:        core.Logger(),
		modelAssets:   core.ModelAssetPuller(),
	}
}

func factoryServiceCollaboratorsFromRuntimeHost(core *runtimehost.Core) FactoryServiceCollaborators {
	if core == nil {
		return FactoryServiceCollaborators{}
	}
	return FactoryServiceCollaborators{
		Sessions:         core.Sessions(),
		LocalModels:        core.LocalModels(),
		RuntimeBuild:       core.RuntimeBuild(),
		WorkersScheduler:   core.WorkersScheduler(),
	}
}

func coreStartupRuntimeConfigFromRuntimeHost(core *runtimehost.Core) *factoryconfig.LoadedFactoryConfig {
	if core == nil {
		return nil
	}
	if bundle := core.StartupBundle(); bundle != nil {
		return bundle.RuntimeCfg
	}
	return nil
}
