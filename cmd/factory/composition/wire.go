//go:build wireinject
// +build wireinject

package composition

import (
	"context"

	"github.com/google/wire"
	"github.com/portpowered/infinite-you/pkg/service"
)

//go:generate go run -mod=mod github.com/google/wire/cmd/wire

// BuildFactoryService is the Wire injector for the factory CLI composition root.
func BuildFactoryService(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (*service.FactoryService, error) {
	wire.Build(
		service.ProvideFactorySessionsRegistry,
		service.ProvideStartupLocalModelDomainPtr,
		service.ProvideFactoryServiceBuildContext,
		service.ProvideRuntimeBuildService,
		service.ProvideFactoryServiceCollaborators,
		service.ProvideFactoryRuntimeBundle,
		service.ProvideHostedWorkersConfig,
		service.ProvideFactoryServiceShell,
		service.ProvideFactorySaveCollaborator,
		service.AttachFactorySaveCollaborator,
	)
	return nil, nil
}
