package server_test

import (
	"bytes"
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

// TestAPIJSONRequestsAndResponsesUseDocumentedContentType proves JSON requests and
// responses on the public HTTP API use the documented application/json media type
// declared in the published OpenAPI contract.
func TestAPIJSONRequestsAndResponsesUseDocumentedContentType(t *testing.T) {
	operation := loadContentNegotiationOperation(t, "openFactorySession")
	requestMediaType := documentedJSONRequestMediaType(t, operation)
	responseMediaType := documentedJSONSuccessResponseMediaType(t, operation)

	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)

	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: dir})
	if err != nil {
		t.Fatalf("marshal open factory session request: %v", err)
	}

	endpoint := strings.TrimSuffix(server.URL(), "/") + "/factory-sessions"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build POST %s: %v", endpoint, err)
	}
	request.Header.Set("Content-Type", requestMediaType)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST %s status = %d, want %d with documented JSON content type %q: %s",
			endpoint,
			response.StatusCode,
			http.StatusOK,
			requestMediaType,
			strings.TrimSpace(string(body)),
		)
	}

	responseContentType := response.Header.Get("Content-Type")
	if !strings.Contains(responseContentType, responseMediaType) {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"Content-Type = %q, want documented JSON response media type %q: %s",
			responseContentType,
			responseMediaType,
			strings.TrimSpace(string(body)),
		)
	}

	var opened factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("decode open factory session response: %v\nbody: %s", err, strings.TrimSpace(string(body)))
	}
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("open factory session response = %#v, want session id", opened)
	}
}

func loadContentNegotiationOperation(t *testing.T, operationID string) contractinventory.Operation {
	t.Helper()

	repositoryRoot := contentNegotiationRepositoryRoot(t)
	openAPIPath := filepath.Join(repositoryRoot, "api", "openapi.yaml")
	inventory, err := loadRESTOperationInventoryFromFile(openAPIPath)
	if err != nil {
		t.Fatalf("load OpenAPI operation inventory from %s: %v", openAPIPath, err)
	}
	for _, operation := range inventory.Operations {
		if operation.OperationID == operationID {
			return operation
		}
	}
	t.Fatalf("operation %q not found in OpenAPI inventory", operationID)
	return contractinventory.Operation{}
}

func documentedJSONRequestMediaType(t *testing.T, operation contractinventory.Operation) string {
	t.Helper()

	if len(operation.RequestMediaTypes) == 0 {
		t.Fatalf("%s request media types are empty, want documented JSON request body", operation.OperationID)
	}
	requestMediaType := operation.RequestMediaTypes[0]
	if !strings.Contains(requestMediaType, "application/json") {
		t.Fatalf(
			"%s documented request media type = %q, want application/json",
			operation.OperationID,
			requestMediaType,
		)
	}
	return requestMediaType
}

func documentedJSONSuccessResponseMediaType(t *testing.T, operation contractinventory.Operation) string {
	t.Helper()

	for _, response := range operation.Responses {
		if response.Status != "200" {
			continue
		}
		if len(response.MediaTypes) == 0 {
			t.Fatalf("%s 200 response media types are empty, want documented JSON response body", operation.OperationID)
		}
		responseMediaType := response.MediaTypes[0]
		if !strings.Contains(responseMediaType, "application/json") {
			t.Fatalf(
				"%s documented 200 response media type = %q, want application/json",
				operation.OperationID,
				responseMediaType,
			)
		}
		return responseMediaType
	}
	t.Fatalf("%s has no documented 200 JSON response in OpenAPI inventory", operation.OperationID)
	return ""
}

func contentNegotiationRepositoryRoot(t *testing.T) string {
	t.Helper()

	repositoryRoot := routingPackageDirectory()
	for {
		if _, err := os.Stat(filepath.Join(repositoryRoot, "go.mod")); err == nil {
			return repositoryRoot
		}
		parent := filepath.Dir(repositoryRoot)
		if parent == repositoryRoot {
			t.Fatal("find repository root from content negotiation test package")
		}
		repositoryRoot = parent
	}
	return ""
}
