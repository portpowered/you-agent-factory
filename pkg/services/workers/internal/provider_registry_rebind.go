package internal

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
)

// ProvidersRebinder reconstructs the Providers root so migrated Codex and
// Claude integrations execute through the same runtime command edge as the
// Workers provider factories.
type ProvidersRebinder func(workers.CommandRunner) (providers.Service, error)

func rebindProvidersService(
	current providers.Service,
	runner workers.CommandRunner,
	rebind ProvidersRebinder,
) (providers.Service, error) {
	if current == nil || rebind == nil || runner == nil {
		return current, nil
	}
	rebound, err := rebind(runner)
	if err != nil {
		return nil, fmt.Errorf("rebind Providers service for runtime command runner: %w", err)
	}
	return rebound, nil
}

func applyReboundProvidersService(service *Service, providerService providers.Service) error {
	if service == nil || providerService == nil {
		return nil
	}
	service.providers = providerService
	assembly, err := newRuntimeAssembly(providerService)
	if err != nil {
		return err
	}
	service.Root = service.Root.ReplaceRuntimeAssembly(assembly)
	if builder, ok := service.executorBuilder.(*workerconstruction.Service); ok {
		service.executorBuilder = builder.WithExecutionFactories(providerService)
	}
	return nil
}
