package functionalhost

import (
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestFunctionalHTTPHost_ExposesReadyStatusAndBoundedShutdown(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{})
	host := StartFunctionalHTTPHost(t, FunctionalHTTPHostConfig{
		FactoryDir:     dir,
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

	host.Stop(t)
	select {
	case <-host.Done():
	default:
		t.Fatal("host remains running after Stop")
	}
}

func TestFunctionalHTTPHost_IndependentHostsUseDistinctPublicAddresses(t *testing.T) {
	first := StartFunctionalHTTPHost(t, FunctionalHTTPHostConfig{
		FactoryDir:     testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{}),
		UseMockWorkers: true,
		RuntimeMode:    interfaces.RuntimeModeService,
	})
	second := StartFunctionalHTTPHost(t, FunctionalHTTPHostConfig{
		FactoryDir:     testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{}),
		UseMockWorkers: true,
		RuntimeMode:    interfaces.RuntimeModeService,
	})

	if first.URL() == second.URL() {
		t.Fatalf("independent hosts share URL %q", first.URL())
	}
}
