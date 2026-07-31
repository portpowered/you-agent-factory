package internal

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
)

// ProviderRegistryRebinder reconstructs the provider registry so migrated
// Codex and Claude integrations execute through the same runtime command edge
// as the Workers provider factories.
type ProviderRegistryRebinder func(workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error)

func rebindProviderRegistry(
	current workers.ProviderRegistry,
	runner workers.CommandRunner,
	rebind ProviderRegistryRebinder,
) (workers.ProviderRegistry, providers.Service, error) {
	if current == nil || rebind == nil || runner == nil {
		return current, nil, nil
	}
	rebound, providerService, err := rebind(runner)
	if err != nil {
		return nil, nil, fmt.Errorf("rebind provider registry for runtime command runner: %w", err)
	}
	return rebound, providerService, nil
}

func applyReboundProviderRegistry(service *Service, registry workers.ProviderRegistry, providerService providers.Service) error {
	if service == nil || registry == nil {
		return nil
	}
	service.providerRegistry = registry
	if providerService != nil {
		service.providers = providerService
	}
	assembly, err := newRuntimeAssembly(registry)
	if err != nil {
		return err
	}
	service.Root = service.Root.ReplaceRuntimeAssembly(assembly)
	if builder, ok := service.executorBuilder.(*workerconstruction.Service); ok {
		service.executorBuilder = builder.
			WithExecutionFactories(providerService, nil).
			WithRunnerSelection(registry.ResolveRunnerSelection).
			WithProviderIdentityResolution(registry.CanonicalIdentity).
			WithProviderRegistry(registry)
	}
	return nil
}
