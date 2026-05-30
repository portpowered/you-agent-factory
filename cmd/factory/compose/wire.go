//go:build wireinject

package compose

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
		provideLocalModelDomain,
		provideFactoryConfigLoad,
		provideServiceClock,
		provideRuntimeBuildService,
		provideFactoryServiceCollaborators,
		provideHostedWorkersConfig,
		provideFactoryService,
	)
	return nil, nil
}
