package local

import (
	"context"
	"errors"
	"testing"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPullModelWithOptions_ProjectsManagedRuntimeOutcomes(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	puller := &managedPullTestAssetPuller{
		result: apisurface.PullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: apisurface.RuntimeModelLocalityLocal,
			Outcome:          legacyPullOutcomeAlreadyPresent,
			CachePath:        "/tmp/models/OMNIVOICE_Q4_K_M/rev1",
			Revision:         "rev1",
		},
	}
	opts := PullOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				Revision:           "rev1",
				CachePath:          "/tmp/models/OMNIVOICE_Q4_K_M/rev1",
				InstalledFileCount: 2,
			},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	}

	result, err := PullModelWithOptions(puller, context.Background(), loaded, "OMNIVOICE_Q4_K_M", opts)
	if err != nil {
		t.Fatalf("PullModelWithOptions: %v", err)
	}
	if result.ManagedPullOutcome != managedPullOutcomeAlreadyReady {
		t.Fatalf("managed pull outcome = %q, want ALREADY_READY", result.ManagedPullOutcome)
	}
	if result.ReadinessState != managedReadinessReady || result.LifecycleState != managedLifecycleInstalled {
		t.Fatalf("readiness/lifecycle = (%q, %q), want READY INSTALLED", result.ReadinessState, result.LifecycleState)
	}
	if result.SourceKind != ManagedRuntimeSourceKindUpstreamRepository {
		t.Fatalf("source kind = %q, want upstream repository", result.SourceKind)
	}
}

func TestPullModelWithOptions_ClassifiesUnsupportedLocalModel(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, &testFactoryConfig{
		Name: "factory",
		Workers: []modelRuntimeWorker{{
			Name:          "cloud-model",
			Type:          apisurface.RuntimeWorkerTypeModel,
			Model:         "CLOUD_ONLY",
			ModelLocality: apisurface.RuntimeModelLocalityCloud,
			Operations:    []apisurface.RuntimeOperation{{Name: "TTS"}},
		}},
	})
	_, err := PullModelWithOptions(&managedPullTestAssetPuller{}, context.Background(), loaded, "CLOUD_ONLY", PullOptions{})
	if !errors.Is(err, apisurface.ErrPullUnsupported) {
		t.Fatalf("PullModelWithOptions error = %v, want ErrModelPullUnsupported", err)
	}
}

type managedPullTestAssetPuller struct {
	result apisurface.PullResult
	err    error
}

func (p *managedPullTestAssetPuller) PullModel(context.Context, *modelRuntimeConfig, string) (apisurface.PullResult, error) {
	return p.result, p.err
}

func (p *managedPullTestAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}

func (p *managedPullTestAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (CacheLayout, error) {
	return CacheLayout{}, nil
}

func (p *managedPullTestAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (RuntimeCacheInspection, error) {
	return RuntimeCacheInspection{}, nil
}

func TestPullModelWithOptions_ModelScopeMirrorSourceDiagnostics(t *testing.T) {
	t.Parallel()
	cfg := catalogFactoryConfig(true)
	cfg.Resources[0].Provider = "MODELSCOPE"
	loaded := mustLoadedCatalogConfig(t, cfg)
	result, err := PullModelWithOptions(&managedPullTestAssetPuller{
		result: apisurface.PullResult{
			ModelName: "OMNIVOICE_Q4_K_M",
			Outcome:   legacyPullOutcomePulled,
		},
	}, context.Background(), loaded, "OMNIVOICE_Q4_K_M", PullOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	})
	if err != nil {
		t.Fatalf("PullModelWithOptions: %v", err)
	}
	if result.SourceKind != ManagedRuntimeSourceKindManagedMirror {
		t.Fatalf("source kind = %q, want managed mirror", result.SourceKind)
	}
	if result.ManagedPullOutcome != managedPullOutcomeInstalledSuccessfully {
		t.Fatalf("pull outcome = %q, want INSTALLED_SUCCESSFULLY", result.ManagedPullOutcome)
	}
	if factoryapi.ManagedRuntimeReadinessState(result.ReadinessState) != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readiness = %q, want READY", result.ReadinessState)
	}
}
