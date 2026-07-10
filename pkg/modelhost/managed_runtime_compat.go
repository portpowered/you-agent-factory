package modelhost

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/localmodels"
)

// EnsureInvocationReady classifies invocation readiness through the model host boundary.
func EnsureInvocationReady(
	ctx context.Context,
	host Host,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (factoryapi.ManagedRuntime, error) {
	if runtimeCfg == nil {
		return factoryapi.ManagedRuntime{}, fmt.Errorf("runtime config is not available")
	}
	if host == nil {
		return localmodels.EnsureManagedRuntimeReadyForInvocation(runtimeCfg, modelName, localmodels.CatalogOptions{
			SourceResolver: localmodels.DefaultManagedRuntimeSourceResolver(),
		})
	}
	snapshot, err := invocationReadinessSnapshot(ctx, host, runtimeCfg, modelName)
	if err != nil {
		return factoryapi.ManagedRuntime{}, err
	}
	managed := ManagedRuntimeFromSnapshot(snapshot)
	if invocationErr := apisurface.InvocationErrorFromManagedRuntime(managed); invocationErr != nil {
		return managed, invocationErr
	}
	return managed, nil
}

func invocationReadinessSnapshot(
	ctx context.Context,
	host Host,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (ReadinessSnapshot, error) {
	return host.InspectReadiness(ctx, runtimeCfg, modelName)
}
