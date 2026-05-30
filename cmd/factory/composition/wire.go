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
		service.BuildFactoryService,
	)
	return nil, nil
}
