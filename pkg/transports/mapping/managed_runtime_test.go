package apisurface

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

	t.Run("managed projection with source diagnostics", func(t *testing.T) {
		sourceKind := "MANAGED_MIRROR"
		sourceID := "managed-mirror:OMNIVOICE_Q4_K_M"
		notes := "assets resolve through configured managed mirror source"
		result := ManagedRuntimePullResultFromService(ModelPullResult{
			ModelName:          "OMNIVOICE_Q4_K_M",
			ProviderLocality:   interfaces.ModelLocalityLocal,
			Outcome:            "ALREADY_PRESENT",
			ManagedPullOutcome: "ALREADY_READY",
			ReadinessState:     "READY",
			SourceKind:         sourceKind,
			SourceID:           sourceID,
			ResolverNotes:      notes,
		}, nil)

		if result.PullOutcome != factoryapi.ManagedRuntimePullOutcomeALREADYREADY {
			t.Fatalf("pull outcome = %s, want ALREADY_READY", result.PullOutcome)
		}
		if result.SourceDiagnostics == nil || result.SourceDiagnostics.SourceKind == nil || *result.SourceDiagnostics.SourceKind != sourceKind {
			t.Fatalf("source diagnostics = %#v, want managed mirror kind", result.SourceDiagnostics)
		}
	})
}
