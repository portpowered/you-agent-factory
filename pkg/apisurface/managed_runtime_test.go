package apisurface

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestManagedRuntimePullResultFromService_MapsLegacyOutcomes(t *testing.T) {
	t.Run("pulled", func(t *testing.T) {
		result := ManagedRuntimePullResultFromService(ModelPullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: interfaces.ModelLocalityLocal,
			Outcome:          "PULLED",
			CachePath:        "/tmp/cache",
			Revision:         "rev1",
		}, []factoryapi.ModelPullDownloadedFile{{Path: "model.gguf", Bytes: 10}})

		if result.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY {
			t.Fatalf("pull outcome = %s, want INSTALLED_SUCCESSFULLY", result.PullOutcome)
		}
		if result.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
			t.Fatalf("readiness = %s, want READY", result.ReadinessState)
		}
	})

	t.Run("already present", func(t *testing.T) {
		result := ManagedRuntimePullResultFromService(ModelPullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: interfaces.ModelLocalityLocal,
			Outcome:          "ALREADY_PRESENT",
		}, nil)

		if result.PullOutcome != factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT {
			t.Fatalf("pull outcome = %s, want ALREADY_PRESENT", result.PullOutcome)
		}
	})
}
