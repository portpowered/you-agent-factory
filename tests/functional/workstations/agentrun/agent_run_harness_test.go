// Package agentrun holds the functional probe that proves whether a Petri
// dispatch to an AGENT_RUN workstation still reaches the agent-run harness.
//
// The probe deliberately asserts on *provider interaction* rather than on
// emitted Factory Events. Runtime synthesizes agent-run response events on the
// detached dispatch path (recordDetachedAgentRunResponse in
// pkg/services/factory_runtime/internal/services/orchestration/runtime/worker_pool.go),
// so every event-shape assertion passes whether or not the harness ran. Only
// the bytes that reach the provider process can tell the two worlds apart.
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
// Measured provider turn counts (deterministic, 3 runs each):
//
//	merge-base a1603f0c3 : 1 provider invocation, prompt "User: AGENT_RUN_PROBE_BODY ..."
//	HEAD       ab963343d : 1 provider invocation, prompt "AGENT_RUN_PROBE_BODY ..."
//
// The harness never drove a second turn at either revision: agentrun's
// runnerInferencer returns messages.InferenceResult{Message: ...} and never
// populates InferenceResult.ToolCalls, so a text-only command runner can never
// make the agent loop dispatch a tool and take another turn. The turn count is
// therefore not the regression; the lost harness is. This assertion fails at
// HEAD and passes at the merge base.
//
// Dispatch chain at the merge base (captured with runtime/debug.PrintStack):
//
//	runtime.startThroughStatelessWorkers
//	  -> attemptLifecycle.startWithPreparation -> executeSafely
//	  -> factory_runtime/internal.workstationExecuteAdapter.executeThroughBoundary
//	  -> workers.workstationPoolBoundary.Publish
//	  -> workstations/internal/service.Pool.DispatchWithAdmission
//	  -> executor.WorkstationExecutor.executeInnerWorker
//	  -> executor.WorkstationBehaviorRouter.Execute
//	  -> agentrun.AgentRunExecutor.Execute (agent loop + tool policy)
//
// Dispatch chain at HEAD: the same publisher now resolves cfg.executeService to
// the stateless Workers service, which never consults the workstation type:
//
//	runtime.startThroughStatelessWorkers
//	  -> attemptLifecycle.startWithPreparation -> executeSafely
//	  -> workers/internal/service.Service.Execute
//	  -> workers/internal/service.Service.runRunner
//	  -> runners.Execute(Identity: runners.AgentIdentity)   (execute.go:253)
//
// The switch is the branch reordering in
// pkg/services/factory_runtime/internal/runtime_build.go
// executeServiceFromWorkstation: the runtimeExecuteService type assertion
// (line 598) now precedes the `boundary != nil` branch (line 601), so the
// workstation pool boundary that reaches WorkstationBehaviorRouter is no
// longer selected for Petri dispatch.
func TestAgentRunWorkstationDispatchExecutesAgentRunHarness(t *testing.T) {
	dir := support.ScaffoldFactory(t, agentRunProbeFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"request":"probe the agent-run harness"}`))

	runner := newAgentRunProbeRunner()
	session, _, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
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
