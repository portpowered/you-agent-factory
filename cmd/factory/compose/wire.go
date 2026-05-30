//go:build wireinject

package compose

import (
	"context"

	"github.com/google/wire"
	"github.com/portpowered/infinite-you/pkg/service"
)

// InjectFactoryService is the wireinject entry for the factory composition root.
// Bootstrap provider set delegates to service.BuildFactoryService; later stories
// replace this with explicit S6 collaborator providers.
func InjectFactoryService(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (*service.FactoryService, error) {
	wire.Build(
		service.BuildFactoryService,
	)
	return nil, nil
}
