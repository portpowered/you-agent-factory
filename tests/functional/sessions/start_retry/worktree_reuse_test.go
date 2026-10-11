package start_retry_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const initialOpeningCheckoutName = "customer-retained"

func newInitialOpeningWorktreeScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	config := initialOpeningFactoryConfig()
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	config["workstations"].([]map[string]any)[0]["worktree"] = initialOpeningCheckoutName
	scenario := initialOpeningScenarioWithConfig(t, config)
	for _, dir := range []string{scenario.candidateDir, scenario.peerDir} {
		support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
		prepareInitialOpeningCheckout(t, filepath.Join(dir, ".worktrees", initialOpeningCheckoutName))
	}
	return scenario
}

func prepareInitialOpeningCheckout(t *testing.T, checkout string) {
	t.Helper()
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatalf("prepare customer checkout: %v", err)
	}
	// Git itself is a controlled command edge; the selected checkout and its
	// customer-owned content are real, test-owned filesystem destinations.
	for name, content := range map[string]string{
		".git": "gitdir: controlled-customer-checkout\n", "customer.txt": "keep my uncommitted work\n",
	} {
		if err := os.WriteFile(filepath.Join(checkout, name), []byte(content), 0o644); err != nil {
			t.Fatalf("author customer checkout %s: %v", name, err)
		}
	}
}

func initialOpeningCheckoutPath(scenario initialOpeningScenario) string {
	return filepath.Join(scenario.candidateDir, ".worktrees", initialOpeningCheckoutName)
}

func testInitialOpeningWorktreeReuse(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, effects *initialOpeningEffects) {
	t.Helper()
	peerHistory := scenario.startPeer(t, sessions)
	request := scenario.request()
	checkout := initialOpeningCheckoutPath(scenario)
	// The second session uses the same Factory and checkout destination with
	// fresh session/runtime identities, preserving terminal recording identity.
	for generation := 1; generation <= 2; generation++ {
		startInitialOpeningSession(t, sessions, request)
		assertInitialOpeningInvocation(t, sessions, request.SessionID)
		initialOpeningHistory(t, sessions, request.SessionID)
		calls := effects.forScenario(scenario)
		if calls[checkout+"|git.verify"] != generation || calls[checkout+"|worker.codex"] != generation ||
			calls[filepath.Clean(scenario.candidateDir)+"|worker.codex"] != 0 {
			t.Fatalf("checkout generation %d external effects = %v, want validation and Codex execution in selected checkout", generation, calls)
		}
		closeInitialOpeningSession(t, sessions, request.SessionID)
		assertInitialOpeningCheckoutContent(t, checkout)
		if after := effects.forScenario(scenario); !reflect.DeepEqual(after, calls) {
			t.Fatalf("session close changed customer checkout effects: before %v, after %v", calls, after)
		}
		assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
		assertInitialOpeningInvocation(t, sessions, scenario.peerID)
		request.SessionID = uuid.NewString()
		request.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
	}
}

func assertInitialOpeningCheckoutContent(t *testing.T, checkout string) {
	t.Helper()
	for name, expected := range map[string]string{
		".git": "gitdir: controlled-customer-checkout\n", "customer.txt": "keep my uncommitted work\n",
	} {
		content, err := os.ReadFile(filepath.Join(checkout, name))
		if err != nil || string(content) != expected {
			t.Fatalf("customer checkout %s after session close = %q, %v, want %q", name, content, err, expected)
		}
	}
}

type initialOpeningWorktreeGit struct{ effects *initialOpeningEffects }

func (git initialOpeningWorktreeGit) Run(_ context.Context, dir string, args ...string) (string, string, int, error) {
	if filepath.Base(dir) == initialOpeningCheckoutName && reflect.DeepEqual(args, []string{"rev-parse", "--is-inside-work-tree"}) {
		git.effects.record("git.verify", dir)
		return "true\n", "", 0, nil
	}
	git.effects.record("git.unexpected", dir)
	return "", "", 1, fmt.Errorf("unexpected Git effect in %s: %v", dir, args)
}

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

func newOpeningPolicyScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	scenario := newInitialOpeningProviderScenario(t)
	path := filepath.Join(scenario.candidateDir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	config["invocationSignature"] = map[string]any{"parameters": []any{
		map[string]any{"name": "prompt", "required": true,
			"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}}},
		map[string]any{"name": "secret", "required": true, "sensitive": true,
			"bindings": []any{map[string]any{"kind": "NAMED"}}},
	}}
	support.WriteWorkstationConfig(t, scenario.candidateDir, "process", "---\ntype: MODEL_WORKSTATION\n---\nselected-control-"+scenario.candidateID+" secret=${secret} {{ (index .Inputs 0).Payload }}\n")
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(scenario.candidateDir, factorydefinitions.InputsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	return scenario
}

func newDeniedChildScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	scenario := newInitialOpeningChildScenario(t)
	path := filepath.Join(scenario.candidateDir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	javascript := config["orchestrator"].(map[string]any)["javascript"].(map[string]any)
	javascript["defaultPolicy"] = map[string]any{"maxAgents": 1, "concurrency": 1, "allowedModels": []string{"gpt-5-codex"}}
	javascript["inlineSource"].(map[string]any)["inline"] = `return agent.run({
  prompt: "selected policy child", label: "selected-child", modelProvider: "codex",
  model: args.prompt === "deny" ? "gpt-denied" : "gpt-5-codex", resourceId: "opening-child-slot"
});`
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return scenario
}

// M11 exercises policy admission through the same explicit runtime binding.
// The next allowed child must acquire its sole slot and finish, while the peer
// remains usable. Effect observations are scoped to this scenario's directory.
func testDeniedChildRetry(t *testing.T, process support.Process, sessions factorysessions.Service, scenario initialOpeningScenario, effects *initialOpeningEffects, serverURL string) {
	t.Helper()
	peer := scenario.startPeer(t, sessions)
	startInitialOpeningSession(t, sessions, scenario.request())
	for _, prompt := range []string{"deny", "allow"} {
		inputs := support.FakeInputs(t.Context(), []string{"you", "--remote", "--server", serverURL, "--session", scenario.candidateID,
			"--json", "run", "--factory", filepath.Join(scenario.candidateDir, "factory.json"), "--output", "primary", "--no-record", prompt})
		inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
		inputs.Input.WorkingDirectory = scenario.candidateDir
		err := process.Execute(inputs.Input)
		calls := effects.forScenario(scenario)[filepath.Clean(scenario.candidateDir)+"|worker.codex"]
		if prompt == "deny" {
			if err == nil || !strings.Contains(fmt.Sprint(err, inputs.Stdout(), inputs.Stderr()), `policy denied: model "gpt-denied" is not listed in allowedModels`) || calls != 0 {
				t.Fatalf("denied child: error=%v calls=%d stdout=%s stderr=%s", err, calls, inputs.Stdout(), inputs.Stderr())
			}
		} else {
			var response factoryapi.InvocationResponse
			if err != nil || json.Unmarshal([]byte(inputs.Stdout()), &response) != nil || response.Status != factoryapi.InvocationTerminalStatusCompleted || calls != 1 || !strings.Contains(inputs.Stdout(), "initial opening COMPLETE") {
				t.Fatalf("corrected child failed to reuse capacity: error=%v calls=%d stdout=%s stderr=%s", err, calls, inputs.Stdout(), inputs.Stderr())
			}
			if response.SessionId == nil || *response.SessionId == "" {
				t.Fatal("corrected child omitted durable identity")
			}
			assertInitialChildResponses(t, sessions, *response.SessionId)
		}
		assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peer)
	}
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
}

func openingPolicySecret(scenario initialOpeningScenario) string {
	return "private-policy-" + scenario.candidateID
}

func newTimedChildScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	scenario := newInitialOpeningChildScenario(t)
	path := filepath.Join(scenario.candidateDir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	javascript := config["orchestrator"].(map[string]any)["javascript"].(map[string]any)
	javascript["defaultPolicy"] = map[string]any{"maxAgents": 1, "concurrency": 1, "maxRetries": 0, "maxWorkerDurationMs": 1000}
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return scenario
}

// Child attempts intentionally use native context deadlines, separate from
// selected scheduling time. Await that deadline at the command edge rather
// than sleeping or substituting an arbitrary provider failure. The next
// invocation uses the same opened runtime's capacity-one resource.
func testTimedChildRetry(t *testing.T, process support.Process, sessions factorysessions.Service, scenario initialOpeningScenario, effects *initialOpeningEffects, gate *timedChildGate, serverURL string) {
	t.Helper()
	peer := scenario.startPeer(t, sessions)
	startInitialOpeningSession(t, sessions, scenario.request())
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", serverURL, "--session", scenario.candidateID,
		"--json", "run", "--factory", filepath.Join(scenario.candidateDir, "factory.json"), "--output", "primary", "--no-record", "timeout child"})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(initialOpeningReadCeiling):
			t.Error("selected timeout invocation did not join")
		}
	})
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("selected timeout command did not enter")
	}
	select {
	case cause := <-gate.stopped:
		if !errors.Is(cause, context.DeadlineExceeded) {
			t.Fatalf("provider termination cause = %v, want deadline exceeded", cause)
		}
	case <-ctx.Done():
		t.Fatal("selected command did not acknowledge its deadline")
	}
	select {
	case err := <-done:
		done <- err // Retain the joined signal for cleanup.
		assertTimedChildFailure(t, sessions, inputs.Stdout(), inputs.Stderr(), err)
	case <-ctx.Done():
		t.Fatal("timeout invocation did not publish terminal outcome")
	}
	if calls := effects.forScenario(scenario)[filepath.Clean(scenario.candidateDir)+"|worker.codex"]; calls != 1 {
		t.Fatalf("timeout command executions = %d, want one without retry", calls)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peer)
	assertTimedChildCapacityReuse(t, process, sessions, scenario, effects, ctx, inputs, peer)
}

func assertTimedChildCapacityReuse(t *testing.T, process support.Process, sessions factorysessions.Service, scenario initialOpeningScenario, effects *initialOpeningEffects, ctx context.Context, inputs *support.CapturedInputs, peer *factorydefinitions.FactoryEventStream) {
	t.Helper()
	retry := support.FakeInputs(ctx, inputs.Input.Args)
	retry.Input.Env, retry.Input.WorkingDirectory = inputs.Input.Env, inputs.Input.WorkingDirectory
	retryErr := process.Execute(retry.Input)
	var response factoryapi.InvocationResponse
	if retryErr != nil || json.Unmarshal([]byte(retry.Stdout()), &response) != nil || response.Status != factoryapi.InvocationTerminalStatusCompleted || response.SessionId == nil || !strings.Contains(retry.Stdout(), "initial opening COMPLETE") {
		t.Fatalf("post-timeout capacity reuse: error=%v stdout=%s stderr=%s", retryErr, retry.Stdout(), retry.Stderr())
	}
	assertInitialChildResponses(t, sessions, *response.SessionId)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peer)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
	if calls := effects.forScenario(scenario)[filepath.Clean(scenario.candidateDir)+"|worker.codex"]; calls != 2 {
		t.Fatalf("capacity reuse command executions = %d, want two", calls)
	}
}

func assertTimedChildFailure(t *testing.T, sessions factorysessions.Service, stdout, stderr string, err error) {
	t.Helper()
	var response factoryapi.InvocationResponse
	if json.Unmarshal([]byte(stdout), &response) != nil || err == nil || response.Status != factoryapi.InvocationTerminalStatusFailed || response.SessionId == nil {
		t.Fatalf("timeout invocation: error=%v stdout=%s stderr=%s", err, stdout, stderr)
	}
	dispatches, readErr := sessions.ListDispatches(t.Context(), *response.SessionId)
	if readErr != nil || len(dispatches.Dispatches) != 1 {
		t.Fatalf("timeout dispatches: %+v %v", dispatches, readErr)
	}
	dispatch := dispatches.Dispatches[0]
	if dispatch.Status != "FAILED" || dispatch.Attempt != 1 || dispatch.FailureClassification != "timeout" || dispatch.FailureDetail == nil || dispatch.FailureDetail.Reason != "timeout" {
		t.Fatalf("timeout classification/attempt: %+v", dispatch)
	}
	assertTimedChildDiagnostics(t, sessions, response, dispatch)
}

func assertTimedChildDiagnostics(t *testing.T, sessions factorysessions.Service, response factoryapi.InvocationResponse, dispatch factorysessions.DispatchSummary) {
	t.Helper()
	if response.ErrorCode == nil || response.Message == nil || !strings.Contains(*response.Message, "child execution timed out after 1s") || !strings.Contains(dispatch.FailureDetail.Message, "child execution timed out after 1s") {
		t.Fatalf("timeout diagnostic: response=%+v detail=%+v", response, dispatch.FailureDetail)
	}
	read, readErr := sessions.GetSession(t.Context(), *response.SessionId)
	if readErr != nil || read.Status != factorysessions.LifecycleStatusFailed {
		t.Fatalf("timeout durable terminal: %+v %v", read, readErr)
	}
}

type timedChildGate struct {
	dir     string
	entered chan struct{}
	stopped chan error
	first   atomic.Bool
}

func (gate *timedChildGate) run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if !gate.first.CompareAndSwap(false, true) {
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("initial opening COMPLETE")}, nil
	}
	close(gate.entered)
	<-ctx.Done()
	gate.stopped <- ctx.Err()
	return platformprocess.CommandResult{}, ctx.Err()
}
