package acp_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// acpDaemonProcess keeps the fresh application home alongside the real
// root-built server. The home is intentionally outside testing.T.TempDir so
// this package can verify that root and ACP-peer cleanup released every owned
// path before the test returns.
type acpDaemonProcess struct {
	*support.FunctionalAPIServer
	home string
}

const singleACPAgentWorkflow = `return (async function () {
  const result = await agent.run({
    label: "daemon-agent",
    prompt: "complete one ACP daemon turn",
    executorProvider: "ACP",
    modelProvider: "cursor-acp"
  });
  if (result.status !== "COMPLETED") { throw "ACP child failed"; }
  return result.output;
})();`

const parallelACPAgentWorkflow = `return (async function () {
  const results = await parallel([
    {label: "daemon-agent-1", prompt: "first", executorProvider: "ACP", modelProvider: "cursor-acp"},
    {label: "daemon-agent-2", prompt: "second", executorProvider: "ACP", modelProvider: "cursor-acp"}
  ]);
  if (results[0].status !== "COMPLETED" || results[1].status !== "COMPLETED") {
    throw "parallel ACP child failed";
  }
  return results;
})();`

func startACPDaemonProcess(t *testing.T, starts *atomic.Int32) *acpDaemonProcess {
	t.Helper()
	home, err := os.MkdirTemp("", "you-acp-daemon-")
	if err != nil {
		t.Fatalf("create ACP daemon home: %v", err)
	}
	var daemon *acpDaemonProcess
	t.Cleanup(func() {
		if daemon == nil {
			removeACPDaemonHome(t, home)
			return
		}
		daemon.cleanup(t)
	})
	factoryDir := support.InstallPackagedFactory(t, home, factorydefinitions.PackagedSpawnFactoryName)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			PlatformProcessCommandFactory: acpHelperCommandFactory(starts),
			ProvidersExecutableLocator:    availableExecutableLocator{},
		},
	})
	daemon = &acpDaemonProcess{FunctionalAPIServer: server, home: home}
	return daemon
}

func (daemon *acpDaemonProcess) Stop(t *testing.T) {
	t.Helper()
	if daemon == nil || daemon.FunctionalAPIServer == nil {
		return
	}
	daemon.FunctionalAPIServer.Stop(t)
	select {
	case <-daemon.Done():
	default:
		t.Errorf("ACP daemon Process.Execute did not join after cancellation")
	}
}

func (daemon *acpDaemonProcess) cleanup(t *testing.T) {
	t.Helper()
	daemon.Stop(t)
	daemon.Close(t)
	select {
	case <-daemon.Done():
	default:
		t.Errorf("ACP daemon Process.Execute remained live during cleanup")
	}
	assertACPDaemonListenerClosed(t, daemon.URL())
	removeACPDaemonHome(t, daemon.home)
}

func removeACPDaemonHome(t testing.TB, home string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		removeErr := os.RemoveAll(home)
		if removeErr == nil {
			_, statErr := os.Stat(home)
			if os.IsNotExist(statErr) {
				return
			}
			if statErr != nil {
				t.Errorf("inspect removed ACP daemon home %s: %v", home, statErr)
				return
			}
			removeErr = fmt.Errorf("path still exists after RemoveAll")
		}
		if time.Now().After(deadline) {
			t.Errorf("remove ACP daemon home %s: %v", home, removeErr)
			return
		}
		// Windows can release an inherited subprocess handle shortly after the
		// joined Process.Execute returns. This bounded retry observes that OS
		// cleanup edge; it is not scenario synchronization or a readiness pad.
		time.Sleep(25 * time.Millisecond)
	}
}

func assertACPDaemonListenerClosed(t testing.TB, baseURL string) {
	t.Helper()
	if strings.TrimSpace(baseURL) == "" {
		t.Errorf("ACP daemon listener URL is empty during cleanup")
		return
	}
	client := &http.Client{
		Timeout: 250 * time.Millisecond,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	defer client.CloseIdleConnections()
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	t.Errorf("ACP daemon listener remains reachable after cleanup: status=%d body=%q readError=%v", response.StatusCode, strings.TrimSpace(string(body)), readErr)
}

func invokeACPDaemonWorkflow(
	t *testing.T,
	server *acpDaemonProcess,
	requestID string,
	source string,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	t.Helper()
	factory := support.GetJSON[factoryapi.Factory](t, server.URL()+"/factory-sessions/~default/factory")
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   source,
				},
			},
		},
		Args: &map[string]any{
			"request": "exercise the ACP daemon",
			"count":   1,
		},
		Orchestrator: factory.Orchestrator,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, fmt.Errorf("marshal durable execution: %w", err)
	}
	response, err := http.Post(server.URL()+"/factory-sessions/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return factoryapi.FactorySessionSyncExecutionResponse{}, fmt.Errorf("durable execution status %d", response.StatusCode)
	}
	var decoded factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return decoded, fmt.Errorf("decode durable execution: %w", err)
	}
	if decoded.SessionId == "" {
		return decoded, fmt.Errorf("durable execution returned an empty Factory Session id")
	}
	return decoded, nil
}

func stopAndAssertACPServer(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()
	server.Stop(t)
	server.Close(t)
	select {
	case <-server.Done():
	default:
		t.Errorf("ACP server Process.Execute remained live after Stop")
	}
	assertACPDaemonListenerClosed(t, server.URL())
}
