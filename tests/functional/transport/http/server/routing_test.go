package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	client := &http.Client{Timeout: routingReachabilityRequestTimeout}
	var shutdownOperation *contractinventory.Operation
	for _, operation := range inventory.Operations {
		operation := operation
		if operation.OperationID == "shutdownServer" {
			shutdownOperation = &operation
			continue
		}
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
			if operation.OperationID == "openFactorySession" {
				if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
					body, _ := io.ReadAll(response.Body)
					t.Fatalf("%s status = %d, want success: %s", operation.OperationID, response.StatusCode, strings.TrimSpace(string(body)))
				}
				var opened factoryapi.OpenFactorySessionResponse
				if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
					t.Fatalf("decode %s response: %v", operation.OperationID, err)
				}
				if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
					t.Fatalf("%s response = %#v, want explicit session", operation.OperationID, opened)
				}
				ctx.scenario.trackSession(t, opened.Session.Id)
				return
			}
			if response.StatusCode == http.StatusNotFound {
				switch operation.OperationID {
				case "getHumanApprovalBySessionId":
					body, readErr := io.ReadAll(response.Body)
					if readErr != nil {
						t.Fatalf("read expected human-approval not-found response: %v", readErr)
					}
					if !strings.Contains(string(body), "human approval not found") {
						t.Fatalf("%s %s (%s) returned an unrelated 404 response: %s", operation.Method, request.URL.String(), operation.OperationID, strings.TrimSpace(string(body)))
					}
					return
				case "removeModel":
					assertModelCacheNotFoundResponse(t, response)
					return
				}
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
	if shutdownOperation == nil {
		t.Fatal("OpenAPI operation inventory does not contain shutdownServer")
	}

	// shutdownServer is process-mutating: keep its lifecycle witness isolated so
	// it cannot terminate the package-owned server needed by later cases.
	shutdownDir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	shutdownServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                shutdownDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer shutdownServer.Stop(t)
	shutdownContext := *ctx
	shutdownContext.baseURL = shutdownServer.URL()

	t.Run(shutdownOperation.OperationID, func(t *testing.T) {
		request, err := shutdownContext.safeRequest(*shutdownOperation)
		if err != nil {
			t.Fatalf("build safe request for %s %s (%s): %v", shutdownOperation.Method, shutdownOperation.Path, shutdownOperation.OperationID, err)
		}

		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("%s %s (%s): %v", shutdownOperation.Method, request.URL.String(), shutdownOperation.OperationID, err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("%s %s (%s) status = %d, want %d: %s", shutdownOperation.Method, request.URL.String(), shutdownOperation.OperationID, response.StatusCode, http.StatusAccepted, strings.TrimSpace(string(body)))
		}

		timer := time.NewTimer(routingReachabilityRequestTimeout)
		defer timer.Stop()
		select {
		case <-shutdownServer.Done():
		case <-timer.C:
			t.Fatal("shutdownServer acknowledged but functional server did not stop")
		}
	})
}

func assertModelCacheNotFoundResponse(t *testing.T, response *http.Response) {
	t.Helper()
	if !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("removeModel 404 Content-Type = %q, want JSON", response.Header.Get("Content-Type"))
	}
	var body factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode removeModel 404 response: %v", err)
	}
	if body.Family != factoryapi.ErrorFamilyNotFound || string(body.Code) != "MODEL_CACHE_NOT_FOUND" {
		t.Fatalf("removeModel 404 response = %#v, want MODEL_CACHE_NOT_FOUND not-found error", body)
	}
}

// TestAPIUnknownRouteReturnsStructuredNotFound proves requests to paths outside the
// published OpenAPI surface return a structured not-found response at the public
// HTTP contract boundary.
func TestAPIUnknownRouteReturnsStructuredNotFound(t *testing.T) {
	dir := scaffoldC06HTTPFactory(t, startupShutdownTestFactoryConfig())
	server := c06SharedHTTPServer(t).newScenario(t, "routing-unknown-route", dir)

	unknownPath := "/functional-routing-unknown-route"
	response, err := http.Get(server.URL() + unknownPath)
	if err != nil {
		t.Fatalf("GET %s: %v", unknownPath, err)
	}
	defer response.Body.Close()

	assertStructuredNotFoundHTTPResponse(t, response)
}

func TestAPIDashboardRoutesServeEmbeddedShellAssetAndFallback(t *testing.T) {
	dir := scaffoldC06HTTPFactory(t, startupShutdownTestFactoryConfig())
	server := c06SharedHTTPServer(t).newScenario(t, "routing-dashboard", dir)

	shellResponse, err := http.Get(server.URL() + "/dashboard/ui")
	if err != nil {
		t.Fatalf("GET dashboard shell: %v", err)
	}
	shellBody, err := io.ReadAll(shellResponse.Body)
	_ = shellResponse.Body.Close()
	if err != nil {
		t.Fatalf("read dashboard shell: %v", err)
	}
	if shellResponse.StatusCode != http.StatusOK {
		t.Fatalf("dashboard shell status = %d, want %d: %s", shellResponse.StatusCode, http.StatusOK, strings.TrimSpace(string(shellBody)))
	}

	const assetMarker = "/dashboard/ui/assets/"
	assetStart := strings.Index(string(shellBody), assetMarker)
	if assetStart < 0 {
		t.Fatalf("dashboard shell did not contain %q", assetMarker)
	}
	assetEnd := strings.Index(string(shellBody)[assetStart:], "\"")
	if assetEnd < 0 {
		t.Fatal("dashboard shell asset path was not quoted")
	}
	assetPath := string(shellBody)[assetStart : assetStart+assetEnd]

	assetResponse, err := http.Get(server.URL() + assetPath)
	if err != nil {
		t.Fatalf("GET dashboard asset: %v", err)
	}
	assetBody, err := io.ReadAll(assetResponse.Body)
	_ = assetResponse.Body.Close()
	if err != nil {
		t.Fatalf("read dashboard asset: %v", err)
	}
	if assetResponse.StatusCode != http.StatusOK || len(assetBody) == 0 {
		t.Fatalf("dashboard asset response = status %d len %d", assetResponse.StatusCode, len(assetBody))
	}

	fallbackResponse, err := http.Get(server.URL() + "/dashboard/ui/client-route")
	if err != nil {
		t.Fatalf("GET dashboard fallback: %v", err)
	}
	fallbackBody, err := io.ReadAll(fallbackResponse.Body)
	_ = fallbackResponse.Body.Close()
	if err != nil {
		t.Fatalf("read dashboard fallback: %v", err)
	}
	if fallbackResponse.StatusCode != http.StatusOK || !strings.Contains(string(fallbackBody), "<div id=\"root\"></div>") {
		t.Fatalf("dashboard fallback response = status %d body %q", fallbackResponse.StatusCode, string(fallbackBody))
	}

	invalidParamsResponse, err := http.Get(server.URL() + "/provider-sessions/detail?kind=session_id&id=routing-reachability")
	if err != nil {
		t.Fatalf("GET provider session detail with missing provider: %v", err)
	}
	defer invalidParamsResponse.Body.Close()
	if invalidParamsResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid provider session detail status = %d, want %d", invalidParamsResponse.StatusCode, http.StatusBadRequest)
	}
}

// TestAPIWrongMethodReturnsDocumentedMethodError proves wrong HTTP methods on known
// OpenAPI routes return the documented method-error response at the public HTTP
// contract boundary instead of a not-found outcome that would hide the mismatch.
func TestAPIWrongMethodReturnsDocumentedMethodError(t *testing.T) {
	dir := scaffoldC06HTTPFactory(t, startupShutdownTestFactoryConfig())
	server := c06SharedHTTPServer(t).newScenario(t, "routing-wrong-method", dir)

	knownPath := "/status"
	request, err := http.NewRequest(http.MethodPost, server.URL()+knownPath, nil)
	if err != nil {
		t.Fatalf("build POST %s: %v", knownPath, err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", knownPath, err)
	}
	defer response.Body.Close()

	assertStructuredMethodNotAllowedHTTPResponse(t, response)
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

func assertStructuredMethodNotAllowedHTTPResponse(t *testing.T, response *http.Response) {
	t.Helper()

	if response.StatusCode != http.StatusMethodNotAllowed {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d: %s", response.StatusCode, http.StatusMethodNotAllowed, strings.TrimSpace(string(body)))
	}
	if response.StatusCode == http.StatusNotFound {
		t.Fatal("wrong-method probe returned not-found, want documented method-error status")
	}

	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("Content-Type = %q, want application/json structured error body: %s", contentType, strings.TrimSpace(string(body)))
	}

	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&errResp); err != nil {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("decode structured method-error response: %v\nbody: %s", err, strings.TrimSpace(string(body)))
	}
	if errResp.Family == factoryapi.ErrorFamilyNotFound {
		t.Fatalf("error family = %q, want method-error family distinct from not-found", errResp.Family)
	}
	if errResp.Code == factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("error code = %q, want method-error code distinct from not-found", errResp.Code)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error family = %q, want %q", errResp.Family, factoryapi.ErrorFamilyBadRequest)
	}
	if errResp.Code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("error code = %q, want %q", errResp.Code, "METHOD_NOT_ALLOWED")
	}
	if strings.TrimSpace(errResp.Message) == "" {
		t.Fatal("error message is empty, want a customer-readable method-error message")
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
	fixture := c06SharedHTTPServer(t)
	scenario := fixture.newScenario(t, "routing-reachability", dir, liveJavaScriptFactoryDir)
	scenario.registerRunner(t, []string{fixture.factoryDir, dir, liveJavaScriptFactoryDir}, generatedClientStreamingRunner{})
	ctx := &routingReachabilityContext{
		t:                        t,
		scenario:                 scenario,
		baseURL:                  scenario.URL(),
		factoryDir:               dir,
		liveJavaScriptFactoryDir: liveJavaScriptFactoryDir,
	}
	ctx.prepareSessions()
	ctx.prepareWork()
	return ctx
}
