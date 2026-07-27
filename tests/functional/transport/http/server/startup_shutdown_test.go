package server_test

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIServerStartsOnConfiguredListenerAndServesStatus proves the public API
// server becomes reachable on its configured loopback listener and serves a
// non-empty readiness status observation after start.
func TestAPIServerStartsOnConfiguredListenerAndServesStatus(t *testing.T) {
	configuredURL := reserveConfiguredLoopbackURL(t)
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--server", configuredURL},
	})
	defer server.Stop(t)

	listenerURL := server.URL()
	if listenerURL == "" {
		t.Fatal("started API server returned empty listener URL")
	}
	if !strings.HasPrefix(listenerURL, "http://127.0.0.1:") &&
		!strings.HasPrefix(listenerURL, "http://localhost:") {
		t.Fatalf("listener URL = %q, want loopback HTTP URL", listenerURL)
	}

	response, err := http.Get(listenerURL + "/status")
	if err != nil {
		t.Fatalf("GET %s/status: %v", listenerURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s/status status = %d, want %d", listenerURL, response.StatusCode, http.StatusOK)
	}

	status := support.GetJSON[factoryapi.StatusResponse](t, listenerURL+"/status")
	if status.FactoryState == "" {
		t.Fatal("GET /status returned empty factoryState")
	}
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
}

func reserveConfiguredLoopbackURL(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve configured loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release configured loopback port: %v", err)
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func startupShutdownTestFactoryConfig() map[string]any {
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
