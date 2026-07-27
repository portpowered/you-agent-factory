package service

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/construction"
	providerconductor "github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

// ProviderRegistryRebinder reconstructs the provider registry so migrated
// Codex and Claude integrations execute through the same runtime command edge
// as the Workers provider factories.
type ProviderRegistryRebinder func(workers.CommandRunner) (*providerregistry.Registry, error)

func rebindProviderRegistry(
	current *providerregistry.Registry,
	runner workers.CommandRunner,
	rebind ProviderRegistryRebinder,
) (*providerregistry.Registry, error) {
	if current == nil || rebind == nil || runner == nil {
		return current, nil
	}
	rebound, err := rebind(runner)
	if err != nil {
		return nil, fmt.Errorf("rebind provider registry for runtime command runner: %w", err)
	}
	return rebound, nil
}

func applyReboundProviderRegistry(service *Service, registry *providerregistry.Registry) error {
	if service == nil || registry == nil {
		return nil
	}
	service.providerRegistry = registry
	service.invocationConductor = providerconductor.New(registry)
	assembly, err := newRuntimeAssembly(registry)
	if err != nil {
		return err
	}
	service.runtimeAssembly = assembly
	if builder, ok := service.executorBuilder.(*workerconstruction.Service); ok {
		service.executorBuilder = builder.
			WithRunnerSelection(registry.ResolveRunnerSelection).
			WithProviderIdentityResolution(registry.CanonicalIdentity)
	}
	return nil
}
