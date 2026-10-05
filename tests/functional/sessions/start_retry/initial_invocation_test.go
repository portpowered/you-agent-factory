package start_retry_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func newInitialOpeningChildScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	config := map[string]any{"name": "initial-opening-child"}
	config["invocationSignature"] = map[string]any{
		"parameters": []any{map[string]any{"name": "prompt", "required": false,
			"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}}}},
	}
	config["orchestrator"] = map[string]any{
		"kind": "JAVASCRIPT",
		"javascript": map[string]any{
			"argsSchema": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string"}}, "additionalProperties": false},
			"inlineSource": map[string]any{
				"encoding": "utf-8",
				"inline": `return (async function () {
  return await agent.run({prompt: "prove opened session child", label: "opening-child",
    modelProvider: "codex", model: "gpt-5-codex"});
})();`,
			},
		},
	}
	// The peer remains a Petri Factory so its existing public Work/history
	// observers distinguish the candidate's child execution from peer Work.
	scenario := newInitialOpeningScenario(t)
	scenario.candidateDir = support.ScaffoldFactory(t, config)
	return scenario
}

func testInitialOpeningChildInvocation(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, effects *initialOpeningEffects, serverURL string) {
	t.Helper()
	peerHistory := scenario.startPeer(t, sessions)
	startInitialOpeningSession(t, sessions, scenario.request())
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--remote", "--server", serverURL, "--session", scenario.candidateID,
		"--json", "run", "--factory", filepath.Join(scenario.candidateDir, "factory.json"), "--output", "primary", "--no-record", "invoke the opened session child",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("CLI child invocation: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode invocation response: %v stdout=%s", err, inputs.Stdout())
	}
	if response.SessionId == nil || *response.SessionId == "" {
		t.Fatalf("child invocation omitted durable session identity: %#v", response)
	}
	if !strings.Contains(inputs.Stdout(), "initial opening COMPLETE") {
		t.Fatalf("child failed to return selected provider output: %s effects=%v", inputs.Stdout(), effects.forScenario(scenario))
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("child invocation status = %s, want COMPLETED", response.Status)
	}
	read, err := sessions.GetSession(t.Context(), *response.SessionId)
	if err != nil || read.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("read durable child completion: %#v, %v", read, err)
	}
	assertInitialOpeningProviderSelection(t, effects, scenario.candidateDir)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
}
