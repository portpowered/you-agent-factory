// Package composition holds customer functional scenarios for JavaScript agent
// composition primitives through the public process and invocation boundary.
package composition_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agentUnaryChildLabel  = "agent-unary-child"
	agentUnaryChildPrompt = "compose-one-child"
	agentUnaryWorkflow    = `return (async function () {
  const child = await agent.run({
    prompt: "` + agentUnaryChildPrompt + `",
    label: "` + agentUnaryChildLabel + `",
  });
  return { child };
})();`

	agentFailureChildLabel  = "agent-failure-child"
	agentFailureChildPrompt = "fail:compose-failing-child"
	agentFailureWorkflow    = `return (async function () {
  const child = await agent.run({
    prompt: "` + agentFailureChildPrompt + `",
    label: "` + agentFailureChildLabel + `",
  });
  return { child };
})();`
)

// TestJavaScriptAgentReturnsUnaryResult proves a JavaScript Factory that
// performs one agent.run child dispatch completes with exactly one unary
// structured child result on the public primary result surface and one
// completed child dispatch on the public Factory Session dispatch listing.
func runJavaScriptAgentReturnsUnaryResult(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldAgentUnaryWorkflow(t)
	providerCalls := fixture.provider.callCount()
	started := startAgentUnaryWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.provider.callCount(); got != providerCalls {
		t.Fatalf("provider call count = %d, want unchanged at %d for fake child execution", got, providerCalls)
	}

	dispatches := listAgentUnaryDispatches(t, fixture, started.SessionId)
	assertExactlyOneCompletedChildDispatch(t, dispatches.Dispatches)
	assertUnaryAgentPrimaryResult(t, started.Result)
}

// TestJavaScriptAgentFailureReturnsStableFailureRecord proves a failed agent.run
// child dispatch surfaces as a stable public failure record on the Factory Session
// dispatch listing and result surfaces without private JavaScript VM diagnostics.
func runJavaScriptAgentFailureReturnsStableFailureRecord(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldAgentFailureWorkflow(t)
	providerCalls := fixture.provider.callCount()
	started := startAgentFailureWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", started.Status)
	}
	if got := fixture.provider.callCount(); got != providerCalls {
		t.Fatalf("provider call count = %d, want unchanged at %d for fake child failure edge", got, providerCalls)
	}

	dispatches := listAgentFailureDispatches(t, fixture, started.SessionId)
	assertExactlyOneFailedChildDispatch(t, dispatches.Dispatches)
	assertStableAgentFailureRecord(t, started.Result)
}

func scaffoldAgentUnaryWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-agent-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(agentUnaryWorkflow), 0o600); err != nil {
		t.Fatalf("write agent unary workflow: %v", err)
	}
	return dir
}

func scaffoldAgentFailureWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-agent-composition-failure"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(agentFailureWorkflow), 0o600); err != nil {
		t.Fatalf("write agent failure workflow: %v", err)
	}
	return dir
}

func startAgentUnaryWorkflow(
	t *testing.T,
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startFakeSync(t, "javascript-agent-unary-composition", dir)
}

func startAgentFailureWorkflow(
	t *testing.T,
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startFakeSync(t, "javascript-agent-failure-composition", dir)
}

func listAgentUnaryDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.fakeDispatches(t, sessionID)
}

func listAgentFailureDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.fakeDispatches(t, sessionID)
}

func assertExactlyOneCompletedChildDispatch(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want exactly 1 child dispatch", len(dispatches))
	}
	dispatch := dispatches[0]
	if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
	}
	if dispatch.Label == nil || *dispatch.Label != agentUnaryChildLabel {
		t.Fatalf("dispatch label = %#v, want %q", dispatch.Label, agentUnaryChildLabel)
	}
	if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
		*dispatch.Javascript.ExecutionMode != "fake" {
		t.Fatalf("dispatch javascript projection = %#v, want fake execution mode", dispatch.Javascript)
	}
}

func assertExactlyOneFailedChildDispatch(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want exactly 1 child dispatch", len(dispatches))
	}
	dispatch := dispatches[0]
	if dispatch.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch status = %q, want FAILED", dispatch.Status)
	}
	if dispatch.Label == nil || *dispatch.Label != agentFailureChildLabel {
		t.Fatalf("dispatch label = %#v, want %q", dispatch.Label, agentFailureChildLabel)
	}
	if dispatch.FailureDetail == nil {
		t.Fatalf("dispatch failureDetail = nil, want stable public failure record")
	}
	if dispatch.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown {
		t.Fatalf("dispatch failure reason = %q, want %q", dispatch.FailureDetail.Reason, workerexecution.WorkFailureTypeUnknown)
	}
	if !strings.Contains(dispatch.FailureDetail.Message, "compose-failing-child") {
		t.Fatalf("dispatch failure message = %#v, want stable customer-readable failure signal", dispatch.FailureDetail.Message)
	}
	for _, leaked := range []string{"stack", "heap", "goja", "VM"} {
		if strings.Contains(dispatch.FailureDetail.Message, leaked) {
			t.Fatalf("dispatch failure message leaked non-customer detail %q: %q", leaked, dispatch.FailureDetail.Message)
		}
	}
	if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
		*dispatch.Javascript.ExecutionMode != "fake" {
		t.Fatalf("dispatch javascript projection = %#v, want fake execution mode", dispatch.Javascript)
	}
}

func assertStableAgentFailureRecord(t *testing.T, result *factoryapi.FactorySessionResult) {
	t.Helper()

	if result == nil {
		t.Fatal("result = nil, want failed Factory Session result")
	}
	if result.SessionStatus == nil || *result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("result sessionStatus = %#v, want FAILED", result.SessionStatus)
	}
	if result.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("result status = %q, want UNAVAILABLE on failed agent composition", result.ResultStatus)
	}
	if result.PrimaryResult != nil {
		t.Fatalf("primary result = %#v, want nil on failed agent composition", result.PrimaryResult)
	}
	if result.FailureDetail != nil {
		for _, leaked := range []string{"stack", "heap", "goja", "VM"} {
			if strings.Contains(result.FailureDetail.Message, leaked) {
				t.Fatalf("result failure message leaked non-customer detail %q: %q", leaked, result.FailureDetail.Message)
			}
		}
	}
}

func assertUnaryAgentPrimaryResult(t *testing.T, result *factoryapi.FactorySessionResult) {
	t.Helper()

	if result == nil || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL Factory Session result", result)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	encoded, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("encode primary result JSON: %v", err)
	}
	var evidence struct {
		Child map[string]any `json:"child"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode unary agent primary result: %v", err)
	}
	child := evidence.Child
	if child == nil {
		t.Fatalf("primary result = %#v, want unary child object", part.Json)
	}
	if status, _ := child["status"].(string); status != "COMPLETED" {
		t.Fatalf("child status = %#v, want COMPLETED", child["status"])
	}
	if childIndex, ok := child["childIndex"].(float64); !ok || int(childIndex) != 1 {
		t.Fatalf("child index = %#v, want 1", child["childIndex"])
	}
	if dispatchID, _ := child["dispatchId"].(string); strings.TrimSpace(dispatchID) == "" {
		t.Fatalf("child dispatchId = %#v, want non-empty completed dispatch id", child["dispatchId"])
	}
	if label, _ := child["label"].(string); label != agentUnaryChildLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], agentUnaryChildLabel)
	}
	output, ok := child["output"].(map[string]any)
	if !ok || output == nil {
		t.Fatalf("child output = %#v, want structured output object", child["output"])
	}
	if text, _ := output["text"].(string); !strings.Contains(text, agentUnaryChildPrompt) {
		t.Fatalf("child output text = %#v, want deterministic fake output containing prompt %q", output["text"], agentUnaryChildPrompt)
	}
}
