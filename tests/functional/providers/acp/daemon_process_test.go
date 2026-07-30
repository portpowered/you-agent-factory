package acp_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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

func startACPDaemonProcess(t *testing.T, starts *atomic.Int32) *support.FunctionalAPIServer {
	t.Helper()
	home, err := os.MkdirTemp("", "you-acp-daemon-")
	if err != nil {
		t.Fatalf("create ACP daemon home: %v", err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		for {
			if err := os.RemoveAll(home); err == nil {
				return
			} else if time.Now().After(deadline) {
				t.Errorf("remove ACP daemon home %s: %v", home, err)
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	})
	factoryDir := support.InstallPackagedFactory(t, home, factorydefinitions.PackagedSpawnFactoryName)
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			PlatformProcessCommandFactory: acpHelperCommandFactory(starts),
			ProvidersExecutableLocator:    availableExecutableLocator{},
		},
	})
}

func invokeACPDaemonWorkflow(
	t *testing.T,
	server *support.FunctionalAPIServer,
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
	return decoded, nil
}
