package localmodels

import (
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestCanonicalModelName_NormalizesCaseAndWhitespace(t *testing.T) {
	if got := CanonicalModelName("  omnivoice_q4_k_m  "); got != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("CanonicalModelName() = %q, want OMNIVOICE_Q4_K_M", got)
	}
	if CanonicalModelName("") != "" {
		t.Fatal("CanonicalModelName(\"\") = non-empty, want empty")
	}
}

func TestListModels_SummarizesConfiguredModelCapabilities(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))

	models, err := ListModels(loaded)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	model := models.Results[0]
	if model.Name != "OMNIVOICE_Q4_K_M" || model.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
		t.Fatalf("model summary = %#v, want OMNIVOICE local model", model)
	}
	if model.Status != factoryapi.ModelStatusREADY || model.LoadState != factoryapi.UNLOADED {
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
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(false))

	model, err := GetModel(loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if model.Status != factoryapi.ModelStatusUNAVAILABLE {
		t.Fatalf("status = %s, want UNAVAILABLE", model.Status)
	}
	if model.Diagnostics["statusReason"] == "" {
		t.Fatalf("diagnostics = %#v, want statusReason", model.Diagnostics)
	}
}

func TestGetModel_ReturnsNotFoundForUnknownModel(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, &interfaces.FactoryConfig{Name: "factory"})

	_, err := GetModel(loaded, "missing")
	if !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("GetModel error = %v, want ErrModelNotFound", err)
	}
}

func mustLoadedCatalogConfig(t *testing.T, factoryCfg *interfaces.FactoryConfig) *factoryconfig.LoadedFactoryConfig {
	t.Helper()
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func catalogFactoryConfig(includeResource bool) *interfaces.FactoryConfig {
	worker := interfaces.WorkerConfig{
		Name:          "voice-local",
		Type:          interfaces.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: interfaces.ModelLocalityLocal,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{interfaces.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []interfaces.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}
	cfg := &interfaces.FactoryConfig{
		Name:    "factory",
		Workers: []interfaces.WorkerConfig{worker},
	}
	if includeResource {
		worker.Resources = []interfaces.ResourceConfig{{Name: "omnivoice-cache", Capacity: 1}}
		cfg.Resources = []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}}
	}
	return cfg
}
