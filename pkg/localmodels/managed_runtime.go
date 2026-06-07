package localmodels

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func buildManagedRuntime(summary factoryapi.ModelSummary, diagnostics factoryapi.StringMap) factoryapi.ManagedRuntime {
	managedDiagnostics := managedRuntimeDiagnostics(summary, diagnostics)
	return factoryapi.ManagedRuntime{
		Identity:            summary.Name,
		ReadinessState:      managedRuntimeReadinessFromStatus(summary.Status),
		LifecycleState:      managedRuntimeLifecycleFromLoadState(summary.LoadState),
		Locality:            summary.ProviderLocality,
		SupportedOperations: summary.Operations,
		Diagnostics:         &managedDiagnostics,
	}
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

func managedRuntimeDiagnostics(summary factoryapi.ModelSummary, diagnostics factoryapi.StringMap) factoryapi.StringMap {
	result := factoryapi.StringMap{
		"readinessState": string(managedRuntimeReadinessFromStatus(summary.Status)),
		"lifecycleState": string(managedRuntimeLifecycleFromLoadState(summary.LoadState)),
		"locality":       string(summary.ProviderLocality),
	}
	for key, value := range diagnostics {
		result[key] = value
	}
	return result
}
