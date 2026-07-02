package compose

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/initializer"
)

// InjectAPITransport composes API handler dependencies through pkg/initializer
// without using the wire FactoryService composition root.
func InjectAPITransport(ctx context.Context, cfg *initializer.Config) (*initializer.APITransport, error) {
	return initializer.InitializeAPITransport(ctx, cfg)
}
