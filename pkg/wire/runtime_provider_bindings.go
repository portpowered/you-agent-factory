package wire

import (
	"context"
	"fmt"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
)

// newConfiguredProvidersService always installs the shell-free Antigravity
// print-mode command effect. An injected serviceedges.Edges.AgyPTYHost exists
// only to satisfy the legacy PTY allocator construction port and must never
// suppress the canonical command adapter; the command effect unconditionally
// takes priority over the legacy PTY effect in executionwire's built-in
// dependency selection.
func newConfiguredProvidersService(
	options []providerswire.Option,
	agyRunner workers.CommandRunner,
) (providers.Service, error) {
	options = append(options, providerswire.WithAgyCommandRunner(agyRunner))
	return providerswire.NewService(options...)
}

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
