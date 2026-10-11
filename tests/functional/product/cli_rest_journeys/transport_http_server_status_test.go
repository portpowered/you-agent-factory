package cli_rest_journeys_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The injected HTTP transport supplies an already-expired server request only
// for this caller's fault cell. It delegates to the complete production handler;
// no response, observation role or session state is replaced.
func c06DeadlineRequestHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Request-Deadline") == "expired" {
			ctx, cancel := context.WithDeadline(r.Context(), time.Unix(0, 0))
			defer cancel()
			r = r.WithContext(ctx)
		}
		handler.ServeHTTP(w, r)
	})
}

func testTransporthttpserverAPIAddressedReadDeadlinePreservesPeer(t *testing.T) {
	t.Parallel()
	dir := scaffoldC06HTTPFactory(t, idleStatusTestFactoryConfig())
	scenario := c06SharedHTTPServer(t).newScenario(t, "addressed-read-deadline", dir)
	selected := scenario.openSession(t, dir)
	peer := scenario.openSession(t, dir)
	for _, cell := range []struct {
		suffix  string
		status  int
		message string
	}{
		{"/status", http.StatusInternalServerError, "failed to get engine state snapshot"},
		{"", http.StatusGatewayTimeout, "factory session request timed out"},
	} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, scenario.URL()+"/factory-sessions/"+selected+cell.suffix, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Test-Request-Deadline", "expired")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		var failure factoryapi.ErrorResponse
		if err := json.Unmarshal(body, &failure); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != cell.status || failure.Code != factoryapi.ErrorResponseCodeINTERNALERROR || failure.Family != factoryapi.ErrorFamilyInternalServerError || failure.Message != cell.message {
			t.Fatalf("expired addressed read %q = %d %s", cell.suffix, response.StatusCode, body)
		}
		assertStatusResponseDoesNotExposeFactoryPaths(t, body, dir)
	}
	for _, id := range []string{selected, peer} {
		if got := getFactorySession(t, scenario.URL(), id); got.Id != id {
			t.Fatalf("session after deadline = %q, want %q", got.Id, id)
		}
		if got := getFactorySessionStatus(t, scenario.URL(), id); got.RuntimeStatus == "" || got.FactoryState == "" {
			t.Fatalf("session %q after deadline = %#v", id, got)
		}
	}
}

func testTransporthttpserverAPIStatusMissingSessionDoesNotFallBack(t *testing.T) {
	t.Parallel()
	fixture := c06SharedHTTPServer(t)
	missing := uuid.NewString()
	response, err := http.Get(fixture.URL() + "/factory-sessions/" + missing + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound || failure.Code != factoryapi.ErrorResponseCodeNOTFOUND || failure.Message != "factory session not found" {
		t.Fatalf("missing addressed status = %d %s", response.StatusCode, body)
	}
	assertStatusResponseDoesNotExposeFactoryPaths(t, body, fixture.factoryDir)
	// A missing explicit selection must not borrow the healthy current session.
	current := support.GetJSON[factoryapi.StatusResponse](t, fixture.URL()+"/status")
	if current.RuntimeStatus == "" || current.FactoryState == "" {
		t.Fatalf("current session after missing addressed read = %#v", current)
	}
}

// TestAPIStatusReportsReadyAfterStartup proves GET /status returns HTTP 200 with a
// populated public runtime summary after the customer process starts.
func testTransporthttpserverAPIStatusReportsReadyAfterStartup(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, idleStatusTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)

	response, err := http.Get(server.URL() + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /status status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.FactoryState == "" {
		t.Fatal("GET /status returned empty factoryState")
	}
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
}

// TestAPIStatusDoesNotLeakInternalConfiguration proves GET /status returns only the
// documented public StatusResponse fields and does not expose internal configuration.
func testTransporthttpserverAPIStatusDoesNotLeakInternalConfiguration(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, idleStatusTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)

	response, err := http.Get(server.URL() + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /status status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET /status body: %v", err)
	}
	assertStatusResponseOnlyPublicFields(t, body)
	assertStatusResponseDoesNotExposeFactoryPaths(t, body, dir)
	if err := support.ValidateProviderSessionFixtureContent(
		"transport-http-status",
		"GET /status",
		body,
	); err != nil {
		t.Fatalf("GET /status leaked forbidden material: %v", err)
	}
}

func assertStatusResponseOnlyPublicFields(t *testing.T, body []byte) {
	t.Helper()

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode GET /status JSON object: %v", err)
	}
	assertJSONObjectOnlyContainsKeys(
		t,
		"status response",
		payload,
		"categories",
		"factoryState",
		"lifecycleControlStatus",
		"resources",
		"runtimeStatus",
		"totalTokens",
	)

	var categories map[string]json.RawMessage
	if err := json.Unmarshal(payload["categories"], &categories); err != nil {
		t.Fatalf("decode status categories: %v", err)
	}
	assertJSONObjectOnlyContainsKeys(
		t,
		"status.categories",
		categories,
		"failed",
		"initial",
		"processing",
		"terminal",
	)

	if resourcesRaw, ok := payload["resources"]; ok && string(resourcesRaw) != "null" {
		var resources []map[string]json.RawMessage
		if err := json.Unmarshal(resourcesRaw, &resources); err != nil {
			t.Fatalf("decode status resources: %v", err)
		}
		for index, resource := range resources {
			assertJSONObjectOnlyContainsKeys(
				t,
				fmt.Sprintf("status.resources[%d]", index),
				resource,
				"available",
				"name",
				"total",
			)
		}
	}
}

func assertJSONObjectOnlyContainsKeys(
	t *testing.T,
	label string,
	object map[string]json.RawMessage,
	allowed ...string,
) {
	t.Helper()

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowedSet[key]; ok {
			continue
		}
		t.Fatalf("%s contains undeclared field %q", label, key)
	}
}

func assertStatusResponseDoesNotExposeFactoryPaths(t *testing.T, body []byte, factoryDir string) {
	t.Helper()

	text := string(body)
	for _, fragment := range []string{
		factoryDir,
		strings.ReplaceAll(factoryDir, "\\", "/"),
	} {
		fragment = strings.TrimSpace(fragment)
		if fragment == "" {
			continue
		}
		if strings.Contains(text, fragment) {
			t.Fatalf("GET /status exposed factory path %q", fragment)
		}
	}
}

func idleStatusTestFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
