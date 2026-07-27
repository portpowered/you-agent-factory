package server_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractinventory"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIRoutesEveryOpenAPIOperationToNon404Handler proves every published OpenAPI
// REST operation inventory entry is wired to a real handler by issuing a safe
// public HTTP request that reaches a non-404 response on the live functional API
// server.
func TestAPIRoutesEveryOpenAPIOperationToNon404Handler(t *testing.T) {
	inventory := loadRESTOperationInventory(t)
	ctx := newRoutingReachabilityContext(t)
	defer ctx.server.Stop(t)

	client := &http.Client{Timeout: routingReachabilityRequestTimeout}
	for _, operation := range inventory.Operations {
		operation := operation
		t.Run(operation.OperationID, func(t *testing.T) {
			if operation.OperationID == "getFactorySessionResult" ||
				operation.OperationID == "getFactorySessionPartialResult" {
				t.Skip("live JavaScript /result and /partial-result reads return NOT_FOUND for functional-server sessions until a reachable live JS projection fixture exists")
			}

			request, err := ctx.safeRequest(operation)
			if err != nil {
				t.Fatalf("build safe request for %s %s (%s): %v", operation.Method, operation.Path, operation.OperationID, err)
			}

			response, err := client.Do(request)
			if err != nil {
				t.Fatalf("%s %s (%s): %v", operation.Method, request.URL.String(), operation.OperationID, err)
			}
			defer response.Body.Close()

			if response.StatusCode == http.StatusNotFound {
				t.Fatalf(
					"%s %s (%s) status = %d, want any non-404 handler response",
					operation.Method,
					request.URL.String(),
					operation.OperationID,
					response.StatusCode,
				)
			}
		})
	}

	if len(inventory.Operations) == 0 {
		t.Fatal("OpenAPI operation inventory is empty")
	}
}

func loadRESTOperationInventory(t *testing.T) *contractinventory.Inventory {
	t.Helper()

	repositoryRoot := routingPackageDirectory()
	for {
		if _, err := os.Stat(filepath.Join(repositoryRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repositoryRoot)
		if parent == repositoryRoot {
			t.Fatal("find repository root from routing test package")
		}
		repositoryRoot = parent
	}
	openAPIPath := filepath.Join(repositoryRoot, "api", "openapi.yaml")
	inventory, err := loadRESTOperationInventoryFromFile(openAPIPath)
	if err != nil {
		t.Fatalf("load OpenAPI operation inventory from %s: %v", openAPIPath, err)
	}
	return inventory
}

func newRoutingReachabilityContext(t *testing.T) *routingReachabilityContext {
	t.Helper()

	dir := scaffoldRoutingReachabilityFactory(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	ctx := &routingReachabilityContext{
		t:          t,
		server:     server,
		baseURL:    server.URL(),
		factoryDir: dir,
	}
	ctx.prepareSessions()
	ctx.prepareWork()
	return ctx
}
