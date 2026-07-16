package local

import (
	"context"
	"errors"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestPullModelWithOptions_ProjectsManagedRuntimeOutcomes(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	puller := &managedPullTestAssetPuller{
		result: apisurface.ModelPullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: workerconfig.ModelLocalityLocal,
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
	loaded := mustLoadedCatalogConfig(t, &interfaces.FactoryConfig{
		Name: "factory",
		Workers: []workerconfig.Config{{
			Name:          "cloud-model",
			Type:          workertaxonomy.WorkerTypeModel,
			Model:         "CLOUD_ONLY",
			ModelLocality: workerconfig.ModelLocalityCloud,
			Operations:    []workerconfig.ModelOperation{{Name: "TTS"}},
		}},
	})
	_, err := PullModelWithOptions(&managedPullTestAssetPuller{}, context.Background(), loaded, "CLOUD_ONLY", PullOptions{})
	if !errors.Is(err, apisurface.ErrModelPullUnsupported) {
		t.Fatalf("PullModelWithOptions error = %v, want ErrModelPullUnsupported", err)
	}
}

type managedPullTestAssetPuller struct {
	result apisurface.ModelPullResult
	err    error
}

func (p *managedPullTestAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return p.result, p.err
}

func (p *managedPullTestAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}

func (p *managedPullTestAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) (CacheLayout, error) {
	return CacheLayout{}, nil
}

func (p *managedPullTestAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (RuntimeCacheInspection, error) {
	return RuntimeCacheInspection{}, nil
}

func TestPullModelWithOptions_ModelScopeMirrorSourceDiagnostics(t *testing.T) {
	cfg := catalogFactoryConfig(true)
	cfg.Resources[0].Provider = "MODELSCOPE"
	loaded := mustLoadedCatalogConfig(t, cfg)
	result, err := PullModelWithOptions(&managedPullTestAssetPuller{
		result: apisurface.ModelPullResult{
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
