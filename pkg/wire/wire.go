//go:build wireinject

package wire

import (
	"context"

	"github.com/google/wire"
	"github.com/portpowered/infinite-you/pkg/service"
)

// InjectFactoryService is the wireinject entry for the factory composition root.
func InjectFactoryService(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (*service.FactoryService, error) {
	wire.Build(
		provideFactoryServiceRoot,
		provideBaseLogger,
		provideFactorySessionsRegistry,
		modelProviderSet,
		provideFactoryConfigLoad,
		provideServiceClock,
		provideRuntimeBuildService,
		provideWorkersSchedulerService,
		provideFactoryServiceCollaborators,
		provideHostedWorkersConfig,
		provideFactoryCore,
		provideFactoryService,
	)
	return nil, nil
}
