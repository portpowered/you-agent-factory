package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestServer_ListModels_RoutesThroughWiredModelService(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, modelWiringFactoryConfig(true))

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	srv := NewServer(svc, 0, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /models status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	response := decodeJSONResponse[factoryapi.ListModelsResponse](t, rec)
	if len(response.Results) != 1 || response.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("models = %#v, want one OMNIVOICE summary", response.Results)
	}
}

func modelWiringFactoryConfig(includeResource bool) map[string]any {
	worker := map[string]any{
		"name":          "voice-local",
		"type":          interfaces.WorkerTypeModel,
		"modelProvider": "CODEX",
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
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}}
	}
	return cfg
}
