package compose

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"go.uber.org/zap"
)

func provideFactoryServiceRoot(cfg *service.FactoryServiceConfig) (service.FactoryServiceRoot, error) {
	return service.ResolveFactoryServiceRoot(cfg)
}

func provideBaseLogger(root service.FactoryServiceRoot) *zap.Logger {
	return root.BaseLogger
}

func provideFactorySessionsRegistry() *factorysessions.Registry {
	return service.NewFactorySessionsRegistry()
}

func provideLocalModelDomain(cfg *service.FactoryServiceConfig) service.LocalModelDomain {
	return service.NewLocalModelDomain(cfg)
}

func provideFactoryConfigLoad(
	cfg *service.FactoryServiceConfig,
	root service.FactoryServiceRoot,
) (service.FactoryConfigLoadResult, error) {
	return service.LoadFactoryConfigForCompose(cfg, root)
}

func provideServiceClock(
	cfg *service.FactoryServiceConfig,
	load service.FactoryConfigLoadResult,
) factory.Clock {
	return service.ServiceClockForCompose(cfg, load)
}

func provideRuntimeBuildService(
	cfg *service.FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels service.LocalModelDomain,
) *runtimebuild.Service {
	domain := localModels
	return service.NewRuntimeBuildService(cfg, clock, baseLogger, &domain)
}

func provideFactoryServiceCollaborators(
	sessions *factorysessions.Registry,
	localModels service.LocalModelDomain,
	runtimeBuild *runtimebuild.Service,
) service.FactoryServiceCollaborators {
	return service.NewFactoryServiceCollaboratorsFromParts(sessions, localModels, runtimeBuild)
}

// provideHostedWorkersConfig documents the hosted-workers collaborator seam for
// wire; ComposeFactoryService applies the same buildHostedWorkersConfig path
// using the runtime bundle logger after the bundle is built.
func provideHostedWorkersConfig(
	cfg *service.FactoryServiceConfig,
	logger *zap.Logger,
	clock factory.Clock,
) hostedworkers.Config {
	return service.NewHostedWorkersConfig(cfg, logger, clock)
}

func provideFactoryService(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
	root service.FactoryServiceRoot,
	collaborators service.FactoryServiceCollaborators,
	load service.FactoryConfigLoadResult,
	clock factory.Clock,
	_ hostedworkers.Config,
) (*service.FactoryService, error) {
	return service.ComposeFactoryService(ctx, cfg, root, collaborators, load, clock)
}
