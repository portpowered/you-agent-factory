package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
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
	if opened.Session.IsDefault {
		t.Fatalf("open factory session response = %#v, want a unique non-default session", opened)
	}
	terminateNegotiatedFactorySession(t, server.URL(), opened.Session.Id)
	closeNegotiatedFactorySession(t, server.URL(), opened.Session.Id)
	assertNegotiatedFactorySessionAbsent(t, server.URL(), opened.Session.Id)
}

// TestAPIUnsupportedContentTypeReturns415 proves requests with an unsupported
// Content-Type against JSON-bodied public endpoints return HTTP 415 Unsupported
// Media Type at the public HTTP contract boundary instead of accepting the body
// or returning an unrelated validation or routing failure.
func TestAPIUnsupportedContentTypeReturns415(t *testing.T) {
	operation := loadContentNegotiationOperation(t, "openFactorySession")
	documentedRequestMediaType := documentedJSONRequestMediaType(t, operation)

	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)
	beforeSessions := liveNegotiationSessionIDs(t, server.URL())

	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: dir})
	if err != nil {
		t.Fatalf("marshal open factory session request: %v", err)
	}

	endpoint := strings.TrimSuffix(server.URL(), "/") + "/factory-sessions"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build POST %s: %v", endpoint, err)
	}
	request.Header.Set("Content-Type", "text/plain")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()

	assertUnsupportedMediaTypeHTTPResponse(t, response, documentedRequestMediaType)
	assertLiveNegotiationSessionIDsUnchanged(t, server.URL(), beforeSessions)
}

// TestAPIMalformedJSONReturnsStructured400 proves requests with the documented JSON
// content type and a malformed body return a structured HTTP 400 at the public HTTP
// contract boundary instead of an unsupported media type rejection or an unstructured
// error response.
func TestAPIMalformedJSONReturnsStructured400(t *testing.T) {
	operation := loadContentNegotiationOperation(t, "openFactorySession")
	requestMediaType := documentedJSONRequestMediaType(t, operation)

	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)
	beforeSessions := liveNegotiationSessionIDs(t, server.URL())

	endpoint := strings.TrimSuffix(server.URL(), "/") + "/factory-sessions"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader([]byte(`{"folderPath":`)))
	if err != nil {
		t.Fatalf("build POST %s: %v", endpoint, err)
	}
	request.Header.Set("Content-Type", requestMediaType)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()

	assertMalformedJSONHTTPResponse(t, response)
	assertLiveNegotiationSessionIDsUnchanged(t, server.URL(), beforeSessions)
}

func liveNegotiationSessionIDs(t *testing.T, baseURL string) map[string]struct{} {
	t.Helper()

	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions?scope=live",
	)
	ids := make(map[string]struct{}, len(listed.Sessions))
	for _, session := range listed.Sessions {
		ids[session.Id] = struct{}{}
	}
	return ids
}

func assertLiveNegotiationSessionIDsUnchanged(t *testing.T, baseURL string, before map[string]struct{}) {
	t.Helper()

	after := liveNegotiationSessionIDs(t, baseURL)
	if len(after) != len(before) {
		t.Fatalf("live Factory Session IDs after rejected request = %#v, want unchanged from %#v", after, before)
	}
	for id := range before {
		if _, ok := after[id]; !ok {
			t.Fatalf("live Factory Session %q disappeared after rejected request; before=%#v after=%#v", id, before, after)
		}
	}
}

func closeNegotiatedFactorySession(t *testing.T, baseURL, sessionID string) {
	t.Helper()

	request, err := http.NewRequest(
		http.MethodDelete,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
		nil,
	)
	if err != nil {
		t.Fatalf("build DELETE Factory Session %q: %v", sessionID, err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("DELETE Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("DELETE Factory Session %q status = %d, want %d: %s", sessionID, response.StatusCode, http.StatusNoContent, strings.TrimSpace(string(body)))
	}
}

func terminateNegotiatedFactorySession(t *testing.T, baseURL, sessionID string) {
	t.Helper()

	request, err := http.NewRequest(
		http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/terminate",
		bytes.NewReader([]byte(`{}`)),
	)
	if err != nil {
		t.Fatalf("build terminate Factory Session %q: %v", sessionID, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST terminate Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST terminate Factory Session %q status = %d, want success: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func assertNegotiatedFactorySessionAbsent(t *testing.T, baseURL, sessionID string) {
	t.Helper()

	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET closed Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET closed Factory Session %q status = %d, want %d: %s", sessionID, response.StatusCode, http.StatusNotFound, strings.TrimSpace(string(body)))
	}
	var notFound factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&notFound); err != nil {
		t.Fatalf("decode closed Factory Session %q response: %v", sessionID, err)
	}
	if notFound.Family != factoryapi.ErrorFamilyNotFound || notFound.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("GET closed Factory Session %q response = %#v, want typed NOT_FOUND", sessionID, notFound)
	}
}

func assertMalformedJSONHTTPResponse(t *testing.T, response *http.Response) {
	t.Helper()

	if response.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"status = %d, want %d for malformed JSON body with documented application/json Content-Type: %s",
			response.StatusCode,
			http.StatusBadRequest,
			strings.TrimSpace(string(body)),
		)
	}
	if response.StatusCode == http.StatusUnsupportedMediaType {
		t.Fatal("malformed JSON probe returned unsupported media type, want HTTP 400 bad request")
	}

	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("Content-Type = %q, want application/json structured error body: %s", contentType, strings.TrimSpace(string(body)))
	}

	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&errResp); err != nil {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("decode structured malformed JSON response: %v\nbody: %s", err, strings.TrimSpace(string(body)))
	}
	if errResp.Code == "UNSUPPORTED_MEDIA_TYPE" {
		t.Fatalf("error code = %q, want bad-request code distinct from unsupported media type", errResp.Code)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("error code = %q, want %q", errResp.Code, factoryapi.ErrorResponseCodeBADREQUEST)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error family = %q, want %q", errResp.Family, factoryapi.ErrorFamilyBadRequest)
	}
	if strings.TrimSpace(errResp.Message) == "" {
		t.Fatal("error message is empty, want a customer-readable malformed JSON message")
	}
}

func assertUnsupportedMediaTypeHTTPResponse(t *testing.T, response *http.Response, documentedJSONMediaType string) {
	t.Helper()

	if response.StatusCode != http.StatusUnsupportedMediaType {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"status = %d, want %d for unsupported Content-Type against documented JSON media type %q: %s",
			response.StatusCode,
			http.StatusUnsupportedMediaType,
			documentedJSONMediaType,
			strings.TrimSpace(string(body)),
		)
	}
	if response.StatusCode == http.StatusOK {
		t.Fatal("unsupported Content-Type probe succeeded, want HTTP 415 rejection")
	}
	if response.StatusCode == http.StatusNotFound {
		t.Fatal("unsupported Content-Type probe returned not-found, want HTTP 415 media-type rejection")
	}
	if response.StatusCode == http.StatusBadRequest {
		t.Fatal("unsupported Content-Type probe returned bad-request, want HTTP 415 media-type rejection")
	}
	if response.StatusCode == http.StatusMethodNotAllowed {
		t.Fatal("unsupported Content-Type probe returned method-not-allowed, want HTTP 415 media-type rejection")
	}

	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("Content-Type = %q, want application/json structured error body: %s", contentType, strings.TrimSpace(string(body)))
	}

	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&errResp); err != nil {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("decode structured unsupported media type response: %v\nbody: %s", err, strings.TrimSpace(string(body)))
	}
	if errResp.Code == factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("error code = %q, want media-type rejection code distinct from bad-request validation", errResp.Code)
	}
	if errResp.Code == factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("error code = %q, want media-type rejection code distinct from not-found", errResp.Code)
	}
	if errResp.Code == factoryapi.ErrorResponseCodeMETHODNOTALLOWED {
		t.Fatalf("error code = %q, want media-type rejection code distinct from method-not-allowed", errResp.Code)
	}
	if errResp.Code != "UNSUPPORTED_MEDIA_TYPE" {
		t.Fatalf("error code = %q, want %q", errResp.Code, "UNSUPPORTED_MEDIA_TYPE")
	}
	if strings.TrimSpace(errResp.Message) == "" {
		t.Fatal("error message is empty, want a customer-readable unsupported media type message")
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
}
