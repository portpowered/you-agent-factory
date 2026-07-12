package compose

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
)

// InjectCLITransport composes local CLI runtime dependencies through
// pkg/initializer without using the wire FactoryService composition root.
func InjectCLITransport(ctx context.Context, cfg *initializer.Config) (*initializer.CLITransport, error) {
	return initializer.InitializeCLITransport(ctx, cfg)
}

// InjectCLIRunner transfers dashboard-enabled process composition to the
// initializer. Dashboard-suppressed invocation paths use the shared in-process
// one-shot invocation bootstrap while that broader migration remains separate.
func InjectCLIRunner(ctx context.Context, cfg *service.FactoryServiceConfig) (initializer.LocalRuntimeRunner, error) {
	if cfg == nil {
		return nil, fmt.Errorf("inject CLI runner: config is required")
	}
	if cfg.SimpleDashboardRenderer != nil {
		transport, err := initializer.InitializeCLITransport(ctx, service.RuntimeHostConfigFromFactoryService(cfg))
		if err != nil {
			return nil, err
		}
		return transport.Runner(), nil
	}
	bootstrap, err := service.BuildInvocationBootstrap(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return bootstrap, nil
}
