package local

import (
	"fmt"
	"strings"

	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type managedRuntimeProjection struct {
	summary          factoryapi.ModelSummary
	baseDiagnostics  factoryapi.StringMap
	cacheInspection  *RuntimeCacheInspection
	sourceResolution *ManagedRuntimeSourceResolution
	includeInspect   bool
}

func buildManagedRuntime(summary factoryapi.ModelSummary, diagnostics factoryapi.StringMap) factoryapi.ManagedRuntime {
	return buildManagedRuntimeProjection(managedRuntimeProjection{
		summary:         summary,
		baseDiagnostics: diagnostics,
	})
}

func buildManagedRuntimeProjection(input managedRuntimeProjection) factoryapi.ManagedRuntime {
	readiness, lifecycle := managedRuntimeStates(input)
	managedDiagnostics := managedRuntimeDiagnostics(input.summary, input.baseDiagnostics, readiness, lifecycle)
	for key, value := range managedRuntimeSourceDiagnostics(managedRuntimeSourceResolutionValue(input)) {
		managedDiagnostics[key] = value
	}
	if input.cacheInspection != nil {
		for key, value := range runtimeCacheInspectDiagnostics(*input.cacheInspection, input.includeInspect) {
			managedDiagnostics[key] = value
		}
	}
	return factoryapi.ManagedRuntime{
		Identity:            input.summary.Name,
		ReadinessState:      readiness,
		LifecycleState:      lifecycle,
		Locality:            input.summary.ProviderLocality,
		SupportedOperations: input.summary.Operations,
		Diagnostics:         &managedDiagnostics,
	}
}

func managedRuntimeStates(input managedRuntimeProjection) (factoryapi.ManagedRuntimeReadinessState, factoryapi.ManagedRuntimeLifecycleState) {
	if input.cacheInspection != nil && input.cacheInspection.Supported {
		inspection := *input.cacheInspection
		if inspection.Installed {
			return factoryapi.ManagedRuntimeReadinessStateREADY, factoryapi.ManagedRuntimeLifecycleStateINSTALLED
		}
		if inspection.PartialArtifacts && inspection.InstalledFileCount == 0 {
			return factoryapi.ManagedRuntimeReadinessStateFAILED, factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED
		}
		if inspection.InstalledFileCount > 0 || inspection.PartialArtifacts {
			return factoryapi.ManagedRuntimeReadinessStateLOADING, factoryapi.ManagedRuntimeLifecycleStateINSTALLING
		}
		return factoryapi.ManagedRuntimeReadinessStateMISSING, factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED
	}
	return managedRuntimeReadinessFromStatus(input.summary.Status), managedRuntimeLifecycleFromLoadState(input.summary.LoadState)
}

func managedRuntimeSourceResolutionValue(input managedRuntimeProjection) ManagedRuntimeSourceResolution {
	if input.sourceResolution != nil {
		return *input.sourceResolution
	}
	return ManagedRuntimeSourceResolution{}
}

func managedRuntimeReadinessFromStatus(status factoryapi.ModelStatus) factoryapi.ManagedRuntimeReadinessState {
	switch status {
	case factoryapi.ModelStatusREADY:
		return factoryapi.ManagedRuntimeReadinessStateREADY
	case factoryapi.ModelStatusUNAVAILABLE:
		return factoryapi.ManagedRuntimeReadinessStateMISSING
	default:
		return factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED
	}
}

func managedRuntimeLifecycleFromLoadState(loadState factoryapi.ModelLoadState) factoryapi.ManagedRuntimeLifecycleState {
	switch loadState {
	case factoryapi.UNLOADED:
		return factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED
	case factoryapi.NOTAPPLICABLE:
		return factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE
	default:
		return factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE
	}
}

func managedRuntimeDiagnostics(
	summary factoryapi.ModelSummary,
	diagnostics factoryapi.StringMap,
	readiness factoryapi.ManagedRuntimeReadinessState,
	lifecycle factoryapi.ManagedRuntimeLifecycleState,
) factoryapi.StringMap {
	result := factoryapi.StringMap{
		"readinessState": string(readiness),
		"lifecycleState": string(lifecycle),
		"locality":       string(summary.ProviderLocality),
	}
	for key, value := range diagnostics {
		result[key] = value
	}
	return result
}

func primaryModelScopedResource(aggregate catalogAggregate, factoryCfg *interfaces.FactoryConfig) *factoryresource.Config {
	if factoryCfg == nil || !aggregate.hasModelScoped {
		return nil
	}
	for _, resource := range factoryCfg.Resources {
		if canonicalModelName(resource.Model) != canonicalModelName(aggregate.name) {
			continue
		}
		if strings.TrimSpace(resource.Type) != factoryresource.TypeModel {
			continue
		}
		copied := resource
		return &copied
	}
	return nil
}

// ManagedRuntimeReadinessForFactory returns the canonical managed-runtime readiness projection
// for one factory dependency identity using the same catalog path for packaged and authored factories.
func ManagedRuntimeReadinessForFactory(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
	opts CatalogOptions,
) (factoryapi.ManagedRuntime, error) {
	if runtimeCfg == nil {
		return factoryapi.ManagedRuntime{}, fmt.Errorf("runtime config is not available")
	}
	catalog := BuildCatalogWithOptions(runtimeCfg, opts)
	key := CanonicalModelName(modelName)
	if key == "" {
		return factoryapi.ManagedRuntime{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return factoryapi.ManagedRuntime{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
	}
	return entry.Summary.ManagedRuntime, nil
}

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
