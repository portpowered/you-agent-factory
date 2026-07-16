package apisurface

import (
	"testing"

	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestManagedRuntimeToAPI_PreservesModelOwnedVocabulary(t *testing.T) {
	required := true
	result := ManagedRuntimeToAPI(managedruntime.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateInstalled,
		Locality:       managedruntime.LocalityLocal,
		SupportedOperations: []managedruntime.Operation{{
			Name: "TTS",
			Inputs: []managedruntime.OperationSlot{{
				Name: "text", ContentTypes: []string{"TEXT"}, Required: &required,
			}},
		}},
		Diagnostics: map[string]string{"sourceKind": "MANAGED_MIRROR"},
	})

	if result.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		result.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED ||
		result.Locality != factoryapi.WorkerModelLocalityLocal {
		t.Fatalf("managed runtime = %#v, want READY/INSTALLED/LOCAL", result)
	}
	if len(result.SupportedOperations) != 1 || result.SupportedOperations[0].Inputs == nil ||
		len(*result.SupportedOperations[0].Inputs) != 1 || (*result.SupportedOperations[0].Inputs)[0].ContentTypes[0] != factoryapi.ModelOperationContentTypeText {
		t.Fatalf("operations = %#v, want TTS text input", result.SupportedOperations)
	}
	if result.Diagnostics == nil || (*result.Diagnostics)["sourceKind"] != "MANAGED_MIRROR" {
		t.Fatalf("diagnostics = %#v, want source kind", result.Diagnostics)
	}
}

func TestManagedRuntimePullResultFromService_MapsLegacyOutcomes(t *testing.T) {
	t.Run("pulled", func(t *testing.T) {
		result := ManagedRuntimePullResultFromService(ModelPullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: workerconfig.ModelLocalityLocal,
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
			ProviderLocality: workerconfig.ModelLocalityLocal,
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
			ProviderLocality:   workerconfig.ModelLocalityLocal,
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
