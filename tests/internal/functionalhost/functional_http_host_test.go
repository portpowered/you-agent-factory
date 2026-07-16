package functionalhost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestFunctionalHTTPHost_ReadinessDeadlineCancelsStalledPublicResponse(t *testing.T) {
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
		close(requestCanceled)
	}))
	defer server.Close()

	host := &FunctionalHTTPHost{url: server.URL, client: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := host.waitForReady(ctx)
	if err == nil {
		t.Fatal("waitForReady succeeded for a stalled public response")
	}
	if !strings.Contains(err.Error(), "last public observation") || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("waitForReady error = %q, want bounded public-observation diagnostic", err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("stalled status request remained live after readiness deadline")
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

func TestFunctionalHTTPServer_ShutdownJoinsOwnedService(t *testing.T) {
	host, err := StartFunctionalHTTPServer(context.Background(), FunctionalHTTPServerConfig{
		Address:        "127.0.0.1:0",
		FactoryDir:     testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{}),
		UseMockWorkers: true,
	})
	if err != nil {
		t.Fatalf("start functional HTTP server: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := host.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown functional HTTP server: %v", err)
	}
	select {
	case <-host.serviceDone:
	default:
		t.Fatal("functional HTTP server returned before its owned service exited")
	}
}
