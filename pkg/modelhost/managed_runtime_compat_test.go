package modelhost

import (
	"context"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestListModelsWithHost_ProjectsMissingManagedRuntimeFromHost(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := NewCatalogHost(stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:     true,
				MissingAssets: []string{"model.gguf"},
			},
		},
	}, Options{})

	models, err := ListModelsWithHost(context.Background(), host, loaded)
	if err != nil {
		t.Fatalf("ListModelsWithHost: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	if models.Results[0].ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING {
		t.Fatalf("readiness = %s, want MISSING", models.Results[0].ManagedRuntime.ReadinessState)
	}
}

func TestGetModelWithHost_ProjectsSupervisedLoadingWhenAssetsInstalled(t *testing.T) {
	factoryCfg := catalogFactoryConfig(true)
	factoryCfg.Resources[0].Backend = "LLAMACPP"
	loaded := mustLoadedCatalogConfig(t, factoryCfg)
	host := NewCatalogHost(stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
		},
	}, Options{})

	model, err := GetModelWithHost(context.Background(), host, loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModelWithHost: %v", err)
	}
	if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateLOADING {
		t.Fatalf("readiness = %s, want LOADING", model.ManagedRuntime.ReadinessState)
	}
}

func TestEnsureInvocationReady_AllowsInstalledAssetsWithoutSupervisedOverlay(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := NewCatalogHost(stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
		},
	}, Options{})

	managed, err := EnsureInvocationReady(context.Background(), host, loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("EnsureInvocationReady: %v", err)
	}
	if managed.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
}

func TestPullWithHost_MapsUnsupportedRuntimeToModelPullUnsupported(t *testing.T) {
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{
		Name: "factory",
		Workers: []interfaces.WorkerConfig{{
			Name:          "cloud-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "GPT_CLOUD",
			ModelLocality: interfaces.ModelLocalityCloud,
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	host := NewCatalogHost(stubAssetGateway{}, Options{})

	_, err = PullWithHost(context.Background(), host, loaded, "GPT_CLOUD")
	if err == nil || !errors.Is(err, apisurface.ErrModelPullUnsupported) {
		t.Fatalf("PullWithHost error = %v, want ErrModelPullUnsupported", err)
	}
}

func TestModelPullResultFromSnapshot_PreservesPullMetadata(t *testing.T) {
	result := ModelPullResultFromSnapshot(PullSnapshot{
		ReadinessSnapshot: ReadinessSnapshot{
			Identity: Identity{
				Name:     "OMNIVOICE_Q4_K_M",
				Locality: factoryapi.WorkerModelLocalityLocal,
			},
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
		},
		PullOutcome:   factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY,
		LegacyOutcome: "PULLED",
		CachePath:     "/tmp/cache",
		Revision:      "rev1",
		DownloadedFiles: []PullDownloadedFile{{
			Path:   "model.gguf",
			Bytes:  42,
			SHA256: "abc",
		}},
	})
	if result.Outcome != "PULLED" || result.CachePath != "/tmp/cache" || len(result.DownloadedFiles) != 1 {
		t.Fatalf("pull result = %#v, want pull metadata", result)
	}
	if result.ManagedPullOutcome != "INSTALLED_SUCCESSFULLY" || result.ReadinessState != "READY" {
		t.Fatalf("managed pull fields = (%q, %q), want INSTALLED_SUCCESSFULLY READY", result.ManagedPullOutcome, result.ReadinessState)
	}
}
