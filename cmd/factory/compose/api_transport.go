package compose

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
)

// InjectAPITransport composes API handler dependencies through pkg/initializer
// without using the wire FactoryService composition root.
func InjectAPITransport(ctx context.Context, cfg *service.FactoryServiceConfig) (*initializer.APITransport, error) {
	return initializer.InitializeAPITransport(ctx, (*initializer.Config)(cfg))
}
