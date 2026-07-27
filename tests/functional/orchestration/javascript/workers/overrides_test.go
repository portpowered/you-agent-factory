// Package workers holds customer functional scenarios for JavaScript per-child
// worker override behavior through the public process and invocation boundary.
package workers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	childCodexLabel    = "child-codex"
	childClaudeLabel   = "child-claude"
	childMockedLabel   = "child-mocked"
	childPassthroughLabel = "child-passthrough"

	mockedChildPrompt      = "mocked child prompt"
	passthroughChildPrompt = "passthrough child prompt"

	perChildProviderModelWorkflow = `return (async function () {
  const codexChild = await agent.run({
    prompt: "use codex provider and model",
    label: "` + childCodexLabel + `",
    modelProvider: "codex",
    model: "codex-child-model",
  });
  const claudeChild = await agent.run({
    prompt: "use claude provider and model",
    label: "` + childClaudeLabel + `",
    modelProvider: "claude",
    model: "claude-child-model",
  });
  return { codexChild, claudeChild };
})();`

	partialMockWorkersWorkflow = `return (async function () {
  const mocked = await agent.run({
    prompt: "` + mockedChildPrompt + `",
    label: "` + childMockedLabel + `",
  });
  const passthrough = await agent.run({
    prompt: "` + passthroughChildPrompt + `",
    label: "` + childPassthroughLabel + `",
  });
  return { mocked, passthrough };
})();`

	mockWorkerAcceptedOutput = "mock worker accepted"
	livePassthroughChildText = "passthrough child provider output"
)

// TestJavaScriptChildrenSelectDifferentProvidersAndModels proves a JavaScript
// Factory with multiple child dispatches can select distinct per-child provider
// and model overrides so each child completes with public dispatch and provider
// evidence matching its configured worker selection rather than a shared default.
func TestJavaScriptChildrenSelectDifferentProvidersAndModels(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, overridesFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")

	provider := testutil.NewMockProvider(
		workers.InferenceResponse{Content: `{"text":"codex child complete"}`},
		workers.InferenceResponse{Content: `{"text":"claude child complete"}`},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderOverride: provider},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startOverridesWorkflow(t, server.URL(), "javascript-per-child-provider-model", perChildProviderModelWorkflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	assertPerChildProviderModelDispatches(t, dispatches.Dispatches)
	assertPerChildProviderModelRequests(t, provider.Calls())
}

// TestJavaScriptMockWorkersReplaceOnlyNamedChildren proves a partial
// --with-mock-workers configuration serves named workstation workers through
// the mock-worker accept path while unmatched JavaScript child dispatches keep
// the live-provider path when unmatchedDispatchPolicy is passthrough and a
// provider edge is injected at the public process boundary.
func TestJavaScriptMockWorkersReplaceOnlyNamedChildren(t *testing.T) {
	t.Parallel()

	t.Run("namedMockWorkerAcceptsConfiguredWorker", func(t *testing.T) {
		t.Parallel()

		dir := support.ScaffoldFactory(t, overridesFactoryConfig())
		support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")

		runner := support.NewRecordingCommandRunner("unexpected live provider execution")
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			UseMockWorkers:            true,
			MockWorkersConfig:         partialNamedJavaScriptMockWorkersConfig(),
			Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
		})
		t.Cleanup(func() { server.Stop(t) })

		started := startOverridesWorkflow(
			t,
			server.URL(),
			"javascript-partial-mock-workers-fake",
			partialMockWorkersWorkflow,
		)
		if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
		}
		if runner.CallCount() != 0 {
			t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
		}

		dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
			t,
			strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
		)
		assertPartialMockFakeChildDispatches(t, dispatches.Dispatches)
		assertPartialMockFakeChildPrimaryResult(t, started.Result)
	})

	t.Run("passthroughChildUsesLiveProvider", func(t *testing.T) {
		t.Parallel()

		dir := support.ScaffoldFactory(t, overridesFactoryConfig())
		support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")

		provider := testutil.NewMockProvider(
			workers.InferenceResponse{Content: `{"text":"` + livePassthroughChildText + `"}`},
			workers.InferenceResponse{Content: `{"text":"` + livePassthroughChildText + `"}`},
		)
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			UseMockWorkers:            true,
			MockWorkersConfig:         partialNamedJavaScriptMockWorkersConfig(),
			Edges:                     serviceedges.Edges{ProviderOverride: provider},
		})
		t.Cleanup(func() { server.Stop(t) })

		started := startOverridesWorkflow(
			t,
			server.URL(),
			"javascript-partial-mock-workers-passthrough",
			partialMockWorkersWorkflow,
		)
		if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
		}

		dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
			t,
			strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
		)
		assertPartialMockPassthroughChildDispatches(t, dispatches.Dispatches)
		assertPartialMockPassthroughProviderRequests(t, provider.Calls())
		assertPartialMockPassthroughPrimaryResult(t, started.Result)
	})
}

func partialNamedJavaScriptMockWorkersConfig() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "worker-a",
			WorkstationName: "process",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	}
}

func assertPartialMockFakeChildDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2 child dispatches", len(dispatches))
	}
	byLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		byLabel[*dispatch.Label] = dispatch
	}
	assertPartialMockFakeChildDispatch(t, byLabel[childMockedLabel], childMockedLabel, mockedChildPrompt)
	assertPartialMockFakeChildDispatch(t, byLabel[childPassthroughLabel], childPassthroughLabel, passthroughChildPrompt)
}

func assertPartialMockFakeChildDispatch(
	t *testing.T,
	dispatch factoryapi.FactorySessionDispatchSummary,
	wantLabel, wantPrompt string,
) {
	t.Helper()

	if dispatch.Label == nil || *dispatch.Label != wantLabel {
		t.Fatalf("dispatch label = %#v, want %q", dispatch.Label, wantLabel)
	}
	if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
		*dispatch.Javascript.ExecutionMode != "fake" {
		t.Fatalf("dispatch %s javascript projection = %#v, want fake execution mode", dispatch.Id, dispatch.Javascript)
	}
}

func assertPartialMockFakeChildPrimaryResult(t *testing.T, result *factoryapi.FactorySessionResult) {
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
		Mocked      map[string]any `json:"mocked"`
		Passthrough map[string]any `json:"passthrough"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode partial mock primary result: %v", err)
	}
	assertPartialMockFakeChildResult(t, evidence.Mocked, childMockedLabel, mockedChildPrompt)
	assertPartialMockFakeChildResult(t, evidence.Passthrough, childPassthroughLabel, passthroughChildPrompt)
}

func assertPartialMockFakeChildResult(
	t *testing.T,
	child map[string]any,
	wantLabel, wantPrompt string,
) {
	t.Helper()

	if child == nil {
		t.Fatalf("child result = nil, want structured child object")
	}
	if label, _ := child["label"].(string); label != wantLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], wantLabel)
	}
	if mode, _ := child["executionMode"].(string); mode != "fake" {
		t.Fatalf("child executionMode = %#v, want fake", child["executionMode"])
	}
	output, ok := child["output"].(map[string]any)
	if !ok || output == nil {
		t.Fatalf("child output = %#v, want structured output object", child["output"])
	}
	text, _ := output["text"].(string)
	if !strings.Contains(text, wantPrompt) {
		t.Fatalf("child output text = %#v, want deterministic fake output containing prompt %q", output["text"], wantPrompt)
	}
}

func assertPartialMockPassthroughChildDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2 child dispatches", len(dispatches))
	}
	byLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		byLabel[*dispatch.Label] = dispatch
	}
	assertPartialMockPassthroughChildDispatch(t, byLabel[childMockedLabel], childMockedLabel)
	assertPartialMockPassthroughChildDispatch(t, byLabel[childPassthroughLabel], childPassthroughLabel)
}

func assertPartialMockPassthroughChildDispatch(
	t *testing.T,
	dispatch factoryapi.FactorySessionDispatchSummary,
	wantLabel string,
) {
	t.Helper()

	if dispatch.Label == nil || *dispatch.Label != wantLabel {
		t.Fatalf("dispatch label = %#v, want %q", dispatch.Label, wantLabel)
	}
	if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
		*dispatch.Javascript.ExecutionMode != "live-provider" {
		t.Fatalf(
			"dispatch %s javascript projection = %#v, want live-provider execution mode",
			dispatch.Id,
			dispatch.Javascript,
		)
	}
}

func assertPartialMockPassthroughProviderRequests(
	t *testing.T,
	calls []workers.ProviderInferenceRequest,
) {
	t.Helper()

	if len(calls) != 2 {
		t.Fatalf("provider call count = %d, want 2 passthrough child dispatches", len(calls))
	}
	for index, call := range calls {
		if !strings.Contains(call.UserMessage, "child prompt") {
			t.Fatalf("provider call[%d] prompt = %q, want child prompt text", index, call.UserMessage)
		}
	}
}

func assertPartialMockPassthroughPrimaryResult(t *testing.T, result *factoryapi.FactorySessionResult) {
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
		Mocked      map[string]any `json:"mocked"`
		Passthrough map[string]any `json:"passthrough"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode partial mock passthrough primary result: %v", err)
	}
	assertPartialMockPassthroughChildResult(t, evidence.Mocked, childMockedLabel)
	assertPartialMockPassthroughChildResult(t, evidence.Passthrough, childPassthroughLabel)
}

func assertPartialMockPassthroughChildResult(
	t *testing.T,
	child map[string]any,
	wantLabel string,
) {
	t.Helper()

	if child == nil {
		t.Fatalf("child result = nil, want structured child object")
	}
	if label, _ := child["label"].(string); label != wantLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], wantLabel)
	}
	if mode, _ := child["executionMode"].(string); mode != "live-provider" {
		t.Fatalf("child executionMode = %#v, want live-provider", child["executionMode"])
	}
	output, ok := child["output"].(map[string]any)
	if !ok || output == nil {
		t.Fatalf("child output = %#v, want structured output object", child["output"])
	}
	text, _ := output["text"].(string)
	if !strings.Contains(text, livePassthroughChildText) {
		t.Fatalf("child output text = %#v, want injected provider output %q", output["text"], livePassthroughChildText)
	}
	if strings.Contains(text, mockWorkerAcceptedOutput) {
		t.Fatalf("child output text = %#v, want provider output not mock-worker accept text", output["text"])
	}
}

func overridesFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func startOverridesWorkflow(
	t *testing.T,
	serverURL, requestID, workflowSource string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   workflowSource,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal overrides workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build overrides workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start overrides workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start overrides workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode overrides workflow response: %v", err)
	}
	return started
}

func assertPerChildProviderModelDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2 child dispatches", len(dispatches))
	}
	byLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		byLabel[*dispatch.Label] = dispatch
	}
	assertOverridesDispatchSelection(
		t,
		byLabel[childCodexLabel],
		childCodexLabel,
		"codex",
		"codex-child-model",
	)
	assertOverridesDispatchSelection(
		t,
		byLabel[childClaudeLabel],
		childClaudeLabel,
		"claude",
		"claude-child-model",
	)
}

func assertOverridesDispatchSelection(
	t *testing.T,
	dispatch factoryapi.FactorySessionDispatchSummary,
	wantLabel, wantProvider, wantModel string,
) {
	t.Helper()

	if dispatch.Label == nil || *dispatch.Label != wantLabel {
		t.Fatalf("dispatch label = %#v, want %q", dispatch.Label, wantLabel)
	}
	gotProvider := dereferenceOverridesValue(dispatch.ModelProvider)
	gotModel := dereferenceOverridesValue(dispatch.Model)
	if gotProvider != wantProvider || gotModel != wantModel {
		t.Fatalf(
			"dispatch %s selection = provider=%q model=%q, want provider=%q model=%q",
			dispatch.Id,
			gotProvider,
			gotModel,
			wantProvider,
			wantModel,
		)
	}
}

func assertPerChildProviderModelRequests(
	t *testing.T,
	calls []workers.ProviderInferenceRequest,
) {
	t.Helper()

	if len(calls) != 2 {
		t.Fatalf("provider call count = %d, want 2", len(calls))
	}
	if calls[0].ModelProvider != "codex" || calls[0].Model != "codex-child-model" {
		t.Fatalf(
			"first provider request = provider %q model %q, want codex/codex-child-model",
			calls[0].ModelProvider,
			calls[0].Model,
		)
	}
	if calls[1].ModelProvider != "claude" || calls[1].Model != "claude-child-model" {
		t.Fatalf(
			"second provider request = provider %q model %q, want claude/claude-child-model",
			calls[1].ModelProvider,
			calls[1].Model,
		)
	}
}

func dereferenceOverridesValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
