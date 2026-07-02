package compose

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/initializer"
)

// InjectCLITransport composes local CLI runtime dependencies through
// pkg/initializer without using the wire FactoryService composition root.
func InjectCLITransport(ctx context.Context, cfg *initializer.Config) (*initializer.CLITransport, error) {
	return initializer.InitializeCLITransport(ctx, cfg)
}
