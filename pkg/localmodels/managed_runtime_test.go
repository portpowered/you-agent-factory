package localmodels

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestBuildManagedRuntime_MapsCompatibilityFieldsToManagedContract(t *testing.T) {
	summary := factoryapi.ModelSummary{
		Name:             "OMNIVOICE_Q4_K_M",
		ProviderLocality: factoryapi.WorkerModelLocalityLocal,
		Status:           factoryapi.ModelStatusREADY,
		LoadState:        factoryapi.UNLOADED,
		Operations:       []factoryapi.ModelOperation{{Name: "TTS"}},
	}
	diagnostics := factoryapi.StringMap{"statusReason": "ready"}

	managed := buildManagedRuntime(summary, diagnostics)

	if managed.Identity != summary.Name {
		t.Fatalf("identity = %q, want %q", managed.Identity, summary.Name)
	}
	if managed.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
	if managed.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("lifecycle = %s, want NOT_INSTALLED", managed.LifecycleState)
	}
	if managed.Locality != summary.ProviderLocality {
		t.Fatalf("locality = %s, want %s", managed.Locality, summary.ProviderLocality)
	}
	if len(managed.SupportedOperations) != 1 || managed.SupportedOperations[0].Name != "TTS" {
		t.Fatalf("supported operations = %#v, want one TTS operation", managed.SupportedOperations)
	}
	if managed.Diagnostics == nil || (*managed.Diagnostics)["readinessState"] != "READY" || (*managed.Diagnostics)["statusReason"] != "ready" {
		t.Fatalf("diagnostics = %#v, want managed-runtime projections", managed.Diagnostics)
	}
}

func TestListModels_PopulatesManagedRuntimeContract(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))

	models, err := ListModels(loaded)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	model := models.Results[0]
	if model.ManagedRuntime.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("managed runtime identity = %q, want OMNIVOICE_Q4_K_M", model.ManagedRuntime.Identity)
	}
	if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("managed readiness = %s, want READY", model.ManagedRuntime.ReadinessState)
	}
}

func TestBuildManagedRuntimeProjection_ReportsLoadingAndFailedCacheStates(t *testing.T) {
	loading := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary: factoryapi.ModelSummary{
			Name:             "OMNIVOICE_Q4_K_M",
			ProviderLocality: factoryapi.WorkerModelLocalityLocal,
		},
		cacheInspection: &RuntimeCacheInspection{
			Supported:          true,
			Installed:          false,
			InstalledFileCount: 1,
			MissingAssets:      []string{"omnivoice-tokenizer-Q4_K_M.gguf"},
		},
	})
	if loading.ReadinessState != factoryapi.ManagedRuntimeReadinessStateLOADING ||
		loading.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLING {
		t.Fatalf("loading runtime = (%s, %s), want LOADING/INSTALLING", loading.ReadinessState, loading.LifecycleState)
	}

	failed := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary: factoryapi.ModelSummary{
			Name:             "OMNIVOICE_Q4_K_M",
			ProviderLocality: factoryapi.WorkerModelLocalityLocal,
		},
		cacheInspection: &RuntimeCacheInspection{
			Supported:        true,
			Installed:        false,
			PartialArtifacts: true,
		},
	})
	if failed.ReadinessState != factoryapi.ManagedRuntimeReadinessStateFAILED ||
		failed.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("failed runtime = (%s, %s), want FAILED/NOT_INSTALLED", failed.ReadinessState, failed.LifecycleState)
	}
}

func TestBuildManagedRuntimeProjection_ReportsInstalledCacheState(t *testing.T) {
	summary := factoryapi.ModelSummary{
		Name:             "OMNIVOICE_Q4_K_M",
		ProviderLocality: factoryapi.WorkerModelLocalityLocal,
		Status:           factoryapi.ModelStatusREADY,
		LoadState:        factoryapi.UNLOADED,
	}
	inspection := RuntimeCacheInspection{
		Supported:          true,
		Installed:          true,
		Revision:           "rev1",
		InstalledFileCount: 2,
	}
	managed := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary:         summary,
		baseDiagnostics: factoryapi.StringMap{"statusReason": "ready"},
		cacheInspection: &inspection,
		includeInspect:  true,
	})
	if managed.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("lifecycle = %s, want INSTALLED", managed.LifecycleState)
	}
	if managed.Diagnostics == nil || (*managed.Diagnostics)["revision"] != "rev1" {
		t.Fatalf("diagnostics = %#v, want revision detail", managed.Diagnostics)
	}
}
