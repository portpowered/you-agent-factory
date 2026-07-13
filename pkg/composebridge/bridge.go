// Package composebridge exposes runtime-build seams for pkg/initializer startup
// composition without creating an import cycle between initializer and service.
package composebridge

import (
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

type (
	Root                     = service.FactoryServiceRoot
	ConfigLoad               = service.FactoryConfigLoadResult
	LocalModelDomain         = service.LocalModelDomain
	ModelService             = apisurface.ModelAPI
	FactoryDefinitionService = service.FactoryDefinitionService
)

// ResolveRoot absolutizes cfg.Dir, assigns cfg.Logger, and mints cfg.RuntimeInstanceID when empty.
func ResolveRoot(cfg *runtimehost.Config) (Root, error) {
	return service.ResolveFactoryServiceRoot(service.FactoryServiceConfigFromRuntimeHost(cfg))
}

// EnsureBackendScope resolves backend scope before core composition.
func EnsureBackendScope(cfg *runtimehost.Config, logger *zap.Logger) error {
	return service.EnsureBackendScopeForCompose(cfg, logger)
}

// LoadConfig loads factory.json and replay metadata for compose.
func LoadConfig(cfg *runtimehost.Config, root Root) (ConfigLoad, error) {
	return service.LoadFactoryConfigForStartup(cfg, root)
}

// ClockForCompose selects the factory clock for the loaded replay artifact.
func ClockForCompose(cfg *runtimehost.Config, load ConfigLoad) factory.Clock {
	return service.ClockForCompose(cfg, load)
}

// HostedWorkers builds the hosted-workers collaborator from config.
func HostedWorkers(cfg *runtimehost.Config, logger *zap.Logger, clock factory.Clock) hostedworkers.Config {
	return service.HostedWorkersForCompose(cfg, logger, clock)
}

// NewLocalModelDomain constructs the local-model collaborator group for a build.
func NewLocalModelDomain(cfg *runtimehost.Config) LocalModelDomain {
	return service.NewLocalModelDomain(service.FactoryServiceConfigFromRuntimeHost(cfg))
}

// NewRuntimeBuildService constructs the runtimebuild collaborator for core composition.
func NewRuntimeBuildService(
	cfg *runtimehost.Config,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels *LocalModelDomain,
	sessions *factorysessions.Registry,
) *runtimebuild.Service {
	return service.NewRuntimeBuildServiceForCompose(cfg, clock, baseLogger, localModels, sessions)
}

// NewWorkersScheduler constructs the workers scheduler collaborator.
func NewWorkersScheduler(
	cfg *runtimehost.Config,
	clock factory.Clock,
	baseLogger *zap.Logger,
	hostedWorkers hostedworkers.Config,
) *workersservice.Service {
	return service.NewWorkersSchedulerService(
		service.FactoryServiceConfigFromRuntimeHost(cfg),
		clock,
		baseLogger,
		hostedWorkers,
	)
}

// CloseRuntimeBundleSinks closes startup bundle sinks when composition fails.
func CloseRuntimeBundleSinks(logSink *logging.RuntimeLogSink, metricsSink *logging.RuntimeMetricsSink) error {
	return service.CloseRuntimeBundleSinksForCompose(logSink, metricsSink)
}

// ReplayFactoryModeOptions builds replay side-effect options for core composition.
func ReplayFactoryModeOptions(
	replayArtifact *interfaces.ReplayArtifact,
) (*replay.SideEffects, []factory.FactoryOption, error) {
	return service.ReplayFactoryModeOptionsForCompose(replayArtifact)
}

// AsRuntimeBundle converts a runtime-build product into the startup bundle.
func AsRuntimeBundle(bundle any) *factoryservice.Bundle {
	return service.AsRuntimeBundleForCompose(bundle)
}

// NewStartupLiveSessionHandle constructs the default session handle during startup.
func NewStartupLiveSessionHandle(bundle *factoryservice.Bundle, spec *runtimebuild.SessionBuildSpec) any {
	return service.NewStartupLiveSessionHandle(bundle, spec)
}

// WireModelAssetPuller selects the model asset puller for core composition.
func WireModelAssetPuller(cfg *runtimehost.Config, production LocalModelDomain) localmodels.AssetPuller {
	return service.WireModelAssetPullerForCompose(cfg, production.Assets)
}

// NewSessionsRegistry constructs the live session registry collaborator.
func NewSessionsRegistry() *factorysessions.Registry {
	return service.NewFactorySessionsRegistry()
}

// NewModelServiceFromCore constructs a model service from a composed core.
func NewModelServiceFromCore(core *runtimehost.Core) ModelService {
	return service.NewModelServiceFromCore(core)
}

// NewFactoryDefinitionServiceFromCore constructs a factory definition service from a core.
func NewFactoryDefinitionServiceFromCore(core *runtimehost.Core) FactoryDefinitionService {
	return service.NewFactoryDefinitionServiceFromCore(core)
}

// StartupWorkerConfigFromCore returns the named worker from the composed startup runtime.
func StartupWorkerConfigFromCore(core *runtimehost.Core, name string) (*interfaces.WorkerConfig, bool) {
	return service.StartupWorkerConfigFromCore(core, name)
}
