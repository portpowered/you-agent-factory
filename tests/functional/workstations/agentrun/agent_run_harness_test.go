// Package agentrun holds the functional probe that proves a Petri dispatch to
// an AGENT_RUN workstation reaches the agent-run harness and publishes one
// interpretable final response event.
package agentrun

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// agentRunHarnessConversationPrefix is the exact conversation framing that
// agentrun.conversationPrompt renders for the first harness turn: the agent
// loop replays its message history as "User: ...", so a provider request that
// starts with this prefix can only have been produced by the agent-run
// harness. A dispatch that bypasses the harness sends the rendered workstation
// body verbatim.
//
// See pkg/services/workers/internal/services/workstations/executor/agentrun/inferencer.go.
const agentRunHarnessConversationPrefix = "User: "

const agentRunProbeBody = "AGENT_RUN_PROBE_BODY read probe.txt and report the contents."

// TestAgentRunWorkstationDispatchExecutesAgentRunHarness proves that a Petri
// dispatch to an AGENT_RUN workstation with an AGENT_WORKER is routed through
// WorkstationBehaviorRouter into the agent-run harness, instead of collapsing
// into one raw provider call on the stateless-workers execute path.
//
// The provider prompt assertion proves the dispatch reached the harness; the
// response-event assertion below proves the completion publication was neither
// lost nor duplicated and retained its structured public provenance.
func TestAgentRunWorkstationDispatchExecutesAgentRunHarness(t *testing.T) {
	dir := support.ScaffoldFactory(t, agentRunProbeFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"request":"probe the agent-run harness"}`))

	runner := newAgentRunProbeRunner()
	session, _, _, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t, dir, serviceedges.Edges{ProviderCommandRunner: runner}, 40*time.Second,
	)
	if session.Runtime.Progress.Categories.Terminal != 1 ||
		session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress = %+v, want one terminal and zero failed AGENT_RUN dispatches",
			session.Runtime.Progress.Categories,
		)
	}

	requests := runner.snapshot()
	if len(requests) == 0 {
		t.Fatal("provider invocations = 0, want the AGENT_RUN dispatch to reach the provider")
	}
	prompt := string(requests[0].Stdin)
	if !strings.Contains(prompt, agentRunProbeBody) {
		t.Fatalf("provider prompt = %q, want the rendered AGENT_RUN workstation body", prompt)
	}
	if !strings.HasPrefix(prompt, agentRunHarnessConversationPrefix) {
		t.Fatalf(
			"AGENT_RUN dispatch reached the provider without agent-run harness conversation framing: "+
				"provider turns = %d, prompt = %q; want a prompt rendered by "+
				"agentrun.conversationPrompt (prefix %q)",
			len(requests), prompt, agentRunHarnessConversationPrefix,
		)
	}
	assertAgentRunFinalResponseEvent(t, responseEvents, "I must read the probe file first.")
}

func assertAgentRunFinalResponseEvent(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
	wantText string,
) {
	t.Helper()

	matchingEvents := 0
	foundText := false
	for _, event := range events {
		if event.Kind != factoryapi.FactoryResponseEventKindMessage ||
			event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
			continue
		}
		if event.Provenance.Provider != "agent-run" ||
			event.Provenance.NativeEventType != "agent_final_response" ||
			event.Provenance.Delivery != factoryapi.FactoryResponseEventProvenanceDeliveryNativeFinal ||
			event.Provenance.Fidelity != factoryapi.FactoryResponseEventProvenanceFidelityFinalOnly ||
			event.Provenance.Representation != factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot {
			continue
		}
		matchingEvents++
		payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
		if err != nil {
			t.Fatalf("decode AGENT_RUN final response event payload: %v", err)
		}
		for _, block := range payload.ContentBlocks {
			text, err := block.AsFactoryResponseEventTextContentBlock()
			if err == nil && strings.Contains(text.Text, wantText) {
				foundText = true
				break
			}
		}
	}
	if matchingEvents != 1 {
		t.Fatalf(
			"AGENT_RUN final response event count = %d, want exactly one interpretable terminal event; events=%#v",
			matchingEvents,
			events,
		)
	}
	if foundText {
		return
	}
	t.Fatalf(
		"AGENT_RUN final response event lost its expected structured text: want provider=agent-run nativeType=agent_final_response text=%q, events=%#v",
		wantText,
		events,
	)
}

type agentRunProbeRunner struct {
	mu       sync.Mutex
	requests []platformprocess.CommandRequest
}

func newAgentRunProbeRunner() *agentRunProbeRunner {
	return &agentRunProbeRunner{}
}

// Run scripts a two-turn conversation: the first response asks for a tool and
// every later response completes. A harness that drove tool turns would call
// the runner more than once and carry the tool result into the next prompt.
func (runner *agentRunProbeRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	call := len(runner.requests)
	runner.mu.Unlock()
	if call == 1 {
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(
			"I must read the probe file first.\n" +
				`<tool_call>{"name":"read_file","arguments":{"path":"probe.txt"}}</tool_call>`,
		)}, nil
	}
	return platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("AGENT_RUN_PROBE_DONE"),
	}, nil
}

func (runner *agentRunProbeRunner) snapshot() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]platformprocess.CommandRequest(nil), runner.requests...)
}

func agentRunProbeFactoryConfig() map[string]any {
	return map[string]any{
		"name": "agent-run-harness-probe",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":            "prober",
			"type":            "AGENT_WORKER",
			"model":           "agent-run-probe-model",
			"modelProvider":   string(modelprovider.ProviderCodex),
			"skipPermissions": true,
			"agentTools":      map[string]any{"policy": "ENABLED"},
			"body":            "You are an agent-run probe worker.",
		}},
		// Declared as []any so ScaffoldFactory does not overwrite the authored
		// AGENT_RUN definition with a generated MODEL_WORKSTATION markdown file.
		"workstations": []any{
			map[string]any{
				"name":      "probe",
				"type":      "AGENT_RUN",
				"worker":    "prober",
				"body":      agentRunProbeBody,
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "done"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}
