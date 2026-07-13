// Package wire constructs the concrete application graph consumed by the
// process root. It does not select process modes or own component lifecycle.
package wire

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
)

// BuildCLIRunner constructs the local runtime selected by the process root.
// Dashboard-enabled runs use the initializer-owned transport graph; remaining
// compatibility paths retain the established service-backed runner.
func BuildCLIRunner(ctx context.Context, cfg *service.FactoryServiceConfig) (initializer.LocalRuntimeRunner, error) {
	if cfg == nil || cfg.SimpleDashboardRenderer == nil {
		return service.BuildFactoryService(ctx, cfg)
	}
	transport, err := initializer.InitializeCLITransport(ctx, service.RuntimeHostConfigFromFactoryService(cfg))
	if err != nil {
		return nil, err
	}
	return transport.Runner(), nil
}
