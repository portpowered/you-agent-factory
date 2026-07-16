package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelcatalog "github.com/portpowered/infinite-you/pkg/models/catalog"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
)

type stubRuntimeCacheInspector struct {
	byModel map[string]RuntimeCacheInspection
}

func (s stubRuntimeCacheInspector) InspectRuntimeCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (RuntimeCacheInspection, error) {
	if s.byModel == nil {
		return RuntimeCacheInspection{}, nil
	}
	inspection, ok := s.byModel[CanonicalModelName(modelName)]
	if !ok {
		return RuntimeCacheInspection{}, nil
	}
	return inspection, nil
}

func TestListAndInspect_ShareStableManagedRuntimeContract(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	opts := CatalogOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				Revision:           "rev-installed",
				CachePath:          "/tmp/models/OMNIVOICE_Q4_K_M/rev-installed",
				InstalledFileCount: 2,
			},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	}

	models, err := ListModelsWithOptions(loaded, opts)
	if err != nil {
		t.Fatalf("ListModelsWithOptions: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	listRuntime := models.Results[0].ManagedRuntime
	if listRuntime.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("list readiness = %s, want READY", listRuntime.ReadinessState)
	}
	if listRuntime.LifecycleState != managedruntime.LifecycleStateInstalled {
		t.Fatalf("list lifecycle = %s, want INSTALLED", listRuntime.LifecycleState)
	}

	detail, err := GetModelWithOptions(loaded, "OMNIVOICE_Q4_K_M", opts)
	if err != nil {
		t.Fatalf("GetModelWithOptions: %v", err)
	}
	inspectRuntime := detail.ManagedRuntime
	if inspectRuntime.Identity != listRuntime.Identity ||
		inspectRuntime.ReadinessState != listRuntime.ReadinessState ||
		inspectRuntime.LifecycleState != listRuntime.LifecycleState ||
		inspectRuntime.Locality != listRuntime.Locality {
		t.Fatalf("inspect runtime = %#v, want list parity %#v", inspectRuntime, listRuntime)
	}
	if detail.Diagnostics["revision"] != "rev-installed" {
		t.Fatalf("inspect diagnostics revision = %q, want rev-installed", detail.Diagnostics["revision"])
	}
	if detail.Diagnostics["installedFileCount"] != "2" {
		t.Fatalf("inspect diagnostics installedFileCount = %q, want 2", detail.Diagnostics["installedFileCount"])
	}
}

func TestListModels_MultipleRuntimesReportIndependentReadiness(t *testing.T) {
	cfg := multiRuntimeCatalogFactoryConfig()
	loaded := mustLoadedCatalogConfig(t, cfg)
	opts := CatalogOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, Revision: "rev-a"},
			"SECOND_RUNTIME":   {Supported: true, Installed: false, MissingAssets: []string{"weights.bin"}},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	}

	models, err := ListModelsWithOptions(loaded, opts)
	if err != nil {
		t.Fatalf("ListModelsWithOptions: %v", err)
	}
	if len(models.Results) != 2 {
		t.Fatalf("models count = %d, want 2", len(models.Results))
	}
	byName := map[string]modelcatalog.Summary{}
	for _, model := range models.Results {
		byName[model.Name] = model
	}
	ready := byName["OMNIVOICE_Q4_K_M"].ManagedRuntime
	missing := byName["SECOND_RUNTIME"].ManagedRuntime
	if ready.ReadinessState != managedruntime.ReadinessStateReady || ready.LifecycleState != managedruntime.LifecycleStateInstalled {
		t.Fatalf("ready runtime = (%s, %s), want READY/INSTALLED", ready.ReadinessState, ready.LifecycleState)
	}
	if missing.ReadinessState != managedruntime.ReadinessStateMissing || missing.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("missing runtime = (%s, %s), want MISSING/NOT_INSTALLED", missing.ReadinessState, missing.LifecycleState)
	}

	detail, err := GetModelWithOptions(loaded, "SECOND_RUNTIME", opts)
	if err != nil {
		t.Fatalf("GetModelWithOptions SECOND_RUNTIME: %v", err)
	}
	if detail.ManagedRuntime.Diagnostics == nil || detail.ManagedRuntime.Diagnostics["sourceKind"] != ManagedRuntimeSourceKindManagedMirror {
		t.Fatalf("mirror runtime source diagnostics = %#v, want MANAGED_MIRROR", detail.ManagedRuntime.Diagnostics)
	}
}

func TestInspectRuntimeCache_UsesLocalCacheWithoutUpstreamFetch(t *testing.T) {
	cacheDir := t.TempDir()
	puller := NewAssetPuller(cacheDir)
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))

	revisionDir := filepath.Join(cacheDir, "OMNIVOICE_Q4_K_M", "rev-local")
	if err := os.MkdirAll(revisionDir, 0o755); err != nil {
		t.Fatalf("mkdir revision cache: %v", err)
	}
	for _, name := range []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"} {
		if err := os.WriteFile(filepath.Join(revisionDir, name), []byte("fixture"), 0o644); err != nil {
			t.Fatalf("write cache file %s: %v", name, err)
		}
	}

	inspection, err := puller.InspectRuntimeCache(context.Background(), loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.Installed || inspection.InstalledFileCount != 2 {
		t.Fatalf("inspection = %#v, want supported installed cache with 2 files", inspection)
	}
}

func multiRuntimeCatalogFactoryConfig() *interfaces.FactoryConfig {
	first := workerconfig.Config{
		Name:          "voice-local",
		Type:          workertaxonomy.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: workerconfig.ModelLocalityLocal,
		Operations:    []workerconfig.ModelOperation{{Name: "TTS"}},
		Resources:     []factoryresource.Config{{Name: "omnivoice-cache", Capacity: 1}},
	}
	second := workerconfig.Config{
		Name:          "second-local",
		Type:          workertaxonomy.WorkerTypeModel,
		Model:         "SECOND_RUNTIME",
		ModelLocality: workerconfig.ModelLocalityLocal,
		Operations:    []workerconfig.ModelOperation{{Name: "EMBED"}},
		Resources:     []factoryresource.Config{{Name: "second-cache", Capacity: 1}},
	}
	return &interfaces.FactoryConfig{
		Name:    "factory",
		Workers: []workerconfig.Config{first, second},
		Resources: []factoryresource.Config{
			{
				Name:       "omnivoice-cache",
				Type:       factoryresource.TypeModel,
				Capacity:   1,
				Model:      "OMNIVOICE_Q4_K_M",
				Backend:    "GGUF",
				LoadPolicy: "ON_DEMAND",
			},
			{
				Name:       "second-cache",
				Type:       factoryresource.TypeModel,
				Capacity:   1,
				Model:      "SECOND_RUNTIME",
				Backend:    "GGUF",
				LoadPolicy: "ON_DEMAND",
				Provider:   "MODELSCOPE",
			},
		},
	}
}
