package runtimeopening

import (
	"fmt"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProviderFromCommandRunnerFactory constructs one provider-backed worker from a
// Workers command runner using the same production edges as direct invocation.
type ProviderFromCommandRunnerFactory func(platformprocess.CommandRunner) (providers.Service, error)

func resolveDurableExecutionProvider(
	providerOverride providers.Service,
	mockWorkers *workers.MockWorkersConfig,
	runtimeCfg interfaces.RuntimeDefinitionLookup,
	platformRunner platformprocess.CommandRunner,
	mockRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	buildProvider ProviderFromCommandRunnerFactory,
) (providers.Service, error) {
	if providerOverride != nil {
		return providerOverride, nil
	}
	if mockWorkers == nil ||
		!mockWorkers.UnmatchedDispatchPolicy.PassthroughUnmatched() ||
		platformRunner == nil ||
		buildProvider == nil {
		return nil, nil
	}
	wrapped := mockRunnerFactory(mockWorkers, runtimeCfg, platformRunner)
	if wrapped == nil {
		return nil, nil
	}
	provider, err := buildProvider(wrapped)
	if err != nil {
		return nil, fmt.Errorf("compose durable session provider: %w", err)
	}
	return provider, nil
}
