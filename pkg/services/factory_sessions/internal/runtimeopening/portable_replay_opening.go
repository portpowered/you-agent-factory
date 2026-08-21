package runtimeopening

import (
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// openPortableReplayDurableOwner constructs the existing durable execution
// owner for a checkpoint-bearing replay. The portable projection contributes
// only the eligibility hint; HasRestorableState on this owner remains the
// authority that decides whether a handoff is possible.
func openPortableReplayDurableOwner(
	configured preparedRuntime,
	root RuntimeRoot,
	clockEdge factoryruntime.Clock,
	providerOverride providers.Service,
	providerCommandRunner workers.CommandRunner,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory,
	durableExecutionFactory DurableExecutionFactory,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	providerIdentities factorysessions.ProviderIdentityResolver,
	resolveClock factoryruntime.ClockResolver,
) (durableexecution.Service, error) {
	if durableExecutionFactory == nil {
		return nil, fmt.Errorf("construct portable replay runtime: durable execution operation is required")
	}
	clock, err := clockForReplay(clockEdge, nil, nil, resolveClock)
	if err != nil {
		return nil, err
	}
	providerForDurable, err := resolveDurableExecutionProvider(
		providerOverride,
		configured.Workers.MockWorkers,
		nil,
		providerCommandRunner,
		workersMockCommandRunnerFactory,
		providerFromCommandRunnerFactory,
	)
	if err != nil {
		return nil, err
	}
	durable, err := durableExecutionFactory(
		configured.Definition,
		configured.Session,
		configured.OperatorDefaults,
		root,
		clock,
		providerForDurable,
		configured.Workers.MockWorkers,
		factorySessionExecutionFactory,
		providerIdentities,
	)
	if err != nil {
		return nil, err
	}
	return durable.Service, nil
}
