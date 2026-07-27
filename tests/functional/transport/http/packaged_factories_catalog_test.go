package http_test

import (
	"encoding/json"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedFactoriesAPI_ReturnsPublishedCatalog proves the HTTP API exposes the published Factory catalog.
func TestPackagedFactoriesAPI_ReturnsPublishedCatalog(t *testing.T) {
	dir := support.ScaffoldFactory(t, packagedFactoryCatalogTestConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	response, err := http.Get(server.URL() + "/packaged-factories")
	if err != nil {
		t.Fatalf("GET /packaged-factories: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /packaged-factories status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	var catalog factoryapi.PackagedFactoryCatalogResponse
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil {
		t.Fatalf("decode GET /packaged-factories: %v", err)
	}
	if len(catalog.Factories) == 0 {
		t.Fatal("GET /packaged-factories returned no published factories")
	}
	for _, factory := range catalog.Factories {
		if factory.Name == "" || factory.Project == "" || factory.Slug == "" || len(factory.Json) == 0 || factory.Yaml == "" {
			t.Fatalf("GET /packaged-factories returned incomplete factory: %#v", factory)
		}
	}
}

func packagedFactoryCatalogTestConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
