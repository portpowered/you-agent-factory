package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractinventory"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

// TestAPIUnknownRouteReturnsStructuredNotFound proves requests to paths outside the
// published OpenAPI surface return a structured not-found response at the public
// HTTP contract boundary.
func TestAPIUnknownRouteReturnsStructuredNotFound(t *testing.T) {
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)

	unknownPath := "/functional-routing-unknown-route"
	response, err := http.Get(server.URL() + unknownPath)
	if err != nil {
		t.Fatalf("GET %s: %v", unknownPath, err)
	}
	defer response.Body.Close()

	assertStructuredNotFoundHTTPResponse(t, response)
}

func assertStructuredNotFoundHTTPResponse(t *testing.T, response *http.Response) {
	t.Helper()

	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d: %s", response.StatusCode, http.StatusNotFound, strings.TrimSpace(string(body)))
	}

	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("Content-Type = %q, want application/json structured error body: %s", contentType, strings.TrimSpace(string(body)))
	}

	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&errResp); err != nil {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("decode structured not-found response: %v\nbody: %s", err, strings.TrimSpace(string(body)))
	}
	if errResp.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("error family = %q, want %q", errResp.Family, factoryapi.ErrorFamilyNotFound)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("error code = %q, want %q", errResp.Code, factoryapi.ErrorResponseCodeNOTFOUND)
	}
	if strings.TrimSpace(errResp.Message) == "" {
		t.Fatal("error message is empty, want a customer-readable not-found message")
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
	liveJavaScriptFactoryDir := scaffoldRoutingLiveJavaScriptFactory(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	ctx := &routingReachabilityContext{
		t:                        t,
		server:                   server,
		baseURL:                  server.URL(),
		factoryDir:               dir,
		liveJavaScriptFactoryDir: liveJavaScriptFactoryDir,
	}
	ctx.prepareSessions()
	ctx.prepareWork()
	return ctx
}
