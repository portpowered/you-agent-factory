package packaged_factories

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedFactoriesAPI_ReturnsPublishedCatalog(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	catalog := support.GetJSON[factoryapi.PackagedFactoryCatalogResponse](
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
