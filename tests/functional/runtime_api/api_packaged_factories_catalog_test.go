package runtime_api

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedFactoriesAPI_ReturnsPublishedCatalog(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true)

	catalog := getGeneratedJSON[factoryapi.PackagedFactoryCatalogResponse](
		t,
		server.URL()+"/packaged-factories",
	)
	if len(catalog.Factories) == 0 {
		t.Fatal("GET /packaged-factories returned no published factories")
	}
	for _, factory := range catalog.Factories {
		if factory.Name == "" || factory.Project == "" || factory.Slug == "" || len(factory.Json) == 0 || factory.Yaml == "" {
			t.Fatalf("GET /packaged-factories returned incomplete factory: %#v", factory)
		}
	}
}
