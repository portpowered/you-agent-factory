// Package composebridge exposes narrow runtime-build adapters consumed by the
// pkg/wire application graph without creating a runtimehost/service import cycle.
package composebridge

import (
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
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

// ensurebackendScope resolves backend scope before core composition.
func ensurebackendScope(cfg *runtimehost.Config, logger *zap.Logger) error {
	return service.EnsureBackendScopeForCompose(cfg, logger)
}

// ClockForCompose selects the factory clock for the loaded replay artifact.
func ClockForCompose(cfg *runtimehost.Config, load ConfigLoad) factory.Clock {
	return service.ClockForCompose(cfg, load)
}

// NewLocalModelDomain constructs the local-model collaborator group for a build.
func NewLocalModelDomain(cfg *runtimehost.Config) (LocalModelDomain, error) {
	return modelhost.NewLocalDomain(service.LocalModelDomainDependencies(service.FactoryServiceConfigFromRuntimeHost(cfg)))
}

// NewRuntimeBuildService constructs the runtimebuild collaborator for core composition.
func NewRuntimeBuildService(
	cfg *runtimehost.Config,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels *LocalModelDomain,
	sessions *factorysessions.Registry,
) (*runtimebuild.Service, error) {
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

// NewModelServiceFromCore constructs a model service from a composed core.
func NewModelServiceFromCore(core *runtimehost.Core) ModelService {
	return service.NewModelServiceFromCore(core)
}

// NewFactoryDefinitionServiceFromCore constructs a factory definition service from a core.
func NewFactoryDefinitionServiceFromCore(core *runtimehost.Core) FactoryDefinitionService {
	return service.NewFactoryDefinitionServiceFromCore(core)
}
