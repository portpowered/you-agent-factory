package internal

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
)

// ProviderRegistryRebinder reconstructs the provider registry so migrated
// Codex and Claude integrations execute through the same runtime command edge
// as the Workers provider factories.
type ProviderRegistryRebinder func(workers.CommandRunner) (workers.ProviderRegistry, error)

func rebindProviderRegistry(
	current workers.ProviderRegistry,
	runner workers.CommandRunner,
	rebind ProviderRegistryRebinder,
) (workers.ProviderRegistry, error) {
	if current == nil || rebind == nil || runner == nil {
		return current, nil
	}
	rebound, err := rebind(runner)
	if err != nil {
		return nil, fmt.Errorf("rebind provider registry for runtime command runner: %w", err)
	}
	return rebound, nil
}

func applyReboundProviderRegistry(service *Service, registry workers.ProviderRegistry) error {
	if service == nil || registry == nil {
		return nil
	}
	service.providerRegistry = registry
	assembly, err := newRuntimeAssembly(registry)
	if err != nil {
		return err
	}
	service.Root = service.Root.ReplaceRuntimeAssembly(assembly)
	if builder, ok := service.executorBuilder.(*workerconstruction.Service); ok {
		service.executorBuilder = builder.
			WithRunnerSelection(registry.ResolveRunnerSelection).
			WithProviderIdentityResolution(registry.CanonicalIdentity).
			WithProviderRegistry(registry)
	}
	return nil
}
