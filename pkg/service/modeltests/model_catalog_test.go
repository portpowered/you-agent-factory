package modeltests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

func TestFactoryService_ListModels_SummarizesConfiguredModelCapabilities(t *testing.T) {
	svc := buildModelCatalogService(t, modelCatalogConfig(true))

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
	svc := buildModelCatalogService(t, modelCatalogConfig(false))

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
	svc := buildModelCatalogService(t, map[string]any{"name": "factory"})

	_, err := svc.GetModel(context.Background(), "missing")
	if !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("GetModel error = %v, want ErrModelNotFound", err)
	}
}

func buildModelCatalogService(t *testing.T, cfg map[string]any) *service.FactoryService {
	t.Helper()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, cfg)

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	return svc
}

func writeFactoryJSON(t *testing.T, dir string, cfg map[string]any) {
	t.Helper()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func modelCatalogConfig(includeResource bool) map[string]any {
	worker := map[string]any{
		"name":          "voice-local",
		"type":          interfaces.WorkerTypeModel,
		"model":         "OMNIVOICE_Q4_K_M",
		"modelLocality": interfaces.ModelLocalityLocal,
		"operations": []map[string]any{{
			"name": "TTS",
			"inputs": []map[string]any{{
				"name":         "text",
				"contentTypes": []string{interfaces.ModelOperationContentTypeText},
				"required":     true,
			}},
			"outputs": []map[string]any{{
				"name":         "audio",
				"contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}
	cfg := map[string]any{
		"name":    "factory",
		"workers": []map[string]any{worker},
	}
	if includeResource {
		worker["resources"] = []map[string]any{{"name": "omnivoice-cache", "capacity": 1}}
		cfg["resources"] = []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       interfaces.ResourceTypeModel,
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "GGUF",
			"loadPolicy": "ON_DEMAND",
		}}
	}
	return cfg
}
