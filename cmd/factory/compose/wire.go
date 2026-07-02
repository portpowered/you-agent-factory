//go:build wireinject

package compose

import (
	"context"

	"github.com/google/wire"
	"github.com/portpowered/infinite-you/pkg/service"
)

// InjectFactoryService is the legacy wireinject entry retained for compose
// equivalence tests. New transport composition must use InjectAPITransport,
// InjectCLITransport, or InjectMCPTransport from pkg/initializer instead.
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
		provideWorkersSchedulerService,
		provideFactoryServiceCollaborators,
		provideHostedWorkersConfig,
		provideFactoryCore,
		provideFactoryService,
	)
	return nil, nil
}
