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
	// Only this explicitly opened Factory has this resource. Looking up the
	// process's default runtime or the active peer cannot admit this child.
	config["resources"] = []map[string]any{{"id": "opening-child-slot", "name": "child-slot", "capacity": 1}}
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
    modelProvider: "codex", model: "gpt-5-codex", resourceId: "opening-child-slot"});
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
	assertInitialChildResponses(t, sessions, *response.SessionId)
	assertInitialOpeningProviderSelection(t, effects, scenario.candidateDir)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
}

func assertInitialChildResponses(t *testing.T, sessions factorysessions.Service, sessionID string) {
	t.Helper()
	// The legacy response subscription selects only live sessions. This public
	// cursor selects the completed durable child's retained progress instead.
	subscription, err := sessions.SubscribeResponses(t.Context(), factorysessions.SessionResponseSubscriptionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("subscribe durable child responses: %v", err)
	}
	defer subscription.Cursor.Detach()
	events, err := subscription.Cursor.Next(t.Context())
	if err != nil {
		t.Fatalf("read retained child responses: %v", err)
	}
	var messages, terminals int
	var lastSequence int64
	for _, event := range events {
		if event.FactorySessionID != sessionID || event.Sequence <= lastSequence || event.DispatchID == "" {
			t.Fatalf("child response lost identity or order: session=%s dispatch=%s sequence=%d after=%d", event.FactorySessionID, event.DispatchID, event.Sequence, lastSequence)
		}
		lastSequence = event.Sequence
		if event.Provenance.NativeEventType == "STREAM_COMPLETED" && event.Phase == factorysessions.ResponseEventPhaseCompleted {
			terminals++
		}
		if assertInitialChildNativeMessage(t, event) {
			messages++
		}
	}
	if messages != 1 || terminals != 1 {
		t.Fatalf("retained child responses: native messages=%d terminal completions=%d, want one each", messages, terminals)
	}
}

func assertInitialChildNativeMessage(t *testing.T, event factorysessions.FactoryResponseEvent) bool {
	t.Helper()
	// Compatibility deltas may repeat text from the provider's snapshot.
	// Native provenance distinguishes callback delivery from result shaping.
	if event.Kind != factorysessions.ResponseEventKindMessage || event.Phase != factorysessions.ResponseEventPhaseCompleted || event.Provenance.Delivery != factorysessions.ResponseEventDeliveryNativeStream {
		return false
	}
	var message factorysessions.ResponseEventMessage
	if err := json.Unmarshal(event.Payload, &message); err != nil {
		t.Fatalf("decode provider message: %v", err)
	}
	if event.Provenance.Provider != "codex" || len(message.ContentBlocks) != 1 || message.ContentBlocks[0].Text != "initial opening COMPLETE" {
		t.Fatalf("child provider message = %s provenance=%#v", event.Payload, event.Provenance)
	}
	return true
}
