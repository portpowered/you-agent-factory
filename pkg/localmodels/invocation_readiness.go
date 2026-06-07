package localmodels

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// EnsureManagedRuntimeReadyForInvocation classifies one managed runtime using
// the same catalog readiness projection as discovery and inspect before
// invocation proceeds.
func EnsureManagedRuntimeReadyForInvocation(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
	opts CatalogOptions,
) (factoryapi.ManagedRuntime, error) {
	managed, err := ManagedRuntimeReadinessForFactory(runtimeCfg, modelName, opts)
	if err != nil {
		return factoryapi.ManagedRuntime{}, err
	}
	if invocationErr := apisurface.InvocationErrorFromManagedRuntime(managed); invocationErr != nil {
		return managed, invocationErr
	}
	return managed, nil
}
