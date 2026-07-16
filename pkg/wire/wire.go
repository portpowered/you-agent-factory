//go:build wireinject

package wire

import (
	"context"

	"github.com/google/wire"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
)

// InjectRuntimeCore constructs the single Factory Session core consumed by the
// application graph before initializer lifecycle execution.
func InjectRuntimeCore(ctx context.Context, cfg *runtimehost.Config) (*runtimehost.Core, error) {
	wire.Build(
		provideRuntimeHostRoot,
		provideRuntimeHostBaseLogger,
		provideFactorySessionsRegistry,
		provideRuntimeHostConfigLoad,
		provideRuntimeHostClock,
		provideRuntimeHostLocalModels,
		provideRuntimeHostRuntimeBuild,
		provideRuntimeHostPersistence,
		provideRuntimeHostDurableExecution,
		provideRuntimeHostRecordingBuild,
		provideRuntimeHostHostedWorkers,
		provideRuntimeHostWorkers,
		provideRuntimeHostCollaborators,
		provideRuntimeHostCore,
		provideRuntimeModelServiceDependencies,
		provideRuntimeModelService,
		provideRuntimeHostCoreWithModels,
	)
	return nil, nil
}

// InjectFactoryService is the wireinject entry for the factory composition root.
func InjectFactoryService(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (*service.FactoryService, error) {
	wire.Build(
		provideRuntimeHostConfigFromFactoryService,
		provideRuntimeHostRoot,
		provideRuntimeHostBaseLogger,
		provideFactorySessionsRegistry,
		provideRuntimeHostConfigLoad,
		provideRuntimeHostClock,
		provideRuntimeHostLocalModels,
		provideRuntimeHostRuntimeBuild,
		provideRuntimeHostPersistence,
		provideRuntimeHostDurableExecution,
		provideRuntimeHostRecordingBuild,
		provideRuntimeHostHostedWorkers,
		provideRuntimeHostWorkers,
		provideRuntimeHostCollaborators,
		provideRuntimeHostCore,
		provideRuntimeModelServiceDependencies,
		provideRuntimeModelService,
		provideRuntimeHostCoreWithModels,
		provideFactoryServiceFromRuntimeHostCore,
	)
	return nil, nil
}
