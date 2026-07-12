package compose

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
)

// InjectCLITransport composes local CLI runtime dependencies through
// pkg/initializer without using the wire FactoryService composition root.
func InjectCLITransport(ctx context.Context, cfg *initializer.Config) (*initializer.CLITransport, error) {
	return initializer.InitializeCLITransport(ctx, cfg)
}

// InjectCLIRunner transfers dashboard-enabled process composition to the
// initializer. Dashboard-suppressed non-invocation paths retain their existing
// service compatibility runner while that broader migration remains separate.
func InjectCLIRunner(ctx context.Context, cfg *service.FactoryServiceConfig) (initializer.LocalRuntimeRunner, error) {
	if cfg == nil || cfg.SimpleDashboardRenderer == nil {
		return service.BuildFactoryService(ctx, cfg)
	}
	transport, err := initializer.InitializeCLITransport(ctx, service.RuntimeHostConfigFromFactoryService(cfg))
	if err != nil {
		return nil, err
	}
	return transport.Runner(), nil
}
