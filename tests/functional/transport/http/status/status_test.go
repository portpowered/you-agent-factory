package status_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIStatusReportsReadyAfterStartup proves GET /status returns HTTP 200 with a
// populated public runtime summary after the customer process starts.
func TestAPIStatusReportsReadyAfterStartup(t *testing.T) {
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
func TestAPIStatusDoesNotLeakInternalConfiguration(t *testing.T) {
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
