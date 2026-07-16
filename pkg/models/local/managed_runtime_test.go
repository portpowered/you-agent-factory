package local

import (
	"testing"

	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestBuildManagedRuntime_MapsCompatibilityFieldsToManagedContract(t *testing.T) {
	summary := managedRuntimeSummary{
		name:       "OMNIVOICE_Q4_K_M",
		locality:   managedruntime.LocalityLocal,
		readiness:  managedruntime.ReadinessStateReady,
		lifecycle:  managedruntime.LifecycleStateNotInstalled,
		operations: []managedruntime.Operation{{Name: "TTS"}},
	}
	diagnostics := map[string]string{"statusReason": "ready"}

	managed := buildManagedRuntime(summary, diagnostics)

	if managed.Identity != summary.name {
		t.Fatalf("identity = %q, want %q", managed.Identity, summary.name)
	}
	if managed.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
	if managed.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("lifecycle = %s, want NOT_INSTALLED", managed.LifecycleState)
	}
	if managed.Locality != summary.locality {
		t.Fatalf("locality = %s, want %s", managed.Locality, summary.locality)
	}
	if len(managed.SupportedOperations) != 1 || managed.SupportedOperations[0].Name != "TTS" {
		t.Fatalf("supported operations = %#v, want one TTS operation", managed.SupportedOperations)
	}
	if managed.Diagnostics["readinessState"] != "READY" || managed.Diagnostics["statusReason"] != "ready" {
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
		summary: managedRuntimeSummary{
			name:     "OMNIVOICE_Q4_K_M",
			locality: managedruntime.LocalityLocal,
		},
		cacheInspection: &RuntimeCacheInspection{
			Supported:          true,
			Installed:          false,
			InstalledFileCount: 1,
			MissingAssets:      []string{"omnivoice-tokenizer-Q4_K_M.gguf"},
		},
	})
	if loading.ReadinessState != managedruntime.ReadinessStateLoading ||
		loading.LifecycleState != managedruntime.LifecycleStateInstalling {
		t.Fatalf("loading runtime = (%s, %s), want LOADING/INSTALLING", loading.ReadinessState, loading.LifecycleState)
	}

	failed := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary: managedRuntimeSummary{
			name:     "OMNIVOICE_Q4_K_M",
			locality: managedruntime.LocalityLocal,
		},
		cacheInspection: &RuntimeCacheInspection{
			Supported:        true,
			Installed:        false,
			PartialArtifacts: true,
		},
	})
	if failed.ReadinessState != managedruntime.ReadinessStateFailed ||
		failed.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("failed runtime = (%s, %s), want FAILED/NOT_INSTALLED", failed.ReadinessState, failed.LifecycleState)
	}
}

func TestBuildManagedRuntimeProjection_ReportsInstalledCacheState(t *testing.T) {
	summary := managedRuntimeSummary{
		name:      "OMNIVOICE_Q4_K_M",
		locality:  managedruntime.LocalityLocal,
		readiness: managedruntime.ReadinessStateReady,
		lifecycle: managedruntime.LifecycleStateNotInstalled,
	}
	inspection := RuntimeCacheInspection{
		Supported:          true,
		Installed:          true,
		Revision:           "rev1",
		InstalledFileCount: 2,
	}
	managed := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary:         summary,
		baseDiagnostics: map[string]string{"statusReason": "ready"},
		cacheInspection: &inspection,
		includeInspect:  true,
	})
	if managed.LifecycleState != managedruntime.LifecycleStateInstalled {
		t.Fatalf("lifecycle = %s, want INSTALLED", managed.LifecycleState)
	}
	if managed.Diagnostics["revision"] != "rev1" {
		t.Fatalf("diagnostics = %#v, want revision detail", managed.Diagnostics)
	}
}
