package acp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "tournament")
	var starts atomic.Int32
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create operator config directory: %v", err)
	}
	config := []byte(`{"workers":{"acp":{"integrations":[{"id":"cursor-functional","name":"cursor-acp","transport":"stdio","command":"cursor-agent acp"}]}}}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}
	factoryDir := support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedTournamentFactoryName)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true, Env: packagedACPEnvironment(homeDir),
		Edges: serviceedges.Edges{
			PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
			ProvidersExecutableLocator:    availableExecutableLocator{},
		},
	})
	factory := support.GetJSON[factoryapi.Factory](t, server.URL()+"/factory-sessions/~default/factory")
	workflowFile := filepath.Join(factoryDir, "scripts", "tournament.workflow.js")
	args := map[string]any{
		"request": "propose a launch strategy", "rounds": 1,
		"executorProvider": "ACP", "modelProvider": "cursor-acp",
		"judgeExecutorProvider": "ACP", "judgeModelProvider": "cursor-acp",
	}
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: "packaged-tournament-acp-stdio",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindWorkflowFile, WorkflowFile: &workflowFile,
		},
		Args: &args, Orchestrator: factory.Orchestrator,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal packaged tournament request: %v", err)
	}
	response, err := http.Post(server.URL()+"/factory-sessions/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("invoke packaged tournament: %v", err)
	}
	defer response.Body.Close()
	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode packaged tournament response: %v", err)
	}
	if result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("packaged tournament status = %q, want SUCCEEDED; result=%#v", result.Status, result)
	}
	encoded, _ := json.Marshal(result.Result)
	for _, want := range []string{`"rounds":1`, `"entrantCount":2`, `"entrant":2`, "candidate two is stronger"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("packaged tournament result = %s, want %q", encoded, want)
		}
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want one persistent stdio peer for three agents", starts.Load())
	}
	server.Stop(t)
}
