package apisurface

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ManagedRuntimePullResultFromService maps a service-owned pull result into the
// public managed-runtime pull contract while preserving legacy outcome fields.
func ManagedRuntimePullResultFromService(result ModelPullResult, files []factoryapi.ModelPullDownloadedFile) factoryapi.ManagedRuntimePullResult {
	legacyOutcome := factoryapi.ModelPullOutcome(strings.TrimSpace(strings.ToUpper(result.Outcome)))
	pull := factoryapi.ManagedRuntimePullResult{
		Identity:       result.ModelName,
		PullOutcome:    managedRuntimePullOutcomeFromLegacy(legacyOutcome),
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
	}
	if cachePath := strings.TrimSpace(result.CachePath); cachePath != "" {
		pull.CachePath = &cachePath
	}
	if revision := strings.TrimSpace(result.Revision); revision != "" {
		pull.Revision = &revision
	}
	if len(files) > 0 {
		copied := append([]factoryapi.ModelPullDownloadedFile(nil), files...)
		pull.DownloadedFiles = &copied
	}
	if strings.TrimSpace(result.ProviderLocality) == interfaces.ModelLocalityCloud {
		pull.ReadinessState = factoryapi.ManagedRuntimeReadinessStateREADY
		pull.PullOutcome = factoryapi.ManagedRuntimePullOutcomeALREADYREADY
	}
	return pull
}

func managedRuntimePullOutcomeFromLegacy(outcome factoryapi.ModelPullOutcome) factoryapi.ManagedRuntimePullOutcome {
	switch outcome {
	case factoryapi.ModelPullOutcomePULLED:
		return factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY
	case factoryapi.ModelPullOutcomeALREADYPRESENT:
		return factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT
	default:
		return factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME
	}
}
