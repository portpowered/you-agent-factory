package runtimeopening

import (
	"fmt"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProviderFromCommandRunnerFactory constructs one provider-backed worker from a
// Workers command runner using the same production edges as direct invocation.
type ProviderFromCommandRunnerFactory func(workers.CommandRunner) (workers.Provider, error)

func resolveDurableExecutionProvider(
	providerOverride workers.Provider,
	mockWorkers *workers.MockWorkersConfig,
	runtimeCfg interfaces.RuntimeDefinitionLookup,
	platformRunner platformprocess.CommandRunner,
	adaptRunner WorkerCommandRunnerAdapter,
	mockRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	buildProvider ProviderFromCommandRunnerFactory,
) (workers.Provider, error) {
	if providerOverride != nil {
		return providerOverride, nil
	}
	if mockWorkers == nil ||
		!mockWorkers.UnmatchedDispatchPolicy.PassthroughUnmatched() ||
		platformRunner == nil ||
		buildProvider == nil {
		return nil, nil
	}
	runner := adaptRunner(platformRunner)
	if runner == nil {
		return nil, nil
	}
	wrapped := mockRunnerFactory(mockWorkers, runtimeCfg, runner)
	if wrapped == nil {
		return nil, nil
	}
	provider, err := buildProvider(wrapped)
	if err != nil {
		return nil, fmt.Errorf("compose durable session provider: %w", err)
	}
	return provider, nil
}
