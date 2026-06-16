package modelhost

import (
	"strconv"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// ClassifyReadiness maps cache inspection and catalog identity into host readiness.
func ClassifyReadiness(identity Identity, inspection CacheInspection, unsupported bool) ReadinessSnapshot {
	if unsupported {
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE,
			FailureClass:   FailureClassUnsupportedRuntime,
			Diagnostics:    managedDiagnostics(identity, factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED, factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE),
		}
	}
	if inspection.Supported {
		readiness, lifecycle, failureClass := readinessFromCacheInspection(inspection)
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: readiness,
			LifecycleState: lifecycle,
			FailureClass:   failureClass,
			Diagnostics:    mergeDiagnostics(identity, readiness, lifecycle, cacheDiagnostics(inspection)),
		}
	}
	readiness := readinessFromLocality(identity.Locality)
	lifecycle := lifecycleFromLocality(identity.Locality)
	return ReadinessSnapshot{
		Identity:       identity,
		ReadinessState: readiness,
		LifecycleState: lifecycle,
		FailureClass:   FailureClassForReadinessState(readiness),
		Diagnostics:    managedDiagnostics(identity, readiness, lifecycle),
	}
}

func readinessFromCacheInspection(inspection CacheInspection) (
	factoryapi.ManagedRuntimeReadinessState,
	factoryapi.ManagedRuntimeLifecycleState,
	FailureClass,
) {
	if inspection.Installed {
		return factoryapi.ManagedRuntimeReadinessStateREADY,
			factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
			FailureClassNone
	}
	if inspection.PartialArtifacts && inspection.InstalledFileCount == 0 {
		return factoryapi.ManagedRuntimeReadinessStateFAILED,
			factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			FailureClassMissingAssets
	}
	if inspection.InstalledFileCount > 0 || inspection.PartialArtifacts {
		return factoryapi.ManagedRuntimeReadinessStateLOADING,
			factoryapi.ManagedRuntimeLifecycleStateINSTALLING,
			FailureClassLoadingTimeout
	}
	return factoryapi.ManagedRuntimeReadinessStateMISSING,
		factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
		FailureClassMissingAssets
}

func readinessFromLocality(locality factoryapi.WorkerModelLocality) factoryapi.ManagedRuntimeReadinessState {
	switch locality {
	case factoryapi.WorkerModelLocalityLocal:
		return factoryapi.ManagedRuntimeReadinessStateMISSING
	default:
		return factoryapi.ManagedRuntimeReadinessStateREADY
	}
}

func lifecycleFromLocality(locality factoryapi.WorkerModelLocality) factoryapi.ManagedRuntimeLifecycleState {
	switch locality {
	case factoryapi.WorkerModelLocalityLocal:
		return factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED
	default:
		return factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE
	}
}

func managedDiagnostics(
	identity Identity,
	readiness factoryapi.ManagedRuntimeReadinessState,
	lifecycle factoryapi.ManagedRuntimeLifecycleState,
) map[string]string {
	diagnostics := map[string]string{
		"readinessState": string(readiness),
		"lifecycleState": string(lifecycle),
		"locality":       string(identity.Locality),
	}
	for key, value := range sourceDiagnostics(identity) {
		diagnostics[key] = value
	}
	if identity.Name != "" {
		diagnostics["identity"] = identity.Name
	}
	return diagnostics
}

func mergeDiagnostics(
	identity Identity,
	readiness factoryapi.ManagedRuntimeReadinessState,
	lifecycle factoryapi.ManagedRuntimeLifecycleState,
	extra map[string]string,
) map[string]string {
	diagnostics := managedDiagnostics(identity, readiness, lifecycle)
	for key, value := range extra {
		diagnostics[key] = value
	}
	return diagnostics
}

func sourceDiagnostics(identity Identity) map[string]string {
	if identity.SourceKind == "" {
		return nil
	}
	return map[string]string{
		"sourceKind":    identity.SourceKind,
		"sourceId":      identity.SourceID,
		"resolverNotes": identity.ResolverNotes,
	}
}

func cacheDiagnostics(inspection CacheInspection) map[string]string {
	if !inspection.Supported {
		return nil
	}
	diagnostics := make(map[string]string)
	if len(inspection.MissingAssets) > 0 {
		diagnostics["missingAssets"] = strings.Join(inspection.MissingAssets, ",")
	}
	if inspection.Revision != "" {
		diagnostics["revision"] = inspection.Revision
	}
	if inspection.CachePath != "" {
		diagnostics["cachePath"] = inspection.CachePath
	}
	if inspection.Installed {
		diagnostics["installedFileCount"] = strconv.Itoa(inspection.InstalledFileCount)
	}
	return diagnostics
}
