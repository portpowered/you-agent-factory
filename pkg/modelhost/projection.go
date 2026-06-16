package modelhost

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// ManagedRuntimeFromSnapshot projects one host readiness snapshot into the public contract.
func ManagedRuntimeFromSnapshot(snapshot ReadinessSnapshot) factoryapi.ManagedRuntime {
	diagnostics := factoryapi.StringMap{}
	for key, value := range snapshot.Diagnostics {
		diagnostics[key] = value
	}
	if snapshot.FailureClass != FailureClassNone {
		diagnostics["failureClass"] = string(snapshot.FailureClass)
	}
	return factoryapi.ManagedRuntime{
		Identity:            snapshot.Identity.Name,
		ReadinessState:      snapshot.ReadinessState,
		LifecycleState:      snapshot.LifecycleState,
		Locality:            snapshot.Identity.Locality,
		SupportedOperations: append([]factoryapi.ModelOperation(nil), snapshot.Identity.SupportedOperations...),
		Diagnostics:         &diagnostics,
	}
}
