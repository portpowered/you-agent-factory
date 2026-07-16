package support

import (
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestComposedFunctionalHTTPHost_ReportsPublicReadiness(t *testing.T) {
	host := StartComposedFunctionalHTTPHost(t, ComposedFunctionalHTTPHostConfig{
		FactoryDir:     testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{}),
		UseMockWorkers: true,
		RuntimeMode:    interfaces.RuntimeModeService,
	})

	response, err := host.Client().Get(host.URL() + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /status status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}
