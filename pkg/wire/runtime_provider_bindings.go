package wire

import (
	"context"
	"fmt"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
)

// provideRuntimeProviderBindings builds graph-worker provider bindings over the
// Factory Runtime's effective command runner, including mock and replay wrappers.
func provideRuntimeProviderBindings(
	edges serviceedges.Edges,
	integrations []operatorsettings.ACPIntegration,
	runner workers.CommandRunner,
) (workers.ProviderRegistry, providers.Service, workerswire.ProviderRegistryRebinder, error) {
	runtimeProviders, err := provideConfiguredProvidersService(edges, integrations, runner)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("construct runtime Providers service: %w", err)
	}
	runtimeRegistry, err := workerswire.NewProviderRegistry(context.Background(), runtimeProviders)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("construct runtime provider registry: %w", err)
	}
	rebinder := workerswire.ProviderRegistryRebinder(func(reboundRunner workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		reboundProviders, rebuildErr := provideConfiguredProvidersService(edges, integrations, reboundRunner)
		if rebuildErr != nil {
			return nil, nil, rebuildErr
		}
		reboundRegistry, registryErr := workerswire.NewProviderRegistry(context.Background(), reboundProviders)
		return reboundRegistry, reboundProviders, registryErr
	})
	return runtimeRegistry, runtimeProviders, rebinder, nil
}
