package local

import (
	"fmt"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

type managedRuntimeProjection struct {
	summary          managedRuntimeSummary
	baseDiagnostics  map[string]string
	cacheInspection  *RuntimeCacheInspection
	sourceResolution *ManagedRuntimeSourceResolution
	includeInspect   bool
}

type managedRuntimeSummary struct {
	name       string
	locality   managedruntime.Locality
	readiness  managedruntime.ReadinessState
	lifecycle  managedruntime.LifecycleState
	operations []managedruntime.Operation
}

func buildManagedRuntime(summary managedRuntimeSummary, diagnostics map[string]string) managedruntime.Runtime {
	return buildManagedRuntimeProjection(managedRuntimeProjection{
		summary:         summary,
		baseDiagnostics: diagnostics,
	})
}

func buildManagedRuntimeProjection(input managedRuntimeProjection) managedruntime.Runtime {
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
	return managedruntime.Runtime{
		Identity:            input.summary.name,
		ReadinessState:      readiness,
		LifecycleState:      lifecycle,
		Locality:            input.summary.locality,
		SupportedOperations: input.summary.operations,
		Diagnostics:         managedDiagnostics,
	}
}

func managedRuntimeStates(input managedRuntimeProjection) (managedruntime.ReadinessState, managedruntime.LifecycleState) {
	if input.cacheInspection != nil && input.cacheInspection.Supported {
		inspection := *input.cacheInspection
		if inspection.Installed {
			return managedruntime.ReadinessStateReady, managedruntime.LifecycleStateInstalled
		}
		if inspection.PartialArtifacts && inspection.InstalledFileCount == 0 {
			return managedruntime.ReadinessStateFailed, managedruntime.LifecycleStateNotInstalled
		}
		if inspection.InstalledFileCount > 0 || inspection.PartialArtifacts {
			return managedruntime.ReadinessStateLoading, managedruntime.LifecycleStateInstalling
		}
		return managedruntime.ReadinessStateMissing, managedruntime.LifecycleStateNotInstalled
	}
	return input.summary.readiness, input.summary.lifecycle
}

func managedRuntimeSourceResolutionValue(input managedRuntimeProjection) ManagedRuntimeSourceResolution {
	if input.sourceResolution != nil {
		return *input.sourceResolution
	}
	return ManagedRuntimeSourceResolution{}
}

func managedRuntimeDiagnostics(
	summary managedRuntimeSummary,
	diagnostics map[string]string,
	readiness managedruntime.ReadinessState,
	lifecycle managedruntime.LifecycleState,
) map[string]string {
	result := map[string]string{
		"readinessState": string(readiness),
		"lifecycleState": string(lifecycle),
		"locality":       string(summary.locality),
	}
	for key, value := range diagnostics {
		result[key] = value
	}
	return result
}

func primaryModelScopedResource(aggregate catalogAggregate, factoryCfg *models.RuntimeConfig) *models.RuntimeResource {
	if factoryCfg == nil || !aggregate.hasModelScoped {
		return nil
	}
	for _, resource := range factoryCfg.Resources {
		if canonicalModelName(resource.Model) != canonicalModelName(aggregate.name) {
			continue
		}
		if strings.TrimSpace(resource.Type) != models.RuntimeResourceTypeModel {
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
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) (managedruntime.Runtime, error) {
	if runtimeCfg == nil {
		return managedruntime.Runtime{}, fmt.Errorf("runtime config is not available")
	}
	key := CanonicalModelName(modelName)
	if key == "" {
		return managedruntime.Runtime{}, fmt.Errorf("%w: empty model name", managedruntime.ErrNotFound)
	}
	return managedRuntimeForCatalog(runtimeCfg, modelName, runtimeCacheInspector, sourceResolver)
}

// EnsureManagedRuntimeReadyForInvocation classifies one managed runtime using
// the same catalog readiness projection as discovery and inspect before
// invocation proceeds.
func EnsureManagedRuntimeReadyForInvocation(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) (managedruntime.Runtime, error) {
	managed, err := ManagedRuntimeReadinessForFactory(
		runtimeCfg, modelName, runtimeCacheInspector, sourceResolver,
	)
	if err != nil {
		return managedruntime.Runtime{}, err
	}
	if invocationErr := managed.InvocationError(); invocationErr != nil {
		return managed, invocationErr
	}
	return managed, nil
}
