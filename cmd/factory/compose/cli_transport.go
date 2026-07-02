package compose

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
)

// InjectCLITransport composes local CLI runtime dependencies through
// pkg/initializer without using the wire FactoryService composition root.
func InjectCLITransport(ctx context.Context, cfg *service.FactoryServiceConfig) (*initializer.CLITransport, error) {
	return initializer.InitializeCLITransport(ctx, (*initializer.Config)(cfg))
}
