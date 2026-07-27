// Package composition holds customer functional scenarios for JavaScript agent
// composition primitives through the public process and invocation boundary.
package composition_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
func TestJavaScriptAgentReturnsUnaryResult(t *testing.T) {
	t.Parallel()

	dir := scaffoldAgentUnaryWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startAgentUnaryWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}

	dispatches := listAgentUnaryDispatches(t, server.URL(), started.SessionId)
	assertExactlyOneCompletedChildDispatch(t, dispatches.Dispatches)
	assertUnaryAgentPrimaryResult(t, started.Result)
}

// TestJavaScriptAgentFailureReturnsStableFailureRecord proves a failed agent.run
// child dispatch surfaces as a stable public failure record on the Factory Session
// dispatch listing and result surfaces without private JavaScript VM diagnostics.
func TestJavaScriptAgentFailureReturnsStableFailureRecord(t *testing.T) {
	t.Parallel()

	dir := scaffoldAgentFailureWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startAgentFailureWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child failure edge", runner.CallCount())
	}

	dispatches := listAgentFailureDispatches(t, server.URL(), started.SessionId)
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

func startAgentUnaryWorkflow(t *testing.T, serverURL, dir string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-agent-unary-composition",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal agent unary workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build agent unary workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start agent unary workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start agent unary workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode agent unary workflow response: %v", err)
	}
	return started
}

func startAgentFailureWorkflow(t *testing.T, serverURL, dir string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-agent-failure-composition",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal agent failure workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build agent failure workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start agent failure workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start agent failure workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode agent failure workflow response: %v", err)
	}
	return started
}

func listAgentUnaryDispatches(
	t *testing.T,
	serverURL string,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	return support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/dispatches",
	)
}

func listAgentFailureDispatches(
	t *testing.T,
	serverURL string,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	return support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/dispatches",
	)
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
