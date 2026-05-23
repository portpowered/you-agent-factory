package service

import (
	"context"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryService_ListModels_SummarizesConfiguredModelCapabilities(t *testing.T) {
	loaded := mustLoadedFactoryConfigForModelCatalogTest(t, &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []interfaces.WorkerConfig{{
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
			Resources: []interfaces.ResourceConfig{{Name: "omnivoice-cache", Capacity: 1}},
		}},
	})
	svc := &FactoryService{runtimeCfg: loaded}

	models, err := svc.ListModels(context.Background())
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

func TestFactoryService_GetModel_ReturnsUnavailableWithoutMatchingLocalModelResource(t *testing.T) {
	loaded := mustLoadedFactoryConfigForModelCatalogTest(t, &interfaces.FactoryConfig{
		Workers: []interfaces.WorkerConfig{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: interfaces.ModelLocalityLocal,
			Operations:    []interfaces.ModelOperation{{Name: "TTS"}},
		}},
	})
	svc := &FactoryService{runtimeCfg: loaded}

	model, err := svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
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

func TestFactoryService_GetModel_ReturnsNotFoundForUnknownModel(t *testing.T) {
	loaded := mustLoadedFactoryConfigForModelCatalogTest(t, &interfaces.FactoryConfig{})
	svc := &FactoryService{runtimeCfg: loaded}

	_, err := svc.GetModel(context.Background(), "missing")
	if !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("GetModel error = %v, want ErrModelNotFound", err)
	}
}

func mustLoadedFactoryConfigForModelCatalogTest(t *testing.T, cfg *interfaces.FactoryConfig) *factoryconfig.LoadedFactoryConfig {
	t.Helper()
	loaded, err := factoryconfig.NewLoadedFactoryConfig("factory-dir", cfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}
