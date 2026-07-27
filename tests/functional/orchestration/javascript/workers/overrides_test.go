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
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	childCodexLabel  = "child-codex"
	childClaudeLabel = "child-claude"

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
		workerexecution.InferenceResponse{Content: `{"text":"codex child complete"}`},
		workerexecution.InferenceResponse{Content: `{"text":"claude child complete"}`},
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
	calls []workerexecution.ProviderInferenceRequest,
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
