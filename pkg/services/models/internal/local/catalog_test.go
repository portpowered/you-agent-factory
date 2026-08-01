package local

import (
	"context"
	"errors"
	"testing"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
)

func TestCanonicalModelName_NormalizesCaseAndWhitespace(t *testing.T) {
	t.Parallel()
	if got := CanonicalModelName("  omnivoice_q4_k_m  "); got != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("CanonicalModelName() = %q, want OMNIVOICE_Q4_K_M", got)
	}
	if CanonicalModelName("") != "" {
		t.Fatal("CanonicalModelName(\"\") = non-empty, want empty")
	}
}

func TestListModels_SummarizesConfiguredModelCapabilities(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))

	list, err := ListModels(loaded)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(list.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(list.Results))
	}
	model := list.Results[0]
	if model.Name != "OMNIVOICE_Q4_K_M" || model.ProviderLocality != managedruntime.LocalityLocal {
		t.Fatalf("model summary = %#v, want OMNIVOICE local model", model)
	}
	if model.Status != modelcatalog.StatusReady || model.LoadState != modelcatalog.LoadStateUnloaded {
		t.Fatalf("model readiness = (%s, %s), want (READY, UNLOADED)", model.Status, model.LoadState)
	}
	if len(model.Operations) != 1 || model.Operations[0].Name != "TTS" {
		t.Fatalf("operations = %#v, want one TTS operation", model.Operations)
	}
	if len(model.Modalities) != 2 {
		t.Fatalf("modalities = %#v, want TEXT and AUDIO", model.Modalities)
	}
	if len(model.Resources) != 1 || model.Resources[0].Name != "omnivoice-cache" {
		t.Fatalf("resources = %#v, want omnivoice-cache summary", model.Resources)
	}
}

func TestGetModel_ReturnsUnavailableWithoutMatchingLocalModelResource(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(false))

	model, err := GetModel(loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if model.Status != modelcatalog.StatusUnavailable {
		t.Fatalf("status = %s, want UNAVAILABLE", model.Status)
	}
	if model.Diagnostics["statusReason"] == "" {
		t.Fatalf("diagnostics = %#v, want statusReason", model.Diagnostics)
	}
}

func TestGetModel_ReturnsNotFoundForUnknownModel(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, &testFactoryConfig{Name: "factory"})

	_, err := GetModel(loaded, "missing")
	if !errors.Is(err, apisurface.ErrNotFound) {
		t.Fatalf("GetModel error = %v, want ErrModelNotFound", err)
	}
}

func TestPullModel_DelegatesToAssetPullerForLocalCatalogModel(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	puller := &recordingCatalogAssetPuller{}

	result, err := PullModel(puller, context.Background(), loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if puller.calls != 1 || puller.modelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("puller calls = %d modelName = %q, want 1 and OMNIVOICE_Q4_K_M", puller.calls, puller.modelName)
	}
	if result.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("result.ModelName = %q, want OMNIVOICE_Q4_K_M", result.ModelName)
	}
}

func TestPullModel_ReturnsNotFoundForUnknownModel(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, &testFactoryConfig{Name: "factory"})
	puller := &recordingCatalogAssetPuller{}

	_, err := PullModel(puller, context.Background(), loaded, "missing")
	if !errors.Is(err, apisurface.ErrNotFound) {
		t.Fatalf("PullModel error = %v, want ErrModelNotFound", err)
	}
	if puller.calls != 0 {
		t.Fatalf("puller calls = %d, want 0 before delegation", puller.calls)
	}
}

func TestSelectInvocationWorker_ResolvesModelWorkerAndOperation(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))

	worker, operation, err := SelectInvocationWorker(loaded, "OMNIVOICE_Q4_K_M", "TTS")
	if err != nil {
		t.Fatalf("SelectInvocationWorker: %v", err)
	}
	if worker.Name != "voice-local" || operation.Name != "TTS" {
		t.Fatalf("worker/operation = (%q, %q), want (voice-local, TTS)", worker.Name, operation.Name)
	}
}

func TestSelectInvocationWorker_ResolvesInferenceWorkerTaxonomyAlias(t *testing.T) {
	t.Parallel()
	factoryCfg := catalogFactoryConfig(true)
	factoryCfg.Workers[0].Type = apisurface.RuntimeWorkerTypeInference
	loaded := mustLoadedCatalogConfig(t, factoryCfg)

	worker, operation, err := SelectInvocationWorker(loaded, "OMNIVOICE_Q4_K_M", "TTS")
	if err != nil {
		t.Fatalf("SelectInvocationWorker: %v", err)
	}
	if worker.Type != apisurface.RuntimeWorkerTypeInference || operation.Name != "TTS" {
		t.Fatalf("worker/operation = (%#v, %q), want inference worker with TTS", worker, operation.Name)
	}
}

type recordingCatalogAssetPuller struct {
	calls     int
	modelName string
}

func (p *recordingCatalogAssetPuller) PullModel(_ context.Context, _ *modelRuntimeConfig, modelName string) (apisurface.PullResult, error) {
	p.calls++
	p.modelName = modelName
	return apisurface.PullResult{ModelName: modelName}, nil
}

func (p *recordingCatalogAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}

func (p *recordingCatalogAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (CacheLayout, error) {
	return CacheLayout{}, nil
}

func (p *recordingCatalogAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (RuntimeCacheInspection, error) {
	return RuntimeCacheInspection{}, nil
}

func mustLoadedCatalogConfig(t *testing.T, factoryCfg *testFactoryConfig) *modelRuntimeConfig {
	t.Helper()
	return projectTestModelsRuntimeConfig(t.TempDir(), factoryCfg)
}

func catalogFactoryConfig(includeResource bool) *testFactoryConfig {
	worker := modelRuntimeWorker{
		Name:          "voice-local",
		Type:          apisurface.RuntimeWorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: apisurface.RuntimeModelLocalityLocal,
		Operations: []apisurface.RuntimeOperation{{
			Name: "TTS",
			Inputs: []apisurface.RuntimeOperationSlot{{
				Name:         "text",
				ContentTypes: []string{apisurface.RuntimeContentTypeText},
				Required:     true,
			}},
			Outputs: []apisurface.RuntimeOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{apisurface.RuntimeContentTypeAudio},
			}},
		}},
	}
	cfg := &testFactoryConfig{
		Name:    "factory",
		Workers: []modelRuntimeWorker{worker},
	}
	if includeResource {
		worker.Resources = []modelRuntimeResource{{Name: "omnivoice-cache", Capacity: 1}}
		cfg.Resources = []modelRuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       apisurface.RuntimeResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}}
	}
	return cfg
}
