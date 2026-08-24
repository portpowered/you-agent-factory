package managedruntime

import (
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// ProjectManagedRuntimeState derives the compatible managed-runtime state from
// Models-owned cache and host observations. The public package owns the fact
// and result values; this internal implementation owns the projection policy.
func ProjectManagedRuntimeState(
	cache models.ManagedRuntimeCacheFacts,
	host models.ManagedRuntimeHostFacts,
) models.ManagedRuntimeStateProjection {
	if projection, handled := unsupportedManagedRuntimeProjection(cache); handled {
		return projection
	}

	cacheState := projectManagedRuntimeCacheState(cache)
	if !host.Observed || !managedRuntimeCacheIsComplete(cache) {
		return cacheState
	}
	return projectManagedRuntimeHostState(cache, host, cacheState)
}

func unsupportedManagedRuntimeProjection(
	cache models.ManagedRuntimeCacheFacts,
) (models.ManagedRuntimeStateProjection, bool) {
	if cache.Supported {
		return models.ManagedRuntimeStateProjection{}, false
	}
	if cache.Locality != LocalityLocal {
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateUnsupported,
			LifecycleState: LifecycleStateNotApplicable,
			FailureReason:  nonEmptyManagedRuntimeReason(cache.FailureReason, "cache is not supported"),
		}, true
	}
	if strings.TrimSpace(cache.FailureReason) == "" {
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateMissing,
			LifecycleState: LifecycleStateNotInstalled,
		}, true
	}
	return models.ManagedRuntimeStateProjection{}, false
}

func projectManagedRuntimeHostState(
	cache models.ManagedRuntimeCacheFacts,
	host models.ManagedRuntimeHostFacts,
	cacheState models.ManagedRuntimeStateProjection,
) models.ManagedRuntimeStateProjection {
	switch host.ReadinessState {
	case ReadinessStateReady:
		return readyManagedRuntimeHostState(host)
	case ReadinessStateLoading:
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateLoading,
			LifecycleState: LifecycleStateLoading,
		}
	case ReadinessStateFailed:
		return failedManagedRuntimeHostState(cache, host)
	default:
		return cacheState
	}
}

func readyManagedRuntimeHostState(host models.ManagedRuntimeHostFacts) models.ManagedRuntimeStateProjection {
	lifecycle := host.LifecycleState
	if lifecycle != LifecycleStateLoaded {
		lifecycle = LifecycleStateInstalled
	}
	return models.ManagedRuntimeStateProjection{
		ReadinessState: ReadinessStateReady,
		LifecycleState: lifecycle,
	}
}

func failedManagedRuntimeHostState(
	cache models.ManagedRuntimeCacheFacts,
	host models.ManagedRuntimeHostFacts,
) models.ManagedRuntimeStateProjection {
	lifecycle := host.LifecycleState
	if lifecycle == LifecycleStateNotInstalled || lifecycle == LifecycleStateInstalling || lifecycle == "" {
		lifecycle = LifecycleStateInstalled
	}
	return models.ManagedRuntimeStateProjection{
		ReadinessState: ReadinessStateFailed,
		LifecycleState: lifecycle,
		FailureReason:  nonEmptyManagedRuntimeReason(cache.FailureReason, "runtime host failed"),
	}
}

func NormalizeManagedRuntimeState(
	locality Locality,
	readiness ReadinessState,
	lifecycle LifecycleState,
) (ReadinessState, LifecycleState) {
	if locality != LocalityLocal {
		return readiness, lifecycle
	}
	switch readiness {
	case ReadinessStateReady:
		if lifecycle == LifecycleStateNotInstalled || lifecycle == LifecycleStateInstalling || lifecycle == "" {
			return ReadinessStateMissing, LifecycleStateNotInstalled
		}
	case ReadinessStateMissing:
		return ReadinessStateMissing, LifecycleStateNotInstalled
	case ReadinessStateLoading:
		if lifecycle == LifecycleStateNotInstalled || lifecycle == "" {
			return ReadinessStateLoading, LifecycleStateInstalling
		}
	}
	return readiness, lifecycle
}

func projectManagedRuntimeCacheState(
	cache models.ManagedRuntimeCacheFacts,
) models.ManagedRuntimeStateProjection {
	if cache.ActivePull && !managedRuntimeCacheIsComplete(cache) {
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateLoading,
			LifecycleState: LifecycleStateInstalling,
		}
	}
	if reason := strings.TrimSpace(cache.FailureReason); reason != "" {
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateFailed,
			LifecycleState: LifecycleStateNotInstalled,
			FailureReason:  reason,
		}
	}
	if managedRuntimeCacheIsComplete(cache) {
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateReady,
			LifecycleState: LifecycleStateInstalled,
		}
	}
	if cache.ActivePull {
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateLoading,
			LifecycleState: LifecycleStateInstalling,
		}
	}
	if managedRuntimeLegacyPartialEvidence(cache) {
		return legacyPartialManagedRuntimeState(cache)
	}
	if managedRuntimeCacheHasPartialEvidence(cache) {
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateFailed,
			LifecycleState: LifecycleStateNotInstalled,
			FailureReason:  "managed cache is incomplete or its manifest is invalid",
		}
	}
	return models.ManagedRuntimeStateProjection{
		ReadinessState: ReadinessStateMissing,
		LifecycleState: LifecycleStateNotInstalled,
	}
}

func legacyPartialManagedRuntimeState(
	cache models.ManagedRuntimeCacheFacts,
) models.ManagedRuntimeStateProjection {
	if cache.PartialArtifacts && cache.InstalledFileCount == 0 {
		return models.ManagedRuntimeStateProjection{
			ReadinessState: ReadinessStateFailed,
			LifecycleState: LifecycleStateNotInstalled,
			FailureReason:  "managed cache contains incomplete artifacts",
		}
	}
	return models.ManagedRuntimeStateProjection{
		ReadinessState: ReadinessStateLoading,
		LifecycleState: LifecycleStateInstalling,
	}
}

func managedRuntimeCacheIsComplete(cache models.ManagedRuntimeCacheFacts) bool {
	if cache.FailureReason != "" || !cache.ManifestValid {
		return legacyManagedRuntimeInstalled(cache)
	}
	if len(cache.ExpectedArtifacts) == 0 {
		return cache.Installed
	}
	observed := make(map[string]models.AssetArtifact, len(cache.ObservedArtifacts))
	for _, artifact := range cache.ObservedArtifacts {
		observed[strings.TrimSpace(artifact.Name)] = artifact
	}
	for _, expected := range cache.ExpectedArtifacts {
		artifact, ok := observed[strings.TrimSpace(expected.Name)]
		if !ok {
			return false
		}
		if expected.Bytes > 0 && artifact.Bytes != expected.Bytes {
			return false
		}
		if managedRuntimeDigestIsVerifiable(expected.SHA256) && !cache.IntegrityVerified {
			return false
		}
	}
	return true
}

func managedRuntimeDigestIsVerifiable(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') &&
			(character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}

func legacyManagedRuntimeInstalled(cache models.ManagedRuntimeCacheFacts) bool {
	return cache.Installed && !cache.ManifestPresent && len(cache.ExpectedArtifacts) == 0 &&
		len(cache.ObservedArtifacts) == 0 && !cache.ActivePull
}

func managedRuntimeCacheHasPartialEvidence(cache models.ManagedRuntimeCacheFacts) bool {
	return len(cache.ObservedArtifacts) > 0 ||
		(cache.ManifestPresent && !cache.ManifestValid) || cache.PartialArtifacts
}

func managedRuntimeLegacyPartialEvidence(cache models.ManagedRuntimeCacheFacts) bool {
	return !cache.ManifestPresent && len(cache.ExpectedArtifacts) == 0 &&
		len(cache.ObservedArtifacts) == 0 &&
		(cache.InstalledFileCount > 0 || cache.PartialArtifacts)
}

func nonEmptyManagedRuntimeReason(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
